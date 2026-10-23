package render

import (
	"fmt"
	"math"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/pdf"
)

// pageCtx draws the areas of a page into a content stream.
type pageCtx struct {
	r      *renderer
	res    *pageRes
	c      pdf.Content
	h      float64 // page height in points for converting the y axis
	annots pdf.Array
	// trace and id are set when a trace is requested: the areas
	// of the page and the template ID of the current container.
	trace *PageTrace
	id    string
	// ox and oy shift the origin, for example for appearances of
	// form fields that draw in the coordinates of their widget. noTag
	// switches off marking for Tagged PDF.
	ox, oy areatree.Length
	noTag  bool
}

// areaID returns the template ID of an area or that of the container.
func (pc *pageCtx) areaID(a areatree.Area) string {
	var id string
	switch v := a.(type) {
	case *areatree.Container:
		id = v.ID
	case *areatree.GlyphRun:
		id = v.ID
	case *areatree.Image:
		id = v.ID
	case *areatree.Path:
		id = v.ID
	}
	if id == "" {
		return pc.id
	}
	return id
}

func (pc *pageCtx) x(l areatree.Length) float64 { return (l - pc.ox).Pt() }
func (pc *pageCtx) y(l areatree.Length) float64 { return pc.h - (l - pc.oy).Pt() }

// box converts a rectangle into a PDF array [llx lly urx ury].
func (pc *pageCtx) box(r areatree.Rect) pdf.Array {
	return pdf.Array{pdf.Real(pc.x(r.X)), pdf.Real(pc.y(r.Y + r.H)), pdf.Real(pc.x(r.X + r.W)), pdf.Real(pc.y(r.Y))}
}

// rect writes re for a rectangle of the area tree.
func (pc *pageCtx) rect(r areatree.Rect) {
	pc.c.Rect(pc.x(r.X), pc.y(r.Y+r.H), r.W.Pt(), r.H.Pt())
}

func (pc *pageCtx) fill(c areatree.Color) error {
	switch c.Space {
	case areatree.Gray:
		pc.c.FillGray(c.C[0])
	case areatree.RGB:
		pc.c.FillRGB(c.C[0], c.C[1], c.C[2])
	default:
		return fmt.Errorf("Farbraum CMYK wird in Phase 0 nicht ausgegeben")
	}
	return nil
}

func (pc *pageCtx) stroke(c areatree.Color) error {
	switch c.Space {
	case areatree.Gray:
		pc.c.StrokeGray(c.C[0])
	case areatree.RGB:
		pc.c.StrokeRGB(c.C[0], c.C[1], c.C[2])
	default:
		return fmt.Errorf("Farbraum CMYK wird in Phase 0 nicht ausgegeben")
	}
	return nil
}

func (pc *pageCtx) area(a areatree.Area) error {
	if pc.trace == nil {
		return pc.draw(a)
	}
	outer := pc.id
	pc.id = pc.areaID(a)
	defer func() { pc.id = outer }()
	first := len(pc.trace.Ops)
	start := pc.c.Ops()
	if pc.id != "" {
		// Placeholder so that the area comes before its children.
		pc.trace.Ops = append(pc.trace.Ops, OpRange{First: start, ID: pc.id})
	}
	err := pc.draw(a)
	if pc.id != "" {
		if end := pc.c.Ops(); end > start {
			pc.trace.Ops[first].Last = end
		} else {
			pc.trace.Ops = append(pc.trace.Ops[:first], pc.trace.Ops[first+1:]...)
		}
	}
	return err
}

func (pc *pageCtx) draw(a areatree.Area) error {
	// In Tagged PDF every leaf area is marked as content or artifact.
	// Containers only mark background and borders themselves, text
	// marks within the text object because of the hyphens.
	if t := pc.r.tag; t != nil && !pc.noTag {
		switch a.(type) {
		case *areatree.Container, *areatree.GlyphRun, *areatree.Link, *areatree.Widget:
		default:
			m := t.marks[a]
			pc.begin(m)
			defer pc.c.EndMarked()
		}
	}
	switch v := a.(type) {
	case *areatree.Container:
		return pc.container(v)
	case *areatree.GlyphRun:
		return pc.glyphs(v)
	case *areatree.Image:
		return pc.image(v)
	case *areatree.Line:
		return pc.line(v.From, v.To, areatree.Border{Width: v.Width, Style: v.Style, Color: v.Color})
	case *areatree.Box:
		return pc.boxArea(v)
	case *areatree.Link:
		return pc.link(v)
	case *areatree.Widget:
		return pc.widget(v)
	case *areatree.Path:
		return pc.path(v)
	}
	return fmt.Errorf("unbekannter Bereich %T", a)
}

// begin opens the marked-content sequence of an area.
func (pc *pageCtx) begin(m mark) {
	if m.artifact {
		pc.c.BeginArtifact()
		return
	}
	pc.c.BeginMarked(m.tag, m.mcid)
}

func (pc *pageCtx) container(c *areatree.Container) error {
	decor := pc.r.tag != nil && (c.Background != nil || hasBorder(c.Border))
	if decor {
		pc.c.BeginArtifact()
	}
	if c.Background != nil {
		pc.c.Save()
		if err := pc.fill(*c.Background); err != nil {
			return err
		}
		pc.rect(c.Rect)
		pc.c.Fill()
		pc.c.Restore()
	}
	if err := pc.borders(c.Rect, c.Border); err != nil {
		return err
	}
	if decor {
		pc.c.EndMarked()
	}
	clip := c.Kind == areatree.KindViewport
	if clip {
		pc.c.Save()
		pc.rect(c.Rect)
		pc.c.Clip()
	}
	for _, ch := range c.Children {
		if err := pc.area(ch); err != nil {
			return err
		}
	}
	if clip {
		pc.c.Restore()
	}
	return nil
}

// borders draws the four edges outside the rectangle r. Solid
// edges are drawn as filled trapezoids so that the corners meet
// cleanly. Other styles run as a line through the middle of the edge.
func (pc *pageCtx) borders(r areatree.Rect, b [4]areatree.Border) error {
	t, rt, bt, l := b[areatree.Top], b[areatree.Right], b[areatree.Bottom], b[areatree.Left]
	w := func(e areatree.Border) areatree.Length {
		if e.Visible() {
			return e.Width
		}
		return 0
	}
	// Outer and inner corners top left, top right, bottom right, bottom left.
	ox0, oy0 := r.X-w(l), r.Y-w(t)
	ox1, oy1 := r.X+r.W+w(rt), r.Y+r.H+w(bt)
	ix0, iy0 := r.X, r.Y
	ix1, iy1 := r.X+r.W, r.Y+r.H
	type pt = areatree.Point
	P := func(x, y areatree.Length) pt { return pt{X: x, Y: y} }
	sides := []struct {
		e    areatree.Border
		quad [4]pt
		from pt
		to   pt
	}{
		{t, [4]pt{P(ox0, oy0), P(ox1, oy0), P(ix1, iy0), P(ix0, iy0)}, P(ox0, oy0+w(t)/2), P(ox1, oy0+w(t)/2)},
		{rt, [4]pt{P(ox1, oy0), P(ox1, oy1), P(ix1, iy1), P(ix1, iy0)}, P(ox1-w(rt)/2, oy0), P(ox1-w(rt)/2, oy1)},
		{bt, [4]pt{P(ox1, oy1), P(ox0, oy1), P(ix0, iy1), P(ix1, iy1)}, P(ox1, oy1-w(bt)/2), P(ox0, oy1-w(bt)/2)},
		{l, [4]pt{P(ox0, oy1), P(ox0, oy0), P(ix0, iy0), P(ix0, iy1)}, P(ox0+w(l)/2, oy1), P(ox0+w(l)/2, oy0)},
	}
	for _, s := range sides {
		if !s.e.Visible() {
			continue
		}
		if s.e.Style != areatree.Solid {
			if err := pc.line(s.from, s.to, s.e); err != nil {
				return err
			}
			continue
		}
		pc.c.Save()
		if err := pc.fill(s.e.Color); err != nil {
			return err
		}
		pc.c.MoveTo(pc.x(s.quad[0].X), pc.y(s.quad[0].Y))
		for _, p := range s.quad[1:] {
			pc.c.LineTo(pc.x(p.X), pc.y(p.Y))
		}
		pc.c.Fill()
		pc.c.Restore()
	}
	return nil
}

// line draws a line in the given style.
func (pc *pageCtx) line(from, to areatree.Point, b areatree.Border) error {
	if !b.Visible() {
		return nil
	}
	pc.c.Save()
	if err := pc.stroke(b.Color); err != nil {
		return err
	}
	wd := b.Width.Pt()
	switch b.Style {
	case areatree.Dashed:
		pc.c.Dash([]float64{3 * wd, 3 * wd}, 0)
	case areatree.Dotted:
		pc.c.LineCap(1)
		pc.c.Dash([]float64{0, 2 * wd}, 0)
	}
	if b.Style == areatree.Double {
		// Two lines with one third of the width each, gap one third.
		dx, dy := to.Y-from.Y, from.X-to.X // normal, only direction matters
		n := normLen(dx, dy)
		third := wd / 3
		pc.c.LineWidth(third)
		for _, s := range []float64{-third, third} {
			ox, oy := float64(dx)/n*s, float64(dy)/n*s
			pc.c.MoveTo(pc.x(from.X)+ox, pc.y(from.Y)-oy)
			pc.c.LineTo(pc.x(to.X)+ox, pc.y(to.Y)-oy)
		}
		pc.c.Stroke()
		pc.c.Restore()
		return nil
	}
	pc.c.LineWidth(wd)
	pc.c.MoveTo(pc.x(from.X), pc.y(from.Y))
	pc.c.LineTo(pc.x(to.X), pc.y(to.Y))
	pc.c.Stroke()
	pc.c.Restore()
	return nil
}

func normLen(dx, dy areatree.Length) float64 {
	n := math.Hypot(float64(dx), float64(dy))
	if n == 0 {
		return 1
	}
	return n
}

func (pc *pageCtx) boxArea(b *areatree.Box) error {
	if b.Fill != nil {
		pc.c.Save()
		if err := pc.fill(*b.Fill); err != nil {
			return err
		}
		pc.rect(b.Rect)
		pc.c.Fill()
		pc.c.Restore()
	}
	if b.Stroke != nil && b.Stroke.Visible() {
		pc.c.Save()
		if err := pc.stroke(b.Stroke.Color); err != nil {
			return err
		}
		pc.c.LineWidth(b.Stroke.Width.Pt())
		pc.rect(b.Rect)
		pc.c.Stroke()
		pc.c.Restore()
	}
	return nil
}

// path fills all rectangles of a path in one go.
func (pc *pageCtx) path(p *areatree.Path) error {
	if len(p.Rects) == 0 {
		return nil
	}
	pc.c.Save()
	if err := pc.fill(p.Fill); err != nil {
		return err
	}
	for _, r := range p.Rects {
		pc.rect(r)
	}
	pc.c.Fill()
	pc.c.Restore()
	return nil
}
