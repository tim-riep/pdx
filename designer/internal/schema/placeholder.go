package schema

import (
	"bytes"
	"encoding/xml"
	"sort"
	"strings"
)

// Placeholder generates data for the design view: texts show the name
// of the field, numbers and dates a matching sample value, repeated
// nodes appear three times, optional ones once.
func (s *Schema) Placeholder() []byte {
	return s.Generate(Gen{Repeat: placeholderRepeat, Optional: true})
}

// placeholderRepeat is the number of entries of repeated nodes.
const placeholderRepeat = 3

// Gen controls the generation of data, for example for edge cases.
type Gen struct {
	// Repeat is the number of entries of repeated nodes, at most as
	// many as the schema allows. 0 takes the smallest allowed number.
	Repeat int
	// Optional includes optional nodes, otherwise they are left out.
	Optional bool
	// LongText makes texts long to test line breaks and overflows.
	LongText bool
}

// Generate creates a data set according to the schema.
func (s *Schema) Generate(g Gen) []byte {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	for i, r := range s.Roots {
		if i > 0 {
			break // a document has one root
		}
		s.write(&b, r, true, 0, g)
	}
	return b.Bytes()
}

func (s *Schema) write(b *bytes.Buffer, n *Node, root bool, depth int, g Gen) {
	count := 1
	switch {
	case root:
	case n.Repeated():
		count = g.Repeat
		if count < n.Min {
			count = n.Min
		}
		if n.Max > 0 && count > n.Max {
			count = n.Max
		}
	case n.Min == 0 && !g.Optional:
		count = 0
	}
	for range count {
		b.WriteString("<" + n.Name)
		if root {
			uris := make([]string, 0, len(s.Prefixes))
			for uri := range s.Prefixes {
				uris = append(uris, uri)
			}
			sort.Strings(uris)
			for _, uri := range uris {
				b.WriteString(` xmlns:` + s.Prefixes[uri] + `="`)
				xml.EscapeText(b, []byte(uri))
				b.WriteString(`"`)
			}
		}
		var elems []*Node
		for _, c := range n.Children {
			if c.Attr {
				if c.Min == 0 && !g.Optional {
					continue
				}
				b.WriteString(" " + strings.TrimPrefix(c.Name, "@") + `="`)
				xml.EscapeText(b, []byte(value(c, g)))
				b.WriteString(`"`)
			} else {
				elems = append(elems, c)
			}
		}
		b.WriteString(">")
		if len(elems) == 0 && !n.Recursive {
			xml.EscapeText(b, []byte(value(n, g)))
		}
		for _, c := range elems {
			if !c.Recursive {
				s.write(b, c, false, depth+1, g)
			}
		}
		b.WriteString("</" + n.Name + ">")
	}
}

// longText is a long text with long words for edge cases.
const longText = "Donaudampfschifffahrtsgesellschaftskapitänsmütze und Rechnungsposition mit ausführlicher Leistungsbeschreibung über mehrere Zeilen, damit Umbrüche, Silbentrennung und Überläufe sichtbar werden"

// value returns the value of a node, long texts only for texts without
// a fixed format.
func value(n *Node, g Gen) string {
	v := sampleValue(n)
	if g.LongText && len(n.Enum) == 0 && (n.Type == "string" || n.Type == "normalizedString" || n.Type == "token") {
		if _, known := knownValues[strings.ToLower(strings.TrimPrefix(n.Name[strings.LastIndex(n.Name, ":")+1:], "@"))]; !known {
			return v + " " + longText
		}
	}
	return v
}

// sampleValue returns a sample value matching the type.
func sampleValue(n *Node) string {
	if len(n.Enum) > 0 {
		return n.Enum[0]
	}
	local := n.Name
	if i := strings.LastIndex(local, ":"); i >= 0 {
		local = local[i+1:]
	}
	local = strings.TrimPrefix(local, "@")
	// Known names get valid values, for example for barcodes that need a
	// verifiable IBAN.
	if v, ok := knownValues[strings.ToLower(local)]; ok && (n.Type == "string" || n.Type == "token" || n.Type == "normalizedString") {
		return v
	}
	switch n.Type {
	case "decimal", "double", "float":
		return "1234.56"
	case "integer", "int", "long", "short", "byte", "nonNegativeInteger", "positiveInteger",
		"unsignedInt", "unsignedLong", "unsignedShort", "unsignedByte", "negativeInteger", "nonPositiveInteger":
		return "3"
	case "date":
		return "2026-01-15"
	case "dateTime":
		return "2026-01-15T09:00:00"
	case "time":
		return "09:00:00"
	case "gYear":
		return "2026"
	case "gYearMonth":
		return "2026-01"
	case "boolean":
		return "true"
	case "anyURI":
		return "https://example.org"
	case "base64Binary", "hexBinary":
		return ""
	case "":
		return ""
	}
	return local
}

// knownValues are sample values for common field names with a fixed format.
var knownValues = map[string]string{
	"iban":     "DE02120300000000202051",
	"bic":      "BYLADEM1001",
	"currency": "EUR",
	"country":  "DE",
	"postcode": "12345",
	"zip":      "12345",
	"email":    "name@example.org",
	"phone":    "+49 30 1234567",
	"unit":     "C62",
}
