// Package xmp erzeugt XMP-Metadatenpakete für PDF und PDF/A.
//
// Das Paket schreibt deterministisch und ohne die im xpacket-Kopf unter
// PDF/A verbotenen Attribute bytes und encoding. Eigene Namespaces werden
// unter PDF/A durch ein pdfaExtension-Schema beschrieben.
package xmp

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/tim-riep/pdx/areatree"
)

// Packet beschreibt den Inhalt eines XMP-Pakets.
type Packet struct {
	// PDFAPart und PDFAConformance setzen pdfaid, etwa 3 und "B". Part 0
	// lässt pdfaid weg.
	PDFAPart        int
	PDFAConformance string
	// UAPart setzt pdfuaid:part (ISO 14289-1 5), 0 lässt es weg.
	UAPart int

	Title, Author, Subject, Keywords, Creator, Producer string
	Created, Modified                                   time.Time

	// Schemas sind eigene Schemata mit Werten.
	Schemas []areatree.XMPSchema
	// Extension schreibt pdfaExtension-Beschreibungen für Schemas.
	Extension bool
}

// Namespaces der vordefinierten Schemata. Eigene Schemata dürfen sie nicht
// überschreiben.
var builtin = map[string]string{
	"dc":            "http://purl.org/dc/elements/1.1/",
	"pdf":           "http://ns.adobe.com/pdf/1.3/",
	"xmp":           "http://ns.adobe.com/xap/1.0/",
	"pdfaid":        "http://www.aiim.org/pdfa/ns/id/",
	"pdfuaid":       "http://www.aiim.org/pdfua/ns/id/",
	"pdfaExtension": "http://www.aiim.org/pdfa/ns/extension/",
	"pdfaSchema":    "http://www.aiim.org/pdfa/ns/schema#",
	"pdfaProperty":  "http://www.aiim.org/pdfa/ns/property#",
	"rdf":           "http://www.w3.org/1999/02/22-rdf-syntax-ns#",
	"x":             "adobe:ns:meta/",
	"xml":           "http://www.w3.org/XML/1998/namespace",
}

var reName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)

// Date formatiert einen Zeitpunkt im XMP-Format mit Zeitzone.
func Date(t time.Time) string { return t.Format("2006-01-02T15:04:05-07:00") }

func esc(s string) string {
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

// Bytes liefert das Paket als UTF-8.
func (p *Packet) Bytes() ([]byte, error) {
	for _, s := range p.Schemas {
		if !reName.MatchString(s.Prefix) {
			return nil, fmt.Errorf("XMP-Präfix %q ist ungültig", s.Prefix)
		}
		if uri, ok := builtin[s.Prefix]; ok && uri != s.NamespaceURI {
			return nil, fmt.Errorf("XMP-Präfix %q ist für %s reserviert", s.Prefix, uri)
		}
		if s.NamespaceURI == "" {
			return nil, fmt.Errorf("XMP-Schema %q ohne Namespace", s.Prefix)
		}
		for _, pr := range s.Properties {
			if !reName.MatchString(pr.Name) {
				return nil, fmt.Errorf("XMP-Eigenschaft %q ist ungültig", pr.Name)
			}
		}
	}

	var b bytes.Buffer
	b.WriteString("<?xpacket begin=\"\xef\xbb\xbf\" id=\"W5M0MpCehiHzreSzNTczkc9d\"?>\n")
	b.WriteString(`<x:xmpmeta xmlns:x="adobe:ns:meta/">` + "\n")
	b.WriteString(`<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">` + "\n")

	desc := func(ns string, body func()) {
		fmt.Fprintf(&b, "<rdf:Description rdf:about=\"\" %s>\n", ns)
		body()
		b.WriteString("</rdf:Description>\n")
	}
	alt := func(name, v string) {
		fmt.Fprintf(&b, "<%s><rdf:Alt><rdf:li xml:lang=\"x-default\">%s</rdf:li></rdf:Alt></%s>\n", name, esc(v), name)
	}
	simple := func(name, v string) {
		fmt.Fprintf(&b, "<%s>%s</%s>\n", name, esc(v), name)
	}

	if p.PDFAPart > 0 {
		desc(`xmlns:pdfaid="`+builtin["pdfaid"]+`"`, func() {
			simple("pdfaid:part", fmt.Sprint(p.PDFAPart))
			if p.PDFAConformance != "" {
				simple("pdfaid:conformance", p.PDFAConformance)
			}
		})
	}
	if p.UAPart > 0 {
		desc(`xmlns:pdfuaid="`+builtin["pdfuaid"]+`"`, func() {
			simple("pdfuaid:part", fmt.Sprint(p.UAPart))
		})
	}
	desc(`xmlns:dc="`+builtin["dc"]+`"`, func() {
		simple("dc:format", "application/pdf")
		if p.Title != "" {
			alt("dc:title", p.Title)
		}
		if p.Author != "" {
			fmt.Fprintf(&b, "<dc:creator><rdf:Seq><rdf:li>%s</rdf:li></rdf:Seq></dc:creator>\n", esc(p.Author))
		}
		if p.Subject != "" {
			alt("dc:description", p.Subject)
		}
	})
	desc(`xmlns:pdf="`+builtin["pdf"]+`"`, func() {
		simple("pdf:Producer", p.Producer)
		if p.Keywords != "" {
			simple("pdf:Keywords", p.Keywords)
		}
	})
	desc(`xmlns:xmp="`+builtin["xmp"]+`"`, func() {
		simple("xmp:CreateDate", Date(p.Created))
		simple("xmp:ModifyDate", Date(p.Modified))
		simple("xmp:MetadataDate", Date(p.Modified))
		if p.Creator != "" {
			simple("xmp:CreatorTool", p.Creator)
		}
	})
	for _, s := range p.Schemas {
		desc(fmt.Sprintf(`xmlns:%s="%s"`, s.Prefix, esc(s.NamespaceURI)), func() {
			for _, pr := range s.Properties {
				simple(s.Prefix+":"+pr.Name, pr.Value)
			}
		})
	}
	// pdfuaid ist in PDF/A nicht vordefiniert und braucht unter PDF/A eine
	// Beschreibung im pdfaExtension-Schema (ISO 19005-3 6.6.2.3.3).
	ext := p.Schemas
	if p.UAPart > 0 {
		ext = append([]areatree.XMPSchema{{
			Prefix: "pdfuaid", NamespaceURI: builtin["pdfuaid"], Description: "PDF/UA identification schema",
			Properties: []areatree.XMPProperty{{Name: "part", ValueType: "Integer", Description: "Indicates, which part of ISO 14289 standard is followed", Category: "internal"}},
		}}, ext...)
	}
	if p.Extension && len(ext) > 0 {
		desc(fmt.Sprintf(`xmlns:pdfaExtension="%s" xmlns:pdfaSchema="%s" xmlns:pdfaProperty="%s"`,
			builtin["pdfaExtension"], builtin["pdfaSchema"], builtin["pdfaProperty"]), func() {
			b.WriteString("<pdfaExtension:schemas><rdf:Bag>\n")
			for _, s := range ext {
				b.WriteString(`<rdf:li rdf:parseType="Resource">` + "\n")
				simple("pdfaSchema:schema", nonEmpty(s.Description, s.Prefix))
				simple("pdfaSchema:namespaceURI", s.NamespaceURI)
				simple("pdfaSchema:prefix", s.Prefix)
				b.WriteString("<pdfaSchema:property><rdf:Seq>\n")
				for _, pr := range s.Properties {
					b.WriteString(`<rdf:li rdf:parseType="Resource">`)
					simple("pdfaProperty:name", pr.Name)
					simple("pdfaProperty:valueType", nonEmpty(pr.ValueType, "Text"))
					simple("pdfaProperty:category", nonEmpty(pr.Category, "external"))
					simple("pdfaProperty:description", nonEmpty(pr.Description, pr.Name))
					b.WriteString("</rdf:li>\n")
				}
				b.WriteString("</rdf:Seq></pdfaSchema:property>\n</rdf:li>\n")
			}
			b.WriteString("</rdf:Bag></pdfaExtension:schemas>\n")
		})
	}
	b.WriteString("</rdf:RDF>\n</x:xmpmeta>\n")
	// Platz für nachträgliche Bearbeitung, wie von XMP empfohlen.
	pad := strings.Repeat(strings.Repeat(" ", 99)+"\n", 20)
	b.WriteString(pad)
	b.WriteString(`<?xpacket end="w"?>`)
	return b.Bytes(), nil
}

func nonEmpty(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
