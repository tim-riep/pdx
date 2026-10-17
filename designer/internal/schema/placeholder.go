package schema

import (
	"bytes"
	"encoding/xml"
	"sort"
	"strings"
)

// Placeholder erzeugt Daten für die Entwurfsansicht: Texte zeigen den Namen
// des Feldes, Zahlen und Daten einen passenden Beispielwert, wiederholte
// Knoten erscheinen dreimal, optionale einmal.
func (s *Schema) Placeholder() []byte {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	for i, r := range s.Roots {
		if i > 0 {
			break // ein Dokument hat eine Wurzel
		}
		s.write(&b, r, true, 0)
	}
	return b.Bytes()
}

// placeholderRepeat ist die Zahl der Einträge wiederholter Knoten.
const placeholderRepeat = 3

func (s *Schema) write(b *bytes.Buffer, n *Node, root bool, depth int) {
	count := 1
	if n.Repeated() {
		count = placeholderRepeat
		if n.Max > 1 && n.Max < count {
			count = n.Max
		}
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
				b.WriteString(" " + strings.TrimPrefix(c.Name, "@") + `="`)
				xml.EscapeText(b, []byte(sampleValue(c)))
				b.WriteString(`"`)
			} else {
				elems = append(elems, c)
			}
		}
		b.WriteString(">")
		if len(elems) == 0 && !n.Recursive {
			xml.EscapeText(b, []byte(sampleValue(n)))
		}
		for _, c := range elems {
			if !c.Recursive {
				s.write(b, c, false, depth+1)
			}
		}
		b.WriteString("</" + n.Name + ">")
	}
}

// sampleValue liefert einen Beispielwert passend zum Typ.
func sampleValue(n *Node) string {
	if len(n.Enum) > 0 {
		return n.Enum[0]
	}
	local := n.Name
	if i := strings.LastIndex(local, ":"); i >= 0 {
		local = local[i+1:]
	}
	local = strings.TrimPrefix(local, "@")
	// Bekannte Namen erhalten gültige Werte, etwa für Barcodes, die eine
	// prüfbare IBAN brauchen.
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

// knownValues sind Beispielwerte für häufige Feldnamen mit festem Format.
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
