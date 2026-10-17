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
	return s.Generate(Gen{Repeat: placeholderRepeat, Optional: true})
}

// placeholderRepeat ist die Zahl der Einträge wiederholter Knoten.
const placeholderRepeat = 3

// Gen steuert das Erzeugen von Daten, etwa für Randfälle.
type Gen struct {
	// Repeat ist die Zahl der Einträge wiederholter Knoten, höchstens so
	// viele, wie das Schema erlaubt. 0 nimmt die kleinste erlaubte Zahl.
	Repeat int
	// Optional nimmt optionale Knoten auf, sonst entfallen sie.
	Optional bool
	// LongText macht Texte lang, um Umbrüche und Überläufe zu prüfen.
	LongText bool
}

// Generate erzeugt einen Datensatz nach dem Schema.
func (s *Schema) Generate(g Gen) []byte {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	for i, r := range s.Roots {
		if i > 0 {
			break // ein Dokument hat eine Wurzel
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

// longText ist ein langer Text mit langen Wörtern für Randfälle.
const longText = "Donaudampfschifffahrtsgesellschaftskapitänsmütze und Rechnungsposition mit ausführlicher Leistungsbeschreibung über mehrere Zeilen, damit Umbrüche, Silbentrennung und Überläufe sichtbar werden"

// value liefert den Wert eines Knotens, lange Texte nur bei Texten ohne
// festes Format.
func value(n *Node, g Gen) string {
	v := sampleValue(n)
	if g.LongText && len(n.Enum) == 0 && (n.Type == "string" || n.Type == "normalizedString" || n.Type == "token") {
		if _, known := knownValues[strings.ToLower(strings.TrimPrefix(n.Name[strings.LastIndex(n.Name, ":")+1:], "@"))]; !known {
			return v + " " + longText
		}
	}
	return v
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
