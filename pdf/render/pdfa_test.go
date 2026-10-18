package render

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/internal/testutil"
	"github.com/tim-riep/pdx/pdf"
	"github.com/tim-riep/pdx/pdf/profile"
)

const facturX = `<?xml version="1.0" encoding="UTF-8"?>
<rsm:CrossIndustryInvoice xmlns:rsm="urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100"/>
`

func pdfa3b(t testing.TB) profile.Profile {
	p, err := profile.Lookup("pdfa-3b")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func pdfaDoc(t testing.TB) *areatree.Document {
	doc := textDoc(t)
	doc.Meta = areatree.Metadata{
		Title: "Rechnung Nr. 4711 – Grüße", Author: "Tim Riep", Subject: "PDF/A-3b-Test",
		Keywords: "Rechnung, Test", Creator: "pdx-Testsuite", Lang: "de-DE",
		XMP: []areatree.XMPSchema{{NamespaceURI: "https://pdx.example/ns/pdx-test/1.0#", Prefix: "pdxt", Description: "pdx Testschema",
			Properties: []areatree.XMPProperty{{Name: "Version", Value: "1.0", Description: "Version des Testschemas"}}}},
	}
	doc.Attachments = []areatree.Attachment{{
		Name: "factur-x.xml", MIMEType: "text/xml", Relationship: areatree.Alternative,
		Description: "Rechnungsdaten", Data: []byte(facturX),
	}}
	// Formen mit Grau und RGB ergänzen.
	doc.Pages[0].Areas = append(doc.Pages[0].Areas, shapes().Pages[0].Areas...)
	return doc
}

func TestPDFA3bGolden(t *testing.T) {
	b, err := Render(pdfaDoc(t), Options{Profile: pdfa3b(t), CreationDate: testDate})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testutil.CheckStructure(b); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"/GTS_PDFA1", "<pdfaid:part>3</pdfaid:part>", "/AFRelationship /Alternative",
		"/Subtype /text#2Fxml", "/EmbeddedFiles", "/UF (factur-x.xml)", "pdfaExtension:schemas"} {
		if !bytes.Contains(b, []byte(want)) {
			t.Errorf("%s fehlt", want)
		}
	}
	testutil.Golden(t, "testdata/pdfa3b.pdf", b)
	b2, _ := Render(pdfaDoc(t), Options{Profile: pdfa3b(t), CreationDate: testDate})
	if !bytes.Equal(b, b2) {
		t.Fatal("PDF/A-Ausgabe nicht reproduzierbar")
	}
	testutil.Conform(t, "pdfa3b.pdf", b, "pdfa-3b")
}

func TestPDFA3bShapesOnly(t *testing.T) {
	b, err := Render(shapes(), Options{Profile: pdfa3b(t), CreationDate: testDate})
	if err != nil {
		t.Fatal(err)
	}
	testutil.Conform(t, "shapes.pdf", b, "pdfa-3b")
}

func TestAttachmentValidation(t *testing.T) {
	cases := map[string]areatree.Attachment{
		"ohne Name": {MIMEType: "text/xml"},
		"MIME":      {Name: "a", MIMEType: "text"},
		"Beziehung": {Name: "a", MIMEType: "text/xml", Relationship: "Irgendwas"},
	}
	for name, a := range cases {
		doc := shapes()
		doc.Attachments = []areatree.Attachment{a}
		if _, err := Render(doc, Options{Profile: pdfa3b(t), CreationDate: testDate}); err == nil {
			t.Errorf("%s: Fehler erwartet", name)
		}
	}
}

// denyAttach ist ein Profil, das keine Anhänge erlaubt, wie später PDF/A-2b.
type denyAttach struct{ profile.PDF17 }

func (denyAttach) Name() string { return "ohne-anhang" }
func (d denyAttach) Allow(f pdf.Feature) error {
	if f == pdf.Attachment {
		return &pdf.FeatureError{Feature: f, Profile: d.Name(), Reason: "Test"}
	}
	return d.PDF17.Allow(f)
}

func TestProfileRefusesAttachment(t *testing.T) {
	doc := shapes()
	doc.Attachments = []areatree.Attachment{{Name: "a.xml", MIMEType: "text/xml", Data: []byte("<a/>")}}
	_, err := Render(doc, Options{Profile: denyAttach{}, CreationDate: testDate})
	var fe *pdf.FeatureError
	if !errors.As(err, &fe) || fe.Feature != pdf.Attachment {
		t.Fatalf("erwartet FeatureError für Anhänge, erhalten %v", err)
	}
}

func lookup(t testing.TB, spec string) profile.Profile {
	t.Helper()
	p, err := profile.Lookup(spec)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// PDF/A-2 erlaubt nur PDF/A-1- und PDF/A-2-Dokumente als Anhang
// (ISO 19005-2 6.8). Alle unzulässigen Anhänge stehen in einer Meldung.
func TestPDFA2Attachments(t *testing.T) {
	inner, err := Render(shapes(), Options{Profile: lookup(t, "pdfa-2b"), CreationDate: testDate})
	if err != nil {
		t.Fatal(err)
	}
	testutil.Conform(t, "innen-2b.pdf", inner, "pdfa-2b")

	doc := shapes()
	doc.Attachments = []areatree.Attachment{
		{Name: "daten.xml", MIMEType: "text/xml", Data: []byte(facturX)},
		{Name: "alt.pdf", MIMEType: "application/pdf", Data: []byte("%PDF-1.7\n%%EOF\n")},
		{Name: "anlage.pdf", MIMEType: "application/pdf", Data: inner},
	}
	_, err = Render(doc, Options{Profile: lookup(t, "pdfa-2b"), CreationDate: testDate})
	if err == nil {
		t.Fatal("unzulässige Anhänge angenommen")
	}
	for _, want := range []string{`"daten.xml" ist kein PDF`, `"alt.pdf" ist kein PDF/A-1- oder PDF/A-2-Dokument`, "6.8"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%q fehlt in %v", want, err)
		}
	}
	if strings.Contains(err.Error(), "anlage.pdf") {
		t.Errorf("PDF/A-2-Anhang abgelehnt: %v", err)
	}
	doc.Attachments = doc.Attachments[2:]
	b, err := Render(doc, Options{Profile: lookup(t, "pdfa-2b"), CreationDate: testDate})
	if err != nil {
		t.Fatal(err)
	}
	testutil.Conform(t, "pdfa2b-anhang.pdf", b, "pdfa-2b")
	// Unter PDF/A-3 ist jeder Anhang erlaubt.
	doc.Attachments = []areatree.Attachment{{Name: "daten.xml", MIMEType: "text/xml", Data: []byte(facturX)}}
	if _, err := Render(doc, Options{Profile: lookup(t, "pdfa-3u"), CreationDate: testDate}); err != nil {
		t.Fatal(err)
	}
}

// Text mit zwei Schriften in den Stufen U.
func TestPDFAUnicodeLevels(t *testing.T) {
	for _, spec := range []string{"pdfa-2u", "pdfa-3u"} {
		doc := textDoc(t)
		doc.Meta = areatree.Metadata{Title: "Unicode", Lang: "de-DE"}
		b, err := Render(doc, Options{Profile: lookup(t, spec), CreationDate: testDate})
		if err != nil {
			t.Fatalf("%s: %v", spec, err)
		}
		part := spec[5:6]
		if !bytes.Contains(b, []byte("<pdfaid:part>"+part+"</pdfaid:part>")) || !bytes.Contains(b, []byte("<pdfaid:conformance>U</pdfaid:conformance>")) {
			t.Errorf("%s: pdfaid fehlt", spec)
		}
		testutil.Conform(t, spec+".pdf", b, spec)
	}
}
