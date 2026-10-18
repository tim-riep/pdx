package einvoice

import (
	"strings"
	"testing"

	"github.com/tim-riep/pdx/areatree"
)

func cii(guideline string) []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<rsm:CrossIndustryInvoice xmlns:rsm="` + NSCII + `" xmlns:ram="` + NSRAM + `">
  <rsm:ExchangedDocumentContext>
    <ram:BusinessProcessSpecifiedDocumentContextParameter><ram:ID>A1</ram:ID></ram:BusinessProcessSpecifiedDocumentContextParameter>
    <ram:GuidelineSpecifiedDocumentContextParameter><ram:ID>` + guideline + `</ram:ID></ram:GuidelineSpecifiedDocumentContextParameter>
  </rsm:ExchangedDocumentContext>
</rsm:CrossIndustryInvoice>`)
}

func TestDetectLevels(t *testing.T) {
	for id, want := range map[string]struct {
		name, file string
		rel        areatree.Relationship
	}{
		"urn:factur-x.eu:1p0:minimum":                                           {"MINIMUM", "factur-x.xml", areatree.Data},
		"urn:factur-x.eu:1p0:basicwl":                                           {"BASIC WL", "factur-x.xml", areatree.Data},
		"urn:cen.eu:en16931:2017#compliant#urn:factur-x.eu:1p0:basic":           {"BASIC", "factur-x.xml", areatree.Alternative},
		"urn:cen.eu:en16931:2017":                                               {"EN 16931", "factur-x.xml", areatree.Alternative},
		"urn:cen.eu:en16931:2017#conformant#urn:factur-x.eu:1p0:extended":       {"EXTENDED", "factur-x.xml", areatree.Alternative},
		"urn:cen.eu:en16931:2017#compliant#urn:xeinkauf.de:kosit:xrechnung_3.0": {"XRECHNUNG", "xrechnung.xml", areatree.Alternative},
	} {
		info, err := Detect(cii(" " + id + "\n"))
		if err != nil {
			t.Errorf("%s: %v", id, err)
			continue
		}
		a := info.Attachment([]byte("x"), "")
		if info.Level.Name != want.name || a.Name != want.file || a.Relationship != want.rel || a.MIMEType != "text/xml" {
			t.Errorf("%s: %+v %+v", id, info.Level, a)
		}
		x := info.XMP()
		if x.Prefix != "fx" || x.NamespaceURI != NSXMP || x.Properties[3].Value != want.name || x.Properties[0].Value != want.file {
			t.Errorf("%s: XMP %+v", id, x)
		}
	}
	// Die Vorlage darf die Beziehung überschreiben.
	info, _ := Detect(cii("urn:factur-x.eu:1p0:minimum"))
	if a := info.Attachment(nil, areatree.Source); a.Relationship != areatree.Source {
		t.Errorf("Beziehung nicht überschrieben: %s", a.Relationship)
	}
}

func TestDetectErrors(t *testing.T) {
	for in, want := range map[string]string{
		`<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"/>`: "die Daten sind UBL (Invoice)",
		`<inv:invoice xmlns:inv="urn:beispiel"/>`:                                   "keine CII-Rechnung",
		`<rsm:CrossIndustryInvoice xmlns:rsm="` + NSCII + `"/>`:                     "BT-24) fehlt",
		string(cii("urn:unbekannt")):                                                `Guideline-ID "urn:unbekannt" ist unbekannt, bekannt sind urn:cen.eu`,
		`<kaputt`:                                                                   "kein wohlgeformtes XML",
	} {
		_, err := Detect([]byte(in))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%.40s: %v, erwartet %q", in, err, want)
		}
	}
}
