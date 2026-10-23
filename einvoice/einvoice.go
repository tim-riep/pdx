// Package einvoice describes hybrid invoices according to Factur-X 1.0 and
// ZUGFeRD 2: detection of data in CII format, derivation of the level from
// the guideline ID, file name and relationship of the embedded XML, and the
// XMP schema fx.
//
// The values follow ZUGFeRD 2.5.2 and Factur-X 1.09.2 and are checked
// against the reference implementation Mustang (Apache 2.0): Profiles.java,
// XMPSchemaZugferd.java, XMPSchemaPDFAExtensions.java and
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

// Namespaces of the data.
const (
	NSCII = "urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100"
	NSRAM = "urn:un:unece:uncefact:data:standard:ReusableAggregateBusinessInformationEntity:100"
	// UBL is detected to give an understandable message.
	nsUBLInvoice    = "urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
	nsUBLCreditNote = "urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2"
)

// NSXMP is the namespace of the XMP schema fx (Factur-X 1.0).
const NSXMP = "urn:factur-x:pdfa:CrossIndustryDocument:invoice:1p0#"

// Level is a conformance level.
type Level struct {
	// Name is the value of fx:ConformanceLevel, such as "EN 16931".
	Name string
	// FileName is the name of the embedded XML.
	FileName string
	// Relationship is the default for AFRelationship. MINIMUM and BASIC WL
	// are not complete invoices, there the XML is Data, otherwise
	// Alternative (ZUGFeRD 2.1.1 Technischer Anhang Teil A 2.2.2).
	Relationship areatree.Relationship
}

// levels maps the guideline IDs (BT-24) to the levels.
var levels = map[string]Level{
	"urn:factur-x.eu:1p0:minimum":                                           {"MINIMUM", "factur-x.xml", areatree.Data},
	"urn:factur-x.eu:1p0:basicwl":                                           {"BASIC WL", "factur-x.xml", areatree.Data},
	"urn:cen.eu:en16931:2017#compliant#urn:factur-x.eu:1p0:basic":           {"BASIC", "factur-x.xml", areatree.Alternative},
	"urn:cen.eu:en16931:2017":                                               {"EN 16931", "factur-x.xml", areatree.Alternative},
	"urn:cen.eu:en16931:2017#conformant#urn:factur-x.eu:1p0:extended":       {"EXTENDED", "factur-x.xml", areatree.Alternative},
	"urn:cen.eu:en16931:2017#compliant#urn:xeinkauf.de:kosit:xrechnung_3.0": {"XRECHNUNG", "xrechnung.xml", areatree.Alternative},
	"urn:cen.eu:en16931:2017#compliant#urn:xeinkauf.de:kosit:xrechnung_3.0#conformant#urn:xeinkauf.de:kosit:extension:xrechnung_3.0": {"XRECHNUNG", "xrechnung.xml", areatree.Alternative},
}

// Info is the result of the detection.
type Info struct {
	Guideline string
	Level     Level
}

// XRechnung reports whether the data must additionally satisfy the rules
// of XRechnung.
func (i *Info) XRechnung() bool { return i.Level.Name == "XRECHNUNG" }

// Detect checks that the data is a CII invoice and derives the level from
// ram:GuidelineSpecifiedDocumentContextParameter/ram:ID.
func Detect(data []byte) (*Info, error) {
	d := xml.NewDecoder(bytes.NewReader(data))
	var root *xml.StartElement
	// Path of the open elements, to find the guideline ID.
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

// inGuideline reports whether the path points to the guideline ID in the context.
func inGuideline(path []xml.Name) bool {
	n := len(path)
	return n == 4 &&
		path[1] == xml.Name{Space: NSCII, Local: "ExchangedDocumentContext"} &&
		path[2] == xml.Name{Space: NSRAM, Local: "GuidelineSpecifiedDocumentContextParameter"} &&
		path[3] == xml.Name{Space: NSRAM, Local: "ID"}
}

// Guidelines returns the known guideline IDs, sorted.
func Guidelines() []string {
	ids := make([]string, 0, len(levels))
	for id := range levels {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Attachment returns the attachment with the XML. rel overrides the
// default of the level, if set.
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

// XMP returns the schema fx with the description for pdfaExtension.
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
