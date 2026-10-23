package sign

import (
	"math"

	"golang.org/x/text/encoding/charmap"

	"github.com/tim-riep/pdx/pdf"
	"github.com/tim-riep/pdx/pdf/reader"
)

// winAnsiFont is a font from the default resources DR of the form.
type winAnsiFont struct {
	ref    pdf.Ref
	first  int
	widths []float64
}

// drFont searches DR for an embedded simple font with
// WinAnsiEncoding. pdx creates it for forms (ISO 32000-1 12.7.3.3),
// PDF/A requires embedded fonts (ISO 19005-2 6.2.11.4), so no
// new font goes into the update.
func drFont(f *reader.File) *winAnsiFont {
	fonts := f.Dict(f.Dict(f.Dict(f.Root()["AcroForm"])["DR"])["Font"])
	for _, v := range fonts {
		r, ok := v.(reader.Ref)
		if !ok {
			continue
		}
		d := f.Dict(r)
		desc := f.Dict(d["FontDescriptor"])
		if f.Resolve(d["Encoding"]) != reader.Name("WinAnsiEncoding") || desc == nil ||
			(desc["FontFile2"] == nil && desc["FontFile"] == nil && desc["FontFile3"] == nil) {
			continue
		}
		first, _ := f.Int(d["FirstChar"])
		wf := &winAnsiFont{ref: pdf.Ref{Num: r.Num}, first: first}
		for _, w := range f.Array(d["Widths"]) {
			switch n := f.Resolve(w).(type) {
			case int64:
				wf.widths = append(wf.widths, float64(n))
			case float64:
				wf.widths = append(wf.widths, n)
			default:
				wf.widths = append(wf.widths, 0)
			}
		}
		return wf
	}
	return nil
}

// encode encodes text in WinAnsi, characters that cannot be represented become ?.
func (wf *winAnsiFont) encode(s string) []byte {
	var out []byte
	for _, r := range s {
		b, ok := charmap.Windows1252.EncodeRune(r)
		if !ok || wf.width(b) == 0 {
			b = '?'
		}
		out = append(out, b)
	}
	return out
}

func (wf *winAnsiFont) width(b byte) float64 {
	i := int(b) - wf.first
	if i < 0 || i >= len(wf.widths) {
		return 0
	}
	return wf.widths[i]
}

// signatureAppearance builds the appearance of a visible
// signature field: the previous background of the field and on top of it the
// lines in the font from DR, as large as fits, at most 10 pt. Without
// a suitable font or area it returns nil, the field keeps its image.
func signatureAppearance(f *reader.File, widget reader.Dict, lines []string) *pdf.Stream {
	rect := f.Array(widget["Rect"])
	if len(rect) != 4 {
		return nil
	}
	var r [4]float64
	for i, v := range rect {
		switch n := f.Resolve(v).(type) {
		case int64:
			r[i] = float64(n)
		case float64:
			r[i] = n
		}
	}
	w, h := math.Abs(r[2]-r[0]), math.Abs(r[3]-r[1])
	wf := drFont(f)
	if wf == nil || w < 8 || h < 8 || len(lines) == 0 {
		return nil
	}
	const pad, lead = 2.0, 1.2
	size := math.Min(10, (h-2*pad)/(float64(len(lines))*lead))
	codes := make([][]byte, len(lines))
	for i, l := range lines {
		codes[i] = wf.encode(l)
		var lw float64
		for _, b := range codes[i] {
			lw += wf.width(b)
		}
		if lw > 0 {
			size = math.Min(size, (w-2*pad)*1000/lw)
		}
	}
	res := pdf.D("Font", pdf.D("FS", wf.ref))
	c := &pdf.Content{}
	if bg, ok := f.Dict(widget["AP"])["N"].(reader.Ref); ok {
		res.Set("XObject", pdf.D("Bg", pdf.Ref{Num: bg.Num}))
		c.Save()
		c.XObject("Bg")
		c.Restore()
	}
	c.BeginText()
	c.Font("FS", size)
	c.FillGray(0)
	for i, b := range codes {
		c.TextMatrix(1, 0, 0, 1, pad, h-pad-size*(float64(i)+0.85)*lead)
		c.ShowBytes(b)
	}
	c.EndText()
	return pdf.NewFlateStream(pdf.D(
		"Type", pdf.Name("XObject"),
		"Subtype", pdf.Name("Form"),
		"BBox", pdf.Array{pdf.Int(0), pdf.Int(0), pdf.Real(w), pdf.Real(h)},
		"Resources", res,
	), c.Bytes())
}
