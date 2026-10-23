// Package xmp generates XMP metadata packets for PDF and PDF/A.
//
// The package writes deterministically and without the attributes bytes and
// encoding in the xpacket header, which PDF/A forbids. Custom namespaces are
// described under PDF/A by a pdfaExtension schema.
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

// Packet describes the content of an XMP packet.
type Packet struct {
	// PDFAPart and PDFAConformance set pdfaid, for example 3 and "B". Part 0
	// omits pdfaid.
	PDFAPart        int
	PDFAConformance string
	// UAPart sets pdfuaid:part (ISO 14289-1 5), 0 omits it.
	UAPart int
	// PDFARev and UARev set pdfaid:rev and pdfuaid:rev, empty omits
	// them. PDF/A-4 and PDF/UA-2 require them.
	PDFARev, UARev string

	Title, Author, Subject, Keywords, Creator, Producer string
	Created, Modified                                   time.Time

	// Schemas are custom schemas with values.
	Schemas []areatree.XMPSchema
	// Extension writes pdfaExtension descriptions for Schemas.
	Extension bool
}

// Namespaces of the predefined schemas. Custom schemas must not
// overwrite them.
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

// Date formats a point in time in XMP format with time zone.
func Date(t time.Time) string { return t.Format("2006-01-02T15:04:05-07:00") }

func esc(s string) string {
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

// Bytes returns the packet as UTF-8.
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
			if p.PDFARev != "" {
				simple("pdfaid:rev", p.PDFARev)
			}
		})
	}
	if p.UAPart > 0 {
		desc(`xmlns:pdfuaid="`+builtin["pdfuaid"]+`"`, func() {
			simple("pdfuaid:part", fmt.Sprint(p.UAPart))
			if p.UARev != "" {
				simple("pdfuaid:rev", p.UARev)
			}
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
	// pdfuaid is not predefined in PDF/A and needs a description in the
	// pdfaExtension schema under PDF/A (ISO 19005-3 6.6.2.3.3).
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
	// Room for later editing, as recommended by XMP.
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
