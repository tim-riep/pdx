package compile

import (
	"bytes"
	"strings"
)

// writer schreibt XML mit fester Einrückung und merkt sich, in welcher Zeile
// welches Vorlagenelement beginnt. Attribute erscheinen in der Reihenfolge,
// in der der Compiler sie übergibt, das macht die Ausgabe deterministisch.
type writer struct {
	b     bytes.Buffer
	depth int
	line  int
	open  []string
	// srcmap ordnet Zeilenbereiche im XSLT den Vorlagen-IDs zu.
	srcmap []Mapping
}

func newWriter() *writer {
	w := &writer{line: 1}
	w.b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	return w
}

// Leerraum zwischen den Elementen entfernt der XSLT-Prozessor beim Lesen des
// Stylesheets, Text steht immer in xsl:text. Die Einrückung ist daher ohne
// Wirkung auf das erzeugte FO.
func (w *writer) nl() {
	w.b.WriteByte('\n')
	w.line++
	w.b.WriteString(strings.Repeat("  ", w.depth))
}

// enter vermerkt, dass ab der nächsten Zeile das Vorlagenelement id beginnt.
// leave schließt den Bereich mit der aktuellen Zeile ab.
func (w *writer) enter(id string) int {
	w.srcmap = append(w.srcmap, Mapping{Start: w.line + 1, End: w.line + 1, ID: id})
	return len(w.srcmap) - 1
}

func (w *writer) leave(i int) {
	if w.line > w.srcmap[i].End {
		w.srcmap[i].End = w.line
	}
}

// comment schreibt einen Kommentar. Doppelte Bindestriche sind in XML
// Kommentaren verboten und werden getrennt.
func (w *writer) comment(s string) {
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "- -")
	}
	w.nl()
	w.b.WriteString("<!-- ")
	w.b.WriteString(s)
	w.b.WriteString(" -->")
}

// start öffnet ein Element. attrs sind Name-Wert-Paare, leere Werte entfallen.
func (w *writer) start(name string, attrs ...string) {
	w.nl()
	w.b.WriteByte('<')
	w.b.WriteString(name)
	w.attrs(attrs)
	w.b.WriteByte('>')
	w.open = append(w.open, name)
	w.depth++
}

// empty schreibt ein leeres Element.
func (w *writer) empty(name string, attrs ...string) {
	w.nl()
	w.b.WriteByte('<')
	w.b.WriteString(name)
	w.attrs(attrs)
	w.b.WriteString("/>")
}

func (w *writer) attrs(attrs []string) {
	for i := 0; i+1 < len(attrs); i += 2 {
		if attrs[i+1] == "" {
			continue
		}
		w.b.WriteByte(' ')
		w.b.WriteString(attrs[i])
		w.b.WriteString(`="`)
		escape(&w.b, attrs[i+1], true)
		w.b.WriteByte('"')
	}
}

// end schließt das zuletzt geöffnete Element.
func (w *writer) end() {
	name := w.open[len(w.open)-1]
	w.open = w.open[:len(w.open)-1]
	w.depth--
	w.nl()
	w.b.WriteString("</")
	w.b.WriteString(name)
	w.b.WriteByte('>')
}

// text schreibt Textinhalt als xsl:text, damit Leerraum erhalten bleibt.
func (w *writer) text(s string) {
	if s == "" {
		return
	}
	w.nl()
	w.b.WriteString("<xsl:text>")
	escape(&w.b, s, false)
	w.b.WriteString("</xsl:text>")
}

func escape(b *bytes.Buffer, s string, attr bool) {
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			if attr {
				b.WriteString("&quot;")
			} else {
				b.WriteRune(r)
			}
		case '\n':
			if attr {
				b.WriteString("&#10;")
			} else {
				b.WriteRune(r)
			}
		default:
			b.WriteRune(r)
		}
	}
}

func (w *writer) bytes() []byte {
	out := append([]byte(nil), w.b.Bytes()...)
	return append(out, '\n')
}

// xpathString liefert einen XPath-Stringliteral mit verdoppelten
// Apostrophen (XPath 2.0 und neuer).
func xpathString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// avtLiteral maskiert geschweifte Klammern für Attributwert-Templates.
func avtLiteral(s string) string {
	s = strings.ReplaceAll(s, "{", "{{")
	return strings.ReplaceAll(s, "}", "}}")
}
