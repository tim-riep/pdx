package layout

import (
	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/font"
)

// placer places galley entries into a reference area of a page. It is
// used for pages of the flow, static content and absolute containers.
type placer struct {
	e       *engine
	page    *areatree.Page
	ref     areatree.Rect
	root    *areatree.Container
	y       areatree.Length
	started bool
	open    []*openFrag
	curRow  *rowFrag
	// seen maps areas with markers to their event on this page.
	seen map[any]*mkEvent
	// deferred are header rows with table markers that are only drawn after
	// the body rows of the page.
	deferred []deferredRow
}

// deferredRow is a row to be drawn later.
type deferredRow struct {
	r      *rowLayout
	y      areatree.Length
	parent *areatree.Container
	repeat bool // repeated header on a following page
}

// openFrag is the fragment of a block on the current page.
type openFrag struct {
	ctx *blockCtx
	c   *areatree.Container
	end areatree.Length // bottom edge of the last content so far
}

func newPlacer(e *engine, page *areatree.Page, ref areatree.Rect, root *areatree.Container) *placer {
	return &placer{e: e, page: page, ref: ref, root: root, y: ref.Y}
}

func (pl *placer) parent() *areatree.Container {
	if n := len(pl.open); n > 0 {
		return pl.open[n-1].c
	}
	return pl.root
}

// place places the entries and closes all open fragments at the end.
// If the page starts in the middle of a table body, the header is repeated,
// if it ends there, the footer.
func (pl *placer) place(items []item) error {
	if first := firstBox(items); first != nil && first.row != nil {
		if t := first.row.tbl; t != nil && first.row.sec == secBody && !t.omitHeader && len(t.header) > 0 &&
			!(first.row == t.firstBody && first.slice == 0) {
			pl.sync(first.path, edgeNone)
			if err := pl.drawRows(t.header); err != nil {
				return err
			}
			pl.started = true
		}
	}
	for _, it := range items {
		switch it.kind {
		case itPenalty:
		case itGlue:
			// At the start of a page only the retained part remains (XSL 1.1 4.3.1).
			if pl.started {
				pl.y += it.h
			} else {
				pl.y += it.keep
			}
		case itBox:
			if err := pl.box(it); err != nil {
				return err
			}
		}
	}
	if err := pl.flushRow(); err != nil {
		return err
	}
	if last := lastBox(items); last != nil && last.row != nil {
		if t := last.row.tbl; t != nil && last.row.sec == secBody && !t.omitFooter && len(t.footer) > 0 &&
			t.continuesAfter(last.row, last.slice) {
			if err := pl.drawRows(t.footer); err != nil {
				return err
			}
		}
	}
	// Headers with table markers now know the body rows of the page.
	for _, d := range pl.deferred {
		nr, err := d.r.relayout(pl.e, pl.page)
		if err != nil {
			return err
		}
		if err := pl.drawRowIn(d.parent, nr, 0, nr.h, d.y, rowRef{key: d.r, repeat: d.repeat}); err != nil {
			return err
		}
	}
	pl.deferred = nil
	pl.closeAll()
	return nil
}

func firstBox(items []item) *item {
	for i := range items {
		if items[i].kind == itBox {
			return &items[i]
		}
	}
	return nil
}

func lastBox(items []item) *item {
	for i := len(items) - 1; i >= 0; i-- {
		if items[i].kind == itBox {
			return &items[i]
		}
	}
	return nil
}

func (pl *placer) box(it item) error {
	if it.row == nil || pl.curRow == nil || pl.curRow.r != it.row || pl.curRow.s1 != it.slice {
		if err := pl.flushRow(); err != nil {
			return err
		}
	}
	pl.sync(it.path, it.edge)
	for _, ctx := range it.path {
		pl.note(ctx, ctx.markers, it.seq == ctx.first, it.seq == ctx.last)
	}
	folio := formatNumber(pl.page.Number, pl.e.format)
	for _, ctx := range it.path {
		if id, ok := ctx.node.Attr("id"); ok && id != "" {
			pl.e.recordID(id, folio)
			pl.e.recordDest(id, pl.page, areatree.Point{X: pl.ref.X + ctx.x, Y: pl.y}, ctx.node.Label())
		}
		for _, id := range ctx.ids {
			pl.e.recordID(id, folio)
			pl.e.recordDest(id, pl.page, areatree.Point{X: pl.ref.X + ctx.x, Y: pl.y}, ctx.node.Label())
		}
	}
	if it.line != nil {
		// Targets in a line point to the block of the line.
		src := ""
		if n := len(it.path); n > 0 {
			src = it.path[n-1].node.Label()
		}
		for _, id := range it.line.ids {
			pl.e.recordID(id, folio)
			pl.e.recordDest(id, pl.page, areatree.Point{X: pl.ref.X + it.line.x, Y: pl.y}, src)
		}
	}
	switch {
	case it.edge == edgeTop:
		pl.y += it.h
	case it.edge == edgeBottom:
		f := pl.open[len(pl.open)-1]
		p := f.ctx.p
		pl.y += p.Padding[areatree.Bottom]
		f.c.H = pl.y - f.c.Y
		pl.y += p.BorderWidth(areatree.Bottom)
		pl.open = pl.open[:len(pl.open)-1]
	case it.line != nil:
		if err := pl.line(it.line); err != nil {
			return err
		}
		pl.y += it.h
	case it.anchor != nil:
		if err := pl.absolute(it.anchor); err != nil {
			return err
		}
	case it.row != nil:
		r := it.row
		if ev, isNew := pl.note(r, r.markers, it.slice == 0, it.slice == len(r.slices)-2); ev != nil && isNew && r.tbl != nil && r.sec == secBody {
			f := r.tbl.fragFor(pl.page)
			f.events = append(f.events, ev)
		}
		if pl.curRow == nil {
			pl.curRow = &rowFrag{r: it.row, s0: it.slice, s1: it.slice + 1, y: pl.y}
		} else {
			pl.curRow.s1++
		}
		pl.y += it.h
	default:
		// Plain box, such as the rest of an fo:block-container with fixed
		// height.
		pl.y += it.h
	}
	pl.started = true
	for _, f := range pl.open {
		f.end = pl.y
	}
	return nil
}

// sync closes fragments that are no longer in the path and opens the
// missing ones. A fragment is the first of its block if it starts with the
// top edge of the block.
func (pl *placer) sync(path []*blockCtx, edge edgeKind) {
	k := 0
	for k < len(pl.open) && k < len(path) && pl.open[k].ctx == path[k] {
		k++
	}
	for len(pl.open) > k {
		pl.closeTop()
	}
	for i := k; i < len(path); i++ {
		ctx := path[i]
		p := ctx.p
		first := edge == edgeTop && i == len(path)-1
		c := &areatree.Container{Kind: areatree.KindBlock, Background: p.Background, Border: p.Border, Source: ctx.node.Label(), ID: ctx.node.TemplateID()}
		setTag(c, ctx.node)
		if pl.e.started(ctx) {
			ensureTag(c).Continued = true
		}
		c.X = pl.ref.X + ctx.x
		c.W = ctx.w
		c.Y = pl.y
		if first {
			c.Y += p.BorderWidth(areatree.Top)
		} else {
			c.Border[areatree.Top] = areatree.Border{}
		}
		par := pl.parent()
		par.Children = append(par.Children, c)
		pl.open = append(pl.open, &openFrag{ctx: ctx, c: c, end: pl.y})
	}
}

// closeTop closes the innermost fragment at a break or path change.
// Without the bottom edge of the block the bottom border is dropped.
func (pl *placer) closeTop() {
	f := pl.open[len(pl.open)-1]
	f.c.H = max(f.end-f.c.Y, 0)
	f.c.Border[areatree.Bottom] = areatree.Border{}
	pl.open = pl.open[:len(pl.open)-1]
}

func (pl *placer) closeAll() {
	for len(pl.open) > 0 {
		pl.closeTop()
	}
}

// line copies the areas of a line to their position on the page and resolves
// page numbers.
func (pl *placer) line(l *lineBox) error {
	c := &areatree.Container{Kind: areatree.KindLine, Rect: areatree.Rect{X: pl.ref.X + l.x, Y: pl.y, W: l.w, H: l.h}}
	for _, a := range l.areas {
		c.Children = append(c.Children, shift(a, pl.ref.X, pl.y))
	}
	for _, d := range l.deferred {
		run := c.Children[d.index].(*areatree.GlyphRun)
		text := formatNumber(pl.page.Number, pl.e.format)
		gs, err := d.st.face.Shape(text, d.st.p.FontSize, font.ShapeOptions{})
		if err != nil {
			return errorAt(d.node, err)
		}
		run.Glyphs = gs
		var w areatree.Length
		for _, g := range gs {
			w += g.Advance
		}
		pl.e.estimate(d.node, text, w != d.reserved)
	}
	par := pl.parent()
	par.Children = append(par.Children, c)
	return nil
}

// absolute places an absolutely positioned fo:block-container. "absolute"
// refers to the reference area, "fixed" to the page. Per XSL 1.1 7.6.4 (from
// CSS2 9.3.2) left and top denote the margin edge, width and
// height the content rectangle. Apache FOP places left and top at the
// content edge, this is a deliberate deviation (testdata/suites/DEVIATIONS.md).
func (pl *placer) absolute(a *absAnchor) error {
	p := a.p
	ref := pl.ref
	if p.Absolute == "fixed" {
		ref = areatree.Rect{W: pl.page.Width, H: pl.page.Height}
	}
	left, top := areatree.Length(0), areatree.Length(0)
	if p.Left != nil {
		left = *p.Left
	}
	if p.Top != nil {
		top = *p.Top
	}
	bl, br := p.BorderWidth(areatree.Left), p.BorderWidth(areatree.Right)
	bt := p.BorderWidth(areatree.Top)
	padL, padR := p.Padding[areatree.Left], p.Padding[areatree.Right]
	padT, padB := p.Padding[areatree.Top], p.Padding[areatree.Bottom]
	// left, right and top measure to the margin edge (CSS2 10.3.7).
	ml, mr, mt := p.Margin[areatree.Left], p.Margin[areatree.Right], p.Margin[areatree.Top]
	var contentW areatree.Length
	switch {
	case p.Width != nil:
		contentW = *p.Width
	default:
		right := areatree.Length(0)
		if p.Right != nil {
			right = *p.Right
		}
		contentW = ref.W - left - right - ml - mr - bl - br - padL - padR
	}
	contentW = max(contentW, 0)
	inner := areatree.Rect{X: ref.X + left + ml + bl + padL, Y: ref.Y + top + mt + bt + padT, W: contentW}
	kind := areatree.KindBlock
	if v, _ := a.node.Attr("overflow"); v == "hidden" || v == "error-if-overflow" {
		kind = areatree.KindViewport
	}
	cont := &areatree.Container{Kind: kind, Background: p.Background, Border: p.Border, Source: a.node.Label(), ID: a.node.TemplateID()}
	setTag(cont, a.node)
	if par := absParent(a.node); par != "" {
		ensureTag(cont).Parent = par
	}
	cont.X, cont.Y = inner.X-padL, inner.Y-padT
	cont.W = contentW + padL + padR
	pl.page.Areas = append(pl.page.Areas, cont)

	// The container is a new reference area. The children inherit its
	// computed indents (start-indent and end-indent are inherited) and
	// measure them from its content rectangle.
	g := newGalley(pl.e, contentW, pl.page.Number)
	g.page = a.page
	if err := g.blocks(a.node, p); err != nil {
		return err
	}
	sub := newPlacer(pl.e, pl.page, inner, cont)
	if err := sub.place(g.items); err != nil {
		return err
	}
	contentH := sub.y - inner.Y
	if p.Height != nil {
		contentH = *p.Height
	}
	cont.H = contentH + padT + padB
	return nil
}
