package render

import (
	"fmt"
	"math"
	"strconv"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/font"
	"github.com/tim-riep/pdx/pdf"
)

// fontSet collects the glyph usage of all pages and embeds each font
// once as a subset.
type fontSet struct {
	r     *renderer
	usage map[areatree.FontID]*font.Usage
	emb   map[areatree.FontID]*font.Embedded
}

func newFontSet(r *renderer) *fontSet {
	return &fontSet{r: r, usage: map[areatree.FontID]*font.Usage{}, emb: map[areatree.FontID]*font.Embedded{}}
}

// collect walks through all pages and records every used glyph.
func (fs *fontSet) collect() error {
	for _, p := range fs.r.doc.Pages {
		var runs []*areatree.GlyphRun
		areatree.Walk(p.Areas, func(a areatree.Area) bool {
			switch v := a.(type) {
			case *areatree.GlyphRun:
				runs = append(runs, v)
			case *areatree.Widget:
				// The glyphs of the appearances belong in the same
				// subset as the page text.
				runs = append(runs, v.Runs...)
			}
			return true
		})
		for _, g := range runs {
			u, err := fs.usageFor(g.Font)
			if err != nil {
				return err
			}
			for _, gl := range g.Glyphs {
				if gl.ID == 0 {
					return fmt.Errorf("Seite %d: Glyph-Run nutzt die .notdef-Glyphe", p.Number)
				}
				u.Add(gl.ID, gl.Text)
			}
		}
	}
	return nil
}

func (fs *fontSet) usageFor(id areatree.FontID) (*font.Usage, error) {
	if u, ok := fs.usage[id]; ok {
		return u, nil
	}
	if int(id) < 0 || int(id) >= len(fs.r.doc.Fonts) {
		return nil, fmt.Errorf("Schrift %d ist im Dokument nicht deklariert", id)
	}
	face, ok := fs.r.doc.Fonts[id].Source.(*font.Face)
	if !ok {
		return nil, fmt.Errorf("Schrift %d hat keine ladbare Schriftquelle", id)
	}
	u := font.NewUsage(face)
	fs.usage[id] = u
	return u, nil
}

// embed embeds the fonts in the order of their FontID.
func (fs *fontSet) embed() error {
	for id := range fs.r.doc.Fonts {
		u, ok := fs.usage[areatree.FontID(id)]
		if !ok {
			continue
		}
		e, err := font.Embed(fs.r.d, u)
		if err != nil {
			return err
		}
		fs.emb[areatree.FontID(id)] = e
	}
	return nil
}

func fontResName(id areatree.FontID) pdf.Name { return pdf.Name("F" + strconv.Itoa(int(id))) }

// glyphs writes a glyph run. The advances of the area tree are reproduced
// exactly via TJ adjustments relative to the /W widths, vertical
// offsets via the text rise Ts.
func (pc *pageCtx) glyphs(g *areatree.GlyphRun) error {
	if len(g.Glyphs) == 0 {
		return nil
	}
	e, ok := pc.r.fonts.emb[g.Font]
	if !ok {
		return fmt.Errorf("Schrift %d wurde nicht eingebettet", g.Font)
	}
	name := fontResName(g.Font)
	pc.res.fonts.Set(name, e.Ref)

	size := g.Size.Pt()
	if size <= 0 {
		return fmt.Errorf("Schriftgröße muss positiv sein")
	}
	pc.c.Save()
	if err := pc.fill(g.Color); err != nil {
		return err
	}
	pc.c.BeginText()
	m, tagged := pc.r.tag.markOf(g)
	tagged = tagged && !pc.noTag
	if tagged {
		pc.begin(m)
	}
	pc.c.Font(name, size)
	pc.c.TextMatrix(1, 0, 0, 1, pc.x(g.Origin.X), pc.y(g.Origin.Y))

	k := 1000 / size
	carry := 0.0 // adjustment from the previous glyph
	rise := areatree.Length(0)
	var gids []uint16
	var adj []float64
	flush := func() {
		if len(gids) > 0 {
			pc.c.ShowGlyphs(gids, adj)
			gids, adj = gids[:0], adj[:0]
		}
	}
	span := false // span of the hyphen open
	for i, gl := range g.Glyphs {
		if tagged && m.hyphen && !m.artifact && i == len(g.Glyphs)-1 {
			// The hyphen from the line break stands for a soft
			// hyphen so that the word stays whole when read aloud and
			// copied (ISO 32000-1 14.8.2.2.3).
			flush()
			pc.c.BeginActualText("\u00ad")
			span = true
		}
		if gl.YOffset != rise {
			flush()
			rise = gl.YOffset
			pc.c.TextRise(-rise.Pt())
		}
		xo := gl.XOffset.Pt() * k
		gids = append(gids, e.CID[gl.ID])
		// Rounding remainders below half a thousandth of an em are
		// accumulated instead of written individually. This keeps the position
		// error below 0.0005 em and the content stream compact.
		a := carry - xo
		if math.Abs(a) < 0.5 {
			adj = append(adj, 0)
			carry = a
		} else {
			adj = append(adj, round3(a))
			carry = 0
		}
		carry += e.Width[gl.ID] - gl.Advance.Pt()*k + xo
	}
	flush()
	if span {
		pc.c.EndMarked()
	}
	if tagged {
		pc.c.EndMarked()
	}
	pc.c.EndText()
	pc.c.Restore()
	return nil
}

func round3(v float64) float64 {
	r := math.Round(v*1000) / 1000
	if r == 0 {
		return 0
	}
	return r
}
