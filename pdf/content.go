package pdf

import (
	"bytes"
	"strconv"
)

// Content baut einen Content Stream. Alle Zahlen laufen über FormatNumber,
// damit die Ausgabe deterministisch bleibt. Koordinaten sind PDF-Koordinaten
// in Punkt mit Ursprung unten links.
type Content struct {
	b   bytes.Buffer
	ops int
}

// Ops liefert die Zahl der bisher geschriebenen Operatoren. Sie entspricht
// der Zählung von veraPDF im Objektkontext (operators[n]).
func (c *Content) Ops() int { return c.ops }

// Bytes liefert den unkomprimierten Stream-Inhalt.
func (c *Content) Bytes() []byte { return c.b.Bytes() }

// Len liefert die Länge des bisherigen Inhalts.
func (c *Content) Len() int { return c.b.Len() }

func (c *Content) nums(vs ...float64) {
	for _, v := range vs {
		c.b.WriteString(FormatNumber(v))
		c.b.WriteByte(' ')
	}
}

func (c *Content) op(s string) {
	c.b.WriteString(s)
	c.b.WriteByte('\n')
	if s == "W n" {
		c.ops += 2
	} else {
		c.ops++
	}
}

// Save schreibt q.
func (c *Content) Save() { c.op("q") }

// Restore schreibt Q.
func (c *Content) Restore() { c.op("Q") }

// Transform schreibt cm.
func (c *Content) Transform(a, b, cc, d, e, f float64) { c.nums(a, b, cc, d, e, f); c.op("cm") }

// LineWidth schreibt w.
func (c *Content) LineWidth(w float64) { c.nums(w); c.op("w") }

// Dash schreibt d. Ein leeres Muster setzt durchgezogene Linien.
func (c *Content) Dash(pattern []float64, phase float64) {
	c.b.WriteByte('[')
	for i, v := range pattern {
		if i > 0 {
			c.b.WriteByte(' ')
		}
		c.b.WriteString(FormatNumber(v))
	}
	c.b.WriteString("] ")
	c.nums(phase)
	c.op("d")
}

// LineCap schreibt J.
func (c *Content) LineCap(v int) { c.b.WriteString(strconv.Itoa(v)); c.op(" J") }

// FillRGB schreibt rg.
func (c *Content) FillRGB(r, g, b float64) { c.nums(r, g, b); c.op("rg") }

// StrokeRGB schreibt RG.
func (c *Content) StrokeRGB(r, g, b float64) { c.nums(r, g, b); c.op("RG") }

// FillGray schreibt g.
func (c *Content) FillGray(v float64) { c.nums(v); c.op("g") }

// StrokeGray schreibt G.
func (c *Content) StrokeGray(v float64) { c.nums(v); c.op("G") }

// MoveTo schreibt m.
func (c *Content) MoveTo(x, y float64) { c.nums(x, y); c.op("m") }

// LineTo schreibt l.
func (c *Content) LineTo(x, y float64) { c.nums(x, y); c.op("l") }

// Rect schreibt re.
func (c *Content) Rect(x, y, w, h float64) { c.nums(x, y, w, h); c.op("re") }

// CurveTo schreibt c, eine kubische Bézierkurve.
func (c *Content) CurveTo(x1, y1, x2, y2, x3, y3 float64) {
	c.nums(x1, y1, x2, y2, x3, y3)
	c.op("c")
}

// ClosePath schreibt h.
func (c *Content) ClosePath() { c.op("h") }

// BeginTag beginnt eine markierte Inhaltsfolge ohne Eigenschaften: /Tag BMC.
// Formularfelder umschließen ihren variablen Text mit /Tx BMC
// (ISO 32000-1 12.7.3.3).
func (c *Content) BeginTag(tag Name) {
	var b bytes.Buffer
	tag.writePDF(&b)
	b.WriteString(" BMC")
	c.op(b.String())
}

// Fill schreibt f.
func (c *Content) Fill() { c.op("f") }

// Stroke schreibt S.
func (c *Content) Stroke() { c.op("S") }

// Clip schreibt W n.
func (c *Content) Clip() { c.op("W n") }

// ExtGState schreibt gs.
func (c *Content) ExtGState(name Name) { name.writePDF(&c.b); c.op(" gs") }

// XObject schreibt Do.
func (c *Content) XObject(name Name) { name.writePDF(&c.b); c.op(" Do") }

// BeginText schreibt BT.
func (c *Content) BeginText() { c.op("BT") }

// EndText schreibt ET.
func (c *Content) EndText() { c.op("ET") }

// Font schreibt Tf.
func (c *Content) Font(name Name, size float64) {
	name.writePDF(&c.b)
	c.b.WriteByte(' ')
	c.nums(size)
	c.op("Tf")
}

// TextMatrix schreibt Tm.
func (c *Content) TextMatrix(a, b, cc, d, e, f float64) { c.nums(a, b, cc, d, e, f); c.op("Tm") }

// TextRise schreibt Ts.
func (c *Content) TextRise(v float64) { c.nums(v); c.op("Ts") }

// ShowGlyphs schreibt TJ für zweibyteige Glyph-IDs (Identity-H). adjust[i]
// ist die Korrektur vor Glyph i in Tausendstel der Schriftgröße, positiv nach
// links wie in TJ üblich. adjust darf nil sein.
func (c *Content) ShowGlyphs(gids []uint16, adjust []float64) {
	const hex = "0123456789ABCDEF"
	c.b.WriteByte('[')
	open := false
	for i, g := range gids {
		if adjust != nil && adjust[i] != 0 {
			if open {
				c.b.WriteString("> ")
				open = false
			}
			c.b.WriteString(FormatNumber(adjust[i]))
			c.b.WriteByte(' ')
		}
		if !open {
			c.b.WriteByte('<')
			open = true
		}
		c.b.WriteByte(hex[g>>12&15])
		c.b.WriteByte(hex[g>>8&15])
		c.b.WriteByte(hex[g>>4&15])
		c.b.WriteByte(hex[g&15])
	}
	if open {
		c.b.WriteByte('>')
	}
	c.op("] TJ")
}

// BeginMarked beginnt eine markierte Inhaltsfolge mit Strukturbezug:
// /Tag <</MCID n>> BDC (ISO 32000-1 14.6, 14.7.4).
func (c *Content) BeginMarked(tag Name, mcid int) {
	var b bytes.Buffer
	tag.writePDF(&b)
	b.WriteString(" <</MCID ")
	b.WriteString(strconv.Itoa(mcid))
	b.WriteString(">> BDC")
	c.op(b.String())
}

// BeginMarkedText beginnt markierten Inhalt mit MCID und Ersatztext:
// /Tag <</MCID n /ActualText (…)>> BDC (ISO 32000-1 14.9.4). Der Ersatztext
// ersetzt beim Vorlesen alles, was die Folge zeichnet.
func (c *Content) BeginMarkedText(tag Name, mcid int, actual string) {
	var b bytes.Buffer
	tag.writePDF(&b)
	b.WriteString(" <</MCID ")
	b.WriteString(strconv.Itoa(mcid))
	b.WriteString(" /ActualText ")
	TextString(actual).writePDF(&b)
	b.WriteString(">> BDC")
	c.op(b.String())
}

// BeginArtifact beginnt einen Artefakt-Bereich: /Artifact BMC (ISO 32000-1
// 14.8.2.2). Artefakte gehören nicht zum Strukturbaum.
func (c *Content) BeginArtifact() { c.op("/Artifact BMC") }

// BeginActualText beginnt eine Span mit Ersatztext:
// /Span <</ActualText …>> BDC (ISO 32000-1 14.9.4). Sie liegt innerhalb
// markierten Inhalts und trägt keine MCID.
func (c *Content) BeginActualText(text string) {
	var b bytes.Buffer
	b.WriteString("/Span <</ActualText ")
	TextString(text).writePDF(&b)
	b.WriteString(">> BDC")
	c.op(b.String())
}

// EndMarked beendet eine markierte Inhaltsfolge: EMC.
func (c *Content) EndMarked() { c.op("EMC") }
