package font

import (
	"fmt"

	"github.com/boxesandglue/textshape/ot"
	"github.com/boxesandglue/textshape/subset"
	"golang.org/x/text/encoding/charmap"

	"github.com/tim-riep/pdx/pdf"
)

// Simple is a simple TrueType font with WinAnsiEncoding.
type Simple struct {
	Ref      pdf.Ref
	BaseFont string
}

// EmbedWinAnsi embeds a simple TrueType font with WinAnsiEncoding
// whose subset contains all WinAnsi characters that the font
// has. Form fields name it as their default font (DR, DA) so that a
// viewer can also display typed text (ISO 32000-1 12.7.3.3).
// For non-symbolic TrueType fonts PDF/A requires WinAnsiEncoding
// or MacRomanEncoding and a (3,1) cmap in the font program
// (ISO 19005-2 6.2.11.6). CFF fonts are not supported.
func EmbedWinAnsi(d *pdf.Document, f *Face) (*Simple, error) {
	if !f.Embeddable() {
		return nil, fmt.Errorf("Schrift %s darf laut Lizenz (OS/2 fsType) nicht eingebettet werden", f.PostScript)
	}
	if f.CFF {
		return nil, fmt.Errorf("Schrift %s hat CFF-Konturen, Formularfelder brauchen eine TrueType-Schrift", f.PostScript)
	}
	in := subset.NewInput()
	in.Flags |= subset.FlagNoLayoutClosure | subset.FlagDropLayoutTables
	in.AddGlyph(0)
	var gids []uint16
	codes := make([]rune, 256) // character per code, 0 without glyph
	for c := 32; c < 256; c++ {
		r := charmap.Windows1252.DecodeByte(byte(c))
		if r == 0xFFFD {
			continue
		}
		g, ok := f.GlyphFor(r)
		if !ok {
			continue
		}
		codes[c] = r
		in.AddUnicode(r)
		in.AddGlyph(ot.GlyphID(g))
		gids = append(gids, g)
	}
	plan, err := subset.CreatePlan(f.font, in)
	if err != nil {
		return nil, fmt.Errorf("Teilmenge von %s: %w", f.PostScript, err)
	}
	program, err := plan.Execute()
	if err != nil {
		return nil, fmt.Errorf("Teilmenge von %s: %w", f.PostScript, err)
	}
	base := subsetTag(f, gids) + "+" + f.PostScript
	var widths pdf.Array
	for c := 32; c < 256; c++ {
		if codes[c] == 0 {
			widths = append(widths, pdf.Int(0))
			continue
		}
		g, _ := f.GlyphFor(codes[c])
		widths = append(widths, pdf.Real(f.scale1000(f.Advance(g))))
	}
	flags := 32 // non-symbolic
	if f.FixedPitch {
		flags |= 1
	}
	if f.Italic {
		flags |= 64
	}
	file := d.Add(pdf.NewFlateStream(pdf.D("Length1", pdf.Int(len(program))), program))
	desc := pdf.D(
		"Type", pdf.Name("FontDescriptor"),
		"FontName", pdf.Name(base),
		"Flags", pdf.Int(flags),
		"FontBBox", pdf.Array{f.r(f.BBox[0]), f.r(f.BBox[1]), f.r(f.BBox[2]), f.r(f.BBox[3])},
		"ItalicAngle", pdf.Real(f.ItalicAngle),
		"Ascent", f.r(f.Ascender),
		"Descent", f.r(f.Descender),
		"CapHeight", f.r(f.CapHeight),
		"StemV", pdf.Int(stemV(f.Weight)),
		"FontFile2", file,
	)
	ref := d.Add(pdf.D(
		"Type", pdf.Name("Font"),
		"Subtype", pdf.Name("TrueType"),
		"BaseFont", pdf.Name(base),
		"FirstChar", pdf.Int(32),
		"LastChar", pdf.Int(255),
		"Widths", widths,
		"Encoding", pdf.Name("WinAnsiEncoding"),
		"FontDescriptor", d.Add(desc),
	))
	return &Simple{Ref: ref, BaseFont: base}, nil
}
