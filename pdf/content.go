package pdf

import (
	"bytes"
	"strconv"
)

// Content builds a content stream. All numbers go through FormatNumber
// so that the output stays deterministic. Coordinates are PDF coordinates
// in points with the origin at the bottom left.
type Content struct {
	b   bytes.Buffer
	ops int
}

// Ops returns the number of operators written so far. It matches the
// count of veraPDF in the object context (operators[n]).
func (c *Content) Ops() int { return c.ops }

// Bytes returns the uncompressed stream content.
func (c *Content) Bytes() []byte { return c.b.Bytes() }

// Len returns the length of the content so far.
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

// Save writes q.
func (c *Content) Save() { c.op("q") }

// Restore writes Q.
func (c *Content) Restore() { c.op("Q") }

// Transform writes cm.
func (c *Content) Transform(a, b, cc, d, e, f float64) { c.nums(a, b, cc, d, e, f); c.op("cm") }

// LineWidth writes w.
func (c *Content) LineWidth(w float64) { c.nums(w); c.op("w") }

// Dash writes d. An empty pattern sets solid lines.
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

// LineCap writes J.
func (c *Content) LineCap(v int) { c.b.WriteString(strconv.Itoa(v)); c.op(" J") }

// FillRGB writes rg.
func (c *Content) FillRGB(r, g, b float64) { c.nums(r, g, b); c.op("rg") }

// StrokeRGB writes RG.
func (c *Content) StrokeRGB(r, g, b float64) { c.nums(r, g, b); c.op("RG") }

// FillGray writes g.
func (c *Content) FillGray(v float64) { c.nums(v); c.op("g") }

// StrokeGray writes G.
func (c *Content) StrokeGray(v float64) { c.nums(v); c.op("G") }

// MoveTo writes m.
func (c *Content) MoveTo(x, y float64) { c.nums(x, y); c.op("m") }

// LineTo writes l.
func (c *Content) LineTo(x, y float64) { c.nums(x, y); c.op("l") }

// Rect writes re.
func (c *Content) Rect(x, y, w, h float64) { c.nums(x, y, w, h); c.op("re") }

// CurveTo writes c, a cubic Bézier curve.
func (c *Content) CurveTo(x1, y1, x2, y2, x3, y3 float64) {
	c.nums(x1, y1, x2, y2, x3, y3)
	c.op("c")
}

// ClosePath writes h.
func (c *Content) ClosePath() { c.op("h") }

// BeginTag begins a marked-content sequence without properties: /Tag BMC.
// Form fields enclose their variable text with /Tx BMC
// (ISO 32000-1 12.7.3.3).
func (c *Content) BeginTag(tag Name) {
	var b bytes.Buffer
	tag.writePDF(&b)
	b.WriteString(" BMC")
	c.op(b.String())
}

// Fill writes f.
func (c *Content) Fill() { c.op("f") }

// Stroke writes S.
func (c *Content) Stroke() { c.op("S") }

// Clip writes W n.
func (c *Content) Clip() { c.op("W n") }

// ExtGState writes gs.
func (c *Content) ExtGState(name Name) { name.writePDF(&c.b); c.op(" gs") }

// XObject writes Do.
func (c *Content) XObject(name Name) { name.writePDF(&c.b); c.op(" Do") }

// BeginText writes BT.
func (c *Content) BeginText() { c.op("BT") }

// EndText writes ET.
func (c *Content) EndText() { c.op("ET") }

// Font writes Tf.
func (c *Content) Font(name Name, size float64) {
	name.writePDF(&c.b)
	c.b.WriteByte(' ')
	c.nums(size)
	c.op("Tf")
}

// TextMatrix writes Tm.
func (c *Content) TextMatrix(a, b, cc, d, e, f float64) { c.nums(a, b, cc, d, e, f); c.op("Tm") }

// TextRise writes Ts.
func (c *Content) TextRise(v float64) { c.nums(v); c.op("Ts") }

// ShowGlyphs writes TJ for two-byte glyph IDs (Identity-H). adjust[i]
// is the adjustment before glyph i in thousandths of the font size, positive
// to the left as usual in TJ. adjust may be nil.
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

// BeginMarked begins a marked-content sequence with a structure reference:
// /Tag <</MCID n>> BDC (ISO 32000-1 14.6, 14.7.4).
func (c *Content) BeginMarked(tag Name, mcid int) {
	var b bytes.Buffer
	tag.writePDF(&b)
	b.WriteString(" <</MCID ")
	b.WriteString(strconv.Itoa(mcid))
	b.WriteString(">> BDC")
	c.op(b.String())
}

// BeginMarkedText begins marked content with MCID and replacement text:
// /Tag <</MCID n /ActualText (…)>> BDC (ISO 32000-1 14.9.4). When read
// aloud, the replacement text replaces everything the sequence draws.
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

// BeginArtifact begins an artifact section: /Artifact BMC (ISO 32000-1
// 14.8.2.2). Artifacts are not part of the structure tree.
func (c *Content) BeginArtifact() { c.op("/Artifact BMC") }

// BeginActualText begins a span with replacement text:
// /Span <</ActualText …>> BDC (ISO 32000-1 14.9.4). It lies within
// marked content and carries no MCID.
func (c *Content) BeginActualText(text string) {
	var b bytes.Buffer
	b.WriteString("/Span <</ActualText ")
	TextString(text).writePDF(&b)
	b.WriteString(">> BDC")
	c.op(b.String())
}

// EndMarked ends a marked-content sequence: EMC.
func (c *Content) EndMarked() { c.op("EMC") }

// ShowBytes writes Tj with single-byte codes of a simple font as a
// hex string.
func (c *Content) ShowBytes(codes []byte) {
	const hex = "0123456789ABCDEF"
	c.b.WriteByte('<')
	for _, b := range codes {
		c.b.WriteByte(hex[b>>4])
		c.b.WriteByte(hex[b&15])
	}
	c.op("> Tj")
}
