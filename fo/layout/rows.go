package layout

import (
	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/fo/tree"
)

// rowFrag ist der auf einer Seite liegende Teil einer Zeile: die Scheiben
// s0 bis s1 (ausschließlich), oben bei y.
type rowFrag struct {
	r      *rowLayout
	s0, s1 int
	y      areatree.Length
}

// flushRow zeichnet das offene Zeilenfragment.
func (pl *placer) flushRow() error {
	if pl.curRow == nil {
		return nil
	}
	f := pl.curRow
	pl.curRow = nil
	r := f.r
	if t := r.tbl; t != nil && t.dynamic && r.sec != secBody {
		if r.sec == secHeader {
			pl.deferred = append(pl.deferred, deferredRow{r: r, y: f.y, parent: pl.parent()})
			return nil
		}
		nr, err := r.relayout(pl.e, pl.page)
		if err != nil {
			return err
		}
		r = nr
	}
	return pl.drawRow(r, r.slices[f.s0], r.slices[f.s1], f.y)
}

// drawRows zeichnet ganze Zeilen ab der aktuellen Position, etwa einen
// wiederholten Tabellenkopf.
func (pl *placer) drawRows(rows []*rowLayout) error {
	for _, r := range rows {
		if t := r.tbl; t != nil && t.dynamic {
			if r.sec == secHeader {
				pl.deferred = append(pl.deferred, deferredRow{r: r, y: pl.y, parent: pl.parent()})
				pl.y += r.h
				continue
			}
			nr, err := r.relayout(pl.e, pl.page)
			if err != nil {
				return err
			}
			if err := pl.drawRow(nr, 0, nr.h, pl.y); err != nil {
				return err
			}
			pl.y += r.h
			continue
		}
		if err := pl.drawRow(r, 0, r.h, pl.y); err != nil {
			return err
		}
		pl.y += r.h
	}
	return nil
}

// drawRow zeichnet den Bereich [a, b) einer Zeile mit Oberkante y.
func (pl *placer) drawRow(r *rowLayout, a, b, y areatree.Length) error {
	return pl.drawRowIn(pl.parent(), r, a, b, y)
}

// drawRowIn zeichnet in einen bestimmten Container.
func (pl *placer) drawRowIn(parent *areatree.Container, r *rowLayout, a, b, y areatree.Length) error {
	whole := a == 0 && b == r.h
	if a == 0 {
		pl.rowIDs(r, y)
	}
	if r.bg != nil {
		bg := *r.bg
		parent.Children = append(parent.Children, &areatree.Box{
			Rect: areatree.Rect{X: pl.ref.X + r.x, Y: y, W: r.w, H: b - a}, Fill: &bg})
	}
	var lines []areatree.Area
	for _, c := range r.cells {
		first := a <= r.sepV
		lastFrag := b == r.h
		top := y + max(r.sepV-a, 0)
		cellH := b - max(a, r.sepV)
		if lastFrag {
			cellH -= r.sepV
		}
		if c.rowSpan > 1 {
			cellH = c.boxH
			lastFrag = true
		}
		ca := max(a-r.sepV-c.inset[areatree.Top], 0)
		cb := b - r.sepV - c.inset[areatree.Top]
		if lastFrag {
			cb = c.contentH
		}
		items := c.items
		if c.rowSpan == 1 {
			items = c.cellRange(ca, cb)
		}

		cont := &areatree.Container{Kind: areatree.KindBlock, Background: c.p.Background, Source: c.node.Label()}
		half := func(s int) areatree.Length {
			if c.border[s].Visible() {
				return c.border[s].Width / 2
			}
			return 0
		}
		full := func(s int) areatree.Length {
			if c.border[s].Visible() {
				return c.border[s].Width
			}
			return 0
		}
		if r.collapse {
			cont.X = pl.ref.X + c.x + half(areatree.Left)
			cont.Y = top + half(areatree.Top)
			cont.W = c.w - half(areatree.Left) - half(areatree.Right)
			cont.H = cellH - half(areatree.Top) - half(areatree.Bottom)
			x0, x1 := pl.ref.X+c.x, pl.ref.X+c.x+c.w
			y0, y1 := top, top+cellH
			add := func(b areatree.Border, from, to areatree.Point) {
				if b.Visible() {
					lines = append(lines, &areatree.Line{From: from, To: to, Width: b.Width, Style: b.Style, Color: b.Color})
				}
			}
			add(c.border[areatree.Top], areatree.Point{X: x0 - half(areatree.Left), Y: y0}, areatree.Point{X: x1 + half(areatree.Right), Y: y0})
			add(c.border[areatree.Bottom], areatree.Point{X: x0 - half(areatree.Left), Y: y1}, areatree.Point{X: x1 + half(areatree.Right), Y: y1})
			add(c.border[areatree.Left], areatree.Point{X: x0, Y: y0}, areatree.Point{X: x0, Y: y1})
			add(c.border[areatree.Right], areatree.Point{X: x1, Y: y0}, areatree.Point{X: x1, Y: y1})
		} else {
			cont.Border = c.border
			bt, bb := full(areatree.Top), full(areatree.Bottom)
			if !first {
				cont.Border[areatree.Top], bt = areatree.Border{}, 0
			}
			if !lastFrag {
				cont.Border[areatree.Bottom], bb = areatree.Border{}, 0
			}
			cont.X = pl.ref.X + c.x + full(areatree.Left)
			cont.Y = top + bt
			cont.W = c.w - full(areatree.Left) - full(areatree.Right)
			cont.H = max(cellH-bt-bb, 0)
		}
		parent.Children = append(parent.Children, cont)

		contentTop := top
		if first {
			contentTop += c.inset[areatree.Top]
		}
		if whole || c.rowSpan > 1 {
			extra := cellH - c.inset[areatree.Top] - c.inset[areatree.Bottom] - c.contentH
			switch c.align {
			case "center":
				contentTop += max(extra/2, 0)
			case "after":
				contentTop += max(extra, 0)
			}
		}
		ref := areatree.Rect{X: pl.ref.X + c.x + c.inset[areatree.Left], Y: contentTop,
			W: max(c.w-c.inset[areatree.Left]-c.inset[areatree.Right], 0)}
		if err := newPlacer(pl.e, pl.page, ref, cont).place(items); err != nil {
			return err
		}
	}
	parent.Children = append(parent.Children, lines...)
	return nil
}

// rowIDs vermerkt die IDs einer Zeile beim Zeichnen ihres ersten Fragments:
// Zeile oder Listenpunkt, die Zellen (auch Label und Rumpf eines
// Listenpunkts) und die Abschnitte der Tabelle bis zur Tabelle selbst.
func (pl *placer) rowIDs(r *rowLayout, y areatree.Length) {
	folio := formatNumber(pl.page.Number, pl.e.format)
	mark := func(n *tree.Node, x areatree.Length) {
		if id, ok := n.Attr("id"); ok && id != "" {
			pl.e.recordID(id, folio)
			pl.e.recordDest(id, pl.page, areatree.Point{X: pl.ref.X + x, Y: y})
		}
	}
	for n := r.node; n != nil && !n.IsFO("table") && !n.IsFO("list-block"); n = n.Parent {
		mark(n, r.x)
	}
	for _, c := range r.cells {
		mark(c.node, c.x)
	}
}
