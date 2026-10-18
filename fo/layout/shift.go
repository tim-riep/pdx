package layout

import (
	"strconv"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/fo/tree"
)

// shift liefert eine um dx, dy verschobene Kopie eines Bereichs.
func shift(a areatree.Area, dx, dy areatree.Length) areatree.Area {
	switch v := a.(type) {
	case *areatree.Container:
		c := *v
		c.X += dx
		c.Y += dy
		c.Children = make([]areatree.Area, len(v.Children))
		for i, ch := range v.Children {
			c.Children[i] = shift(ch, dx, dy)
		}
		return &c
	case *areatree.GlyphRun:
		g := *v
		g.Origin.X += dx
		g.Origin.Y += dy
		g.Glyphs = append([]areatree.Glyph(nil), v.Glyphs...)
		return &g
	case *areatree.Image:
		im := *v
		im.X += dx
		im.Y += dy
		return &im
	case *areatree.Line:
		l := *v
		l.From.X += dx
		l.From.Y += dy
		l.To.X += dx
		l.To.Y += dy
		return &l
	case *areatree.Path:
		pp := *v
		pp.X += dx
		pp.Y += dy
		pp.Rects = make([]areatree.Rect, len(v.Rects))
		for i, r := range v.Rects {
			r.X += dx
			r.Y += dy
			pp.Rects[i] = r
		}
		return &pp
	case *areatree.Widget:
		w := *v
		w.X += dx
		w.Y += dy
		w.Runs = make([]*areatree.GlyphRun, len(v.Runs))
		for i, r := range v.Runs {
			w.Runs[i] = shift(r, dx, dy).(*areatree.GlyphRun)
		}
		if v.Selected != nil {
			sel := *v.Selected
			sel.X += dx
			sel.Y += dy
			w.Selected = &sel
		}
		return &w
	case *areatree.Link:
		l := *v
		l.X += dx
		l.Y += dy
		return &l
	case *areatree.Box:
		b := *v
		b.X += dx
		b.Y += dy
		return &b
	}
	return a
}

func itoa(n int) string { return strconv.Itoa(n) }

func errorAt(n *tree.Node, err error) error {
	return tree.Errorf(n, "%v", err)
}
