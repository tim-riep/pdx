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
		XMP: []areatree.XMPSchema{{NamespaceURI: "https://github.com/tim-riep/pdx/ns/test#", Prefix: "pdxt", Description: "pdx Testschema",
			Properties: []areatree.XMPProperty{{Name: "Version", Value: "1.0", Description: "Version des Testschemas"}}}},
	}
	doc.Attachments = []areatree.Attachment{{
		Name: "factur-x.xml", MIMEType: "text/xml", Relationship: areatree.Alternative,
		Description: "Rechnungsdaten", Data: []byte(facturX),
	}}
	// Add shapes with gray and RGB.
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

// denyAttach is a profile that allows no attachments, like PDF/A-2b later.
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

// PDF/A-2 only allows PDF/A-1 and PDF/A-2 documents as attachments
// (ISO 19005-2 6.8). All invalid attachments are in one message.
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
	for _, want := range []string{`"daten.xml" ist kein PDF`, `"alt.pdf" ist kein Dokument in PDF/A-1, PDF/A-2`, "6.8"} {
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
	// Under PDF/A-3 every attachment is allowed.
	doc.Attachments = []areatree.Attachment{{Name: "daten.xml", MIMEType: "text/xml", Data: []byte(facturX)}}
	if _, err := Render(doc, Options{Profile: lookup(t, "pdfa-3u"), CreationDate: testDate}); err != nil {
		t.Fatal(err)
	}
}

// Text with two fonts at the U levels.
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

// PDF/A-4 is based on PDF 2.0: header %PDF-2.0, no info dictionary,
// pdfaid:rev 2020 and no level except F. Under 4, attachments are only
// allowed as PDF/A-1, -2 or -4, 4f requires at least one.
func TestPDFA4(t *testing.T) {
	doc := textDoc(t)
	doc.Meta = areatree.Metadata{Title: "PDF/A-4", Lang: "de-DE"}
	b, err := Render(doc, Options{Profile: lookup(t, "pdfa-4"), CreationDate: testDate})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testutil.CheckStructure(b); err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(b, []byte("%PDF-2.0\n")) {
		t.Error("Kopf nicht %PDF-2.0")
	}
	trailer := b[bytes.LastIndex(b, []byte("trailer")):]
	if bytes.Contains(trailer, []byte("/Info")) {
		t.Error("Info-Dictionary darf unter PDF/A-4 nicht stehen")
	}
	for _, bad := range []string{"/Producer (", "pdfaid:conformance", "pdfaExtension"} {
		if bytes.Contains(b, []byte(bad)) {
			t.Errorf("%q darf unter PDF/A-4 nicht stehen", bad)
		}
	}
	if !bytes.Contains(b, []byte("<pdfaid:rev>2020</pdfaid:rev>")) {
		t.Error("pdfaid:rev fehlt")
	}
	testutil.Conform(t, "pdfa-4.pdf", b, "pdfa-4")

	// 4f without an attachment is an error, with an attachment conforming.
	if _, err := Render(doc, Options{Profile: lookup(t, "pdfa-4f"), CreationDate: testDate}); err == nil ||
		!strings.Contains(err.Error(), "ISO 19005-4 6.9") {
		t.Errorf("4f ohne Anhang: %v", err)
	}
	doc.Attachments = []areatree.Attachment{{Name: "daten.xml", MIMEType: "text/xml", Relationship: areatree.Data, Data: []byte(facturX)}}
	b, err = Render(doc, Options{Profile: lookup(t, "pdfa-4f"), CreationDate: testDate})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte("<pdfaid:conformance>F</pdfaid:conformance>")) {
		t.Error("pdfaid:conformance F fehlt")
	}
	testutil.Conform(t, "pdfa-4f.pdf", b, "pdfa-4f")

	// Under PDF/A-4 without a level only PDF/A attachments, PDF/A-3 is not one of them.
	inner, err := Render(shapes(), Options{Profile: pdfa3b(t), CreationDate: testDate})
	if err != nil {
		t.Fatal(err)
	}
	doc.Attachments = append(doc.Attachments, areatree.Attachment{Name: "a3.pdf", MIMEType: "application/pdf", Data: inner})
	_, err = Render(doc, Options{Profile: lookup(t, "pdfa-4"), CreationDate: testDate})
	for _, want := range []string{`"daten.xml" ist kein PDF`, `"a3.pdf" ist kein Dokument in PDF/A-1, PDF/A-2, PDF/A-4`} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q fehlt in %v", want, err)
		}
	}
}
