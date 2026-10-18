// Package einvoice beschreibt hybride Rechnungen nach Factur-X 1.0 und
// ZUGFeRD 2: Erkennung der Daten im Format CII, Ableitung der Stufe aus der
// Guideline-ID, Dateiname und Beziehung des eingebetteten XML sowie das
// XMP-Schema fx.
//
// Die Werte folgen ZUGFeRD 2.5.2 und Factur-X 1.09.2 und sind gegen die
// Referenzimplementierung Mustang (Apache 2.0) geprüft: Profiles.java,
// XMPSchemaZugferd.java, XMPSchemaPDFAExtensions.java und
// ZUGFeRDExporterFromA3.java.
package einvoice

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/tim-riep/pdx/areatree"
)

// Namespaces der Daten.
const (
	NSCII = "urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100"
	NSRAM = "urn:un:unece:uncefact:data:standard:ReusableAggregateBusinessInformationEntity:100"
	// UBL wird erkannt, um eine verständliche Meldung zu geben.
	nsUBLInvoice    = "urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
	nsUBLCreditNote = "urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2"
)

// NSXMP ist der Namespace des XMP-Schemas fx (Factur-X 1.0).
const NSXMP = "urn:factur-x:pdfa:CrossIndustryDocument:invoice:1p0#"

// Level ist eine Konformitätsstufe.
type Level struct {
	// Name ist der Wert von fx:ConformanceLevel, etwa "EN 16931".
	Name string
	// FileName ist der Name des eingebetteten XML.
	FileName string
	// Relationship ist die Vorgabe für AFRelationship. MINIMUM und BASIC WL
	// sind keine vollständigen Rechnungen, das XML ist dort Data, sonst
	// Alternative (ZUGFeRD 2.1.1 Technischer Anhang Teil A 2.2.2).
	Relationship areatree.Relationship
}

// levels ordnet die Guideline-IDs (BT-24) den Stufen zu.
var levels = map[string]Level{
	"urn:factur-x.eu:1p0:minimum":                                           {"MINIMUM", "factur-x.xml", areatree.Data},
	"urn:factur-x.eu:1p0:basicwl":                                           {"BASIC WL", "factur-x.xml", areatree.Data},
	"urn:cen.eu:en16931:2017#compliant#urn:factur-x.eu:1p0:basic":           {"BASIC", "factur-x.xml", areatree.Alternative},
	"urn:cen.eu:en16931:2017":                                               {"EN 16931", "factur-x.xml", areatree.Alternative},
	"urn:cen.eu:en16931:2017#conformant#urn:factur-x.eu:1p0:extended":       {"EXTENDED", "factur-x.xml", areatree.Alternative},
	"urn:cen.eu:en16931:2017#compliant#urn:xeinkauf.de:kosit:xrechnung_3.0": {"XRECHNUNG", "xrechnung.xml", areatree.Alternative},
	"urn:cen.eu:en16931:2017#compliant#urn:xeinkauf.de:kosit:xrechnung_3.0#conformant#urn:xeinkauf.de:kosit:extension:xrechnung_3.0": {"XRECHNUNG", "xrechnung.xml", areatree.Alternative},
}

// Info ist das Ergebnis der Erkennung.
type Info struct {
	Guideline string
	Level     Level
}

// XRechnung meldet, ob die Daten zusätzlich die Regeln von XRechnung
// erfüllen müssen.
func (i *Info) XRechnung() bool { return i.Level.Name == "XRECHNUNG" }

// Detect prüft, dass die Daten eine CII-Rechnung sind, und leitet die Stufe
// aus ram:GuidelineSpecifiedDocumentContextParameter/ram:ID ab.
func Detect(data []byte) (*Info, error) {
	d := xml.NewDecoder(bytes.NewReader(data))
	var root *xml.StartElement
	// Pfad der offenen Elemente, um die Guideline-ID zu finden.
	var path []xml.Name
	var guideline strings.Builder
	found := false
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("E-Rechnung: Daten sind kein wohlgeformtes XML: %v", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if root == nil {
				r := t.Copy()
				root = &r
				switch {
				case t.Name.Space == nsUBLInvoice || t.Name.Space == nsUBLCreditNote:
					return nil, fmt.Errorf("E-Rechnung: die Daten sind UBL (%s), Factur-X und ZUGFeRD verlangen CII (rsm:CrossIndustryInvoice)", t.Name.Local)
				case t.Name.Space != NSCII || t.Name.Local != "CrossIndustryInvoice":
					return nil, fmt.Errorf("E-Rechnung: Wurzel {%s}%s ist keine CII-Rechnung, erwartet rsm:CrossIndustryInvoice im Namespace %s", t.Name.Space, t.Name.Local, NSCII)
				}
			}
			path = append(path, t.Name)
		case xml.EndElement:
			if inGuideline(path) {
				found = true
			}
			path = path[:len(path)-1]
		case xml.CharData:
			if inGuideline(path) {
				guideline.Write(t)
			}
		}
		if found {
			break
		}
	}
	if root == nil {
		return nil, fmt.Errorf("E-Rechnung: Daten ohne Wurzelelement")
	}
	id := strings.TrimSpace(guideline.String())
	if id == "" {
		return nil, fmt.Errorf("E-Rechnung: ram:GuidelineSpecifiedDocumentContextParameter/ram:ID (BT-24) fehlt, daraus folgt die Stufe")
	}
	lv, ok := levels[id]
	if !ok {
		return nil, fmt.Errorf("E-Rechnung: Guideline-ID %q ist unbekannt, bekannt sind %s", id, strings.Join(Guidelines(), ", "))
	}
	return &Info{Guideline: id, Level: lv}, nil
}

// inGuideline meldet, ob der Pfad auf die Guideline-ID im Kontext zeigt.
func inGuideline(path []xml.Name) bool {
	n := len(path)
	return n == 4 &&
		path[1] == xml.Name{Space: NSCII, Local: "ExchangedDocumentContext"} &&
		path[2] == xml.Name{Space: NSRAM, Local: "GuidelineSpecifiedDocumentContextParameter"} &&
		path[3] == xml.Name{Space: NSRAM, Local: "ID"}
}

// Guidelines liefert die bekannten Guideline-IDs sortiert.
func Guidelines() []string {
	ids := make([]string, 0, len(levels))
	for id := range levels {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Attachment liefert den Anhang mit dem XML. rel überschreibt die Vorgabe
// der Stufe, wenn gesetzt.
func (i *Info) Attachment(data []byte, rel areatree.Relationship) areatree.Attachment {
	if rel == "" {
		rel = i.Level.Relationship
	}
	return areatree.Attachment{
		Name: i.Level.FileName, MIMEType: "text/xml", Relationship: rel,
		Description: "Rechnungsdaten im Format Factur-X/ZUGFeRD, Stufe " + i.Level.Name,
		Data:        data,
	}
}

// XMP liefert das Schema fx mit Beschreibung für pdfaExtension.
func (i *Info) XMP() areatree.XMPSchema {
	return areatree.XMPSchema{
		Prefix: "fx", NamespaceURI: NSXMP, Description: "Factur-X PDFA Extension Schema",
		Properties: []areatree.XMPProperty{
			{Name: "DocumentFileName", Value: i.Level.FileName, Description: "name of the embedded XML invoice file"},
			{Name: "DocumentType", Value: "INVOICE", Description: "INVOICE"},
			{Name: "Version", Value: "1.0", Description: "The actual version of the ZUGFeRD XML schema"},
			{Name: "ConformanceLevel", Value: i.Level.Name, Description: "The selected ZUGFeRD profile completeness"},
		},
	}
}
