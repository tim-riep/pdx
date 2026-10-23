package compile

import (
	"bytes"
	"strings"
)

// writer writes XML with fixed indentation and records on which line each
// template element begins. Attributes appear in the order in which the
// compiler passes them, which makes the output deterministic.
type writer struct {
	b     bytes.Buffer
	depth int
	line  int
	open  []string
	// srcmap maps line ranges in the XSLT to template IDs.
	srcmap []Mapping
}

func newWriter() *writer {
	w := &writer{line: 1}
	w.b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	return w
}

// The XSLT processor strips whitespace between the elements when reading the
// stylesheet, text is always in xsl:text. The indentation therefore has no
// effect on the generated FO.
func (w *writer) nl() {
	w.b.WriteByte('\n')
	w.line++
	w.b.WriteString(strings.Repeat("  ", w.depth))
}

// enter records that the template element id begins on the next line.
// leave closes the range with the current line.
func (w *writer) enter(id string) int {
	w.srcmap = append(w.srcmap, Mapping{Start: w.line + 1, End: w.line + 1, ID: id})
	return len(w.srcmap) - 1
}

func (w *writer) leave(i int) {
	if w.line > w.srcmap[i].End {
		w.srcmap[i].End = w.line
	}
}

// comment writes a comment. Double hyphens are forbidden in XML comments
// and are separated.
func (w *writer) comment(s string) {
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "- -")
	}
	w.nl()
	w.b.WriteString("<!-- ")
	w.b.WriteString(s)
	w.b.WriteString(" -->")
}

// start opens an element. attrs are name-value pairs, empty values are omitted.
func (w *writer) start(name string, attrs ...string) {
	w.nl()
	w.b.WriteByte('<')
	w.b.WriteString(name)
	w.attrs(attrs)
	w.b.WriteByte('>')
	w.open = append(w.open, name)
	w.depth++
}

// empty writes an empty element.
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

// end closes the most recently opened element.
func (w *writer) end() {
	name := w.open[len(w.open)-1]
	w.open = w.open[:len(w.open)-1]
	w.depth--
	w.nl()
	w.b.WriteString("</")
	w.b.WriteString(name)
	w.b.WriteByte('>')
}

// text writes text content as xsl:text so that whitespace is preserved.
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

// xpathString returns an XPath string literal with doubled
// apostrophes (XPath 2.0 and later).
func xpathString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// avtLiteral escapes curly braces for attribute value templates.
func avtLiteral(s string) string {
	s = strings.ReplaceAll(s, "{", "{{")
	return strings.ReplaceAll(s, "}", "}}")
}
