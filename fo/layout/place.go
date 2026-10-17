package layout

import (
	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/font"
)

// placer setzt Galley-Einträge in einen Referenzbereich einer Seite. Er wird
// für Seiten des Flusses, statische Inhalte und absolute Container genutzt.
type placer struct {
	e       *engine
	page    *areatree.Page
	ref     areatree.Rect
	root    *areatree.Container
	y       areatree.Length
	started bool
	open    []*openFrag
	curRow  *rowFrag
	// seen ordnet Bereichen mit Markern ihr Ereignis auf dieser Seite zu.
	seen map[any]*mkEvent
	// deferred sind Kopfzeilen mit Tabellenmarkern, die erst nach den
	// Rumpfzeilen der Seite gezeichnet werden.
	deferred []deferredRow
}

// deferredRow ist eine später zu zeichnende Zeile.
type deferredRow struct {
	r      *rowLayout
	y      areatree.Length
	parent *areatree.Container
}

// openFrag ist das Fragment eines Blocks auf der aktuellen Seite.
type openFrag struct {
	ctx *blockCtx
	c   *areatree.Container
	end areatree.Length // Unterkante des bisher letzten Inhalts
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

// place setzt die Einträge und schließt am Ende alle offenen Fragmente.
// Beginnt die Seite mitten im Rumpf einer Tabelle, wird der Kopf wiederholt,
// endet sie dort, der Fuß.
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
			// Am Seitenanfang bleibt nur der erhaltene Anteil (XSL 1.1 4.3.1).
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
	// Köpfe mit Tabellenmarkern kennen jetzt die Rumpfzeilen der Seite.
	for _, d := range pl.deferred {
		nr, err := d.r.relayout(pl.e, pl.page)
		if err != nil {
			return err
		}
		if err := pl.drawRowIn(d.parent, nr, 0, nr.h, d.y); err != nil {
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
			pl.e.recordDest(id, pl.page, areatree.Point{X: pl.ref.X + ctx.x, Y: pl.y})
		}
		for _, id := range ctx.ids {
			pl.e.recordID(id, folio)
			pl.e.recordDest(id, pl.page, areatree.Point{X: pl.ref.X + ctx.x, Y: pl.y})
		}
	}
	if it.line != nil {
		for _, id := range it.line.ids {
			pl.e.recordID(id, folio)
			pl.e.recordDest(id, pl.page, areatree.Point{X: pl.ref.X + it.line.x, Y: pl.y})
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
		// Schlichte Box, etwa der Rest eines fo:block-container mit fester
		// Höhe.
		pl.y += it.h
	}
	pl.started = true
	for _, f := range pl.open {
		f.end = pl.y
	}
	return nil
}

// sync schließt Fragmente, die nicht mehr im Pfad liegen, und öffnet die
// fehlenden. Ein Fragment ist das erste seines Blocks, wenn es mit der
// Oberkante des Blocks beginnt.
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
		c := &areatree.Container{Kind: areatree.KindBlock, Background: p.Background, Border: p.Border, Source: ctx.node.Label()}
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

// closeTop schließt das innerste Fragment an einem Umbruch oder Pfadwechsel.
// Ohne die Unterkante des Blocks entfällt der untere Rahmen.
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

// line kopiert die Bereiche einer Zeile an ihre Lage auf der Seite und löst
// Seitenzahlen auf.
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

// absolute setzt einen absolut positionierten fo:block-container. "absolute"
// bezieht sich auf den Referenzbereich, "fixed" auf die Seite. left und top
// bezeichnen nach XSL 1.1 7.6.4 (aus CSS2 9.3.2) die Randkante, width und
// height das Inhaltsrechteck. Apache FOP setzt left und top an die
// Inhaltskante, das ist eine bewusste Abweichung (testdata/suites/DEVIATIONS.md).
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
	var contentW areatree.Length
	switch {
	case p.Width != nil:
		contentW = *p.Width
	default:
		right := areatree.Length(0)
		if p.Right != nil {
			right = *p.Right
		}
		contentW = ref.W - left - right - bl - br - padL - padR
	}
	contentW = max(contentW, 0)
	inner := areatree.Rect{X: ref.X + left + bl + padL, Y: ref.Y + top + bt + padT, W: contentW}
	kind := areatree.KindBlock
	if v, _ := a.node.Attr("overflow"); v == "hidden" || v == "error-if-overflow" {
		kind = areatree.KindViewport
	}
	cont := &areatree.Container{Kind: kind, Background: p.Background, Border: p.Border, Source: a.node.Label()}
	cont.X, cont.Y = inner.X-padL, inner.Y-padT
	cont.W = contentW + padL + padR
	pl.page.Areas = append(pl.page.Areas, cont)

	// Der Container ist ein neuer Referenzbereich, Einzüge beginnen bei 0.
	inside := *p
	inside.StartIndent, inside.EndIndent = 0, 0
	g := newGalley(pl.e, contentW, pl.page.Number)
	if err := g.blocks(a.node, &inside); err != nil {
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
