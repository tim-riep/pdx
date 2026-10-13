package layout

import (
	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/fo/props"
	"github.com/tim-riep/pdx/fo/tree"
)

// column setzt Blöcke in einen Referenzbereich, etwa die Region body einer
// Seite, eine statische Region oder einen absolut positionierten Container.
type column struct {
	e   *engine
	seq *sequence
	// breakPage beginnt eine neue Seite. nil bedeutet: kein Umbruch, Inhalt
	// läuft über (statische Inhalte, absolute Container).
	breakPage func() error

	ref     areatree.Rect
	root    *areatree.Container
	y       areatree.Length
	stack   []*openBlock
	pending areatree.Length // noch nicht gesetzter Abstand
	atTop   bool            // noch kein Inhalt im Referenzbereich
}

// openBlock ist ein geöffneter Block mit seinem Fragment auf der aktuellen
// Seite. frag ist nil, solange der Block noch keinen Inhalt hat.
type openBlock struct {
	node      *tree.Node
	p         *props.Computed
	frag      *areatree.Container
	fragments int
}

func (c *column) reset(ref areatree.Rect, root *areatree.Container) {
	c.ref, c.root, c.y, c.pending, c.atTop = ref, root, ref.Y, 0, true
}

func (c *column) bottom() areatree.Length { return c.ref.Y + c.ref.H }

func (c *column) pageNumber() int {
	if c.seq != nil && c.seq.page != nil {
		return c.seq.page.Number
	}
	return 0
}

// contentX und contentW liefern das Inhaltsrechteck eines Blocks.
func (c *column) contentX(p *props.Computed) areatree.Length { return c.ref.X + p.StartIndent }
func (c *column) contentW(p *props.Computed) areatree.Length {
	return max(c.ref.W-p.StartIndent-p.EndIndent, 0)
}

// parentFrag liefert den Container, in den neue Bereiche gehören.
func (c *column) parentFrag() *areatree.Container {
	for i := len(c.stack) - 1; i >= 0; i-- {
		if c.stack[i].frag != nil {
			return c.stack[i].frag
		}
	}
	return c.root
}

// flushSpace setzt den angesammelten Abstand (am Seitenanfang verworfen) und
// legt für alle geöffneten Blöcke ohne Fragment eines an.
func (c *column) flushSpace() {
	if !c.atTop {
		c.y += c.pending
	}
	c.pending = 0
	for _, ob := range c.stack {
		if ob.frag == nil {
			c.materialize(ob)
		}
	}
}

func (c *column) materialize(ob *openBlock) {
	p := ob.p
	first := ob.fragments == 0
	f := &areatree.Container{Kind: areatree.KindBlock, Background: p.Background, Border: p.Border, Source: ob.node.Label()}
	f.X = c.contentX(p) - p.Padding[areatree.Left]
	f.W = c.contentW(p) + p.Padding[areatree.Left] + p.Padding[areatree.Right]
	f.Y = c.y
	if first {
		f.Y += p.BorderWidth(areatree.Top)
	} else {
		f.Border[areatree.Top] = areatree.Border{}
	}
	c.parentFrag().Children = append(c.parentFrag().Children, f)
	ob.frag = f
	ob.fragments++
}

// ensureRoom sorgt dafür, dass h Millipunkt auf die Seite passen, und
// beginnt sonst eine neue Seite.
func (c *column) ensureRoom(h areatree.Length) error {
	c.flushSpace()
	if c.breakPage != nil && !c.atTop && c.y+h > c.bottom() {
		if err := c.breakPage(); err != nil {
			return err
		}
		c.flushSpace()
	}
	return nil
}

// place setzt einen Bereich der Höhe h an die aktuelle Position.
func (c *column) place(a areatree.Area, h areatree.Length) {
	frag := c.parentFrag()
	frag.Children = append(frag.Children, a)
	c.y += h
	c.atTop = false
}

// suspend schließt die Fragmente auf der Seite vor einem Umbruch und liefert
// die geöffneten Blöcke.
func (c *column) suspend() []*openBlock {
	for i := len(c.stack) - 1; i >= 0; i-- {
		ob := c.stack[i]
		if ob.frag != nil {
			ob.frag.H = max(c.y-ob.frag.Y, 0)
			ob.frag.Border[areatree.Bottom] = areatree.Border{}
			ob.frag = nil
		}
	}
	open := c.stack
	c.stack = nil
	return open
}

// resume übernimmt geöffnete Blöcke auf die neue Seite.
func (c *column) resume(open []*openBlock) {
	c.stack = open
	for _, ob := range c.stack {
		if ob.fragments > 0 {
			c.materialize(ob)
		}
	}
}

// blocks setzt die Blockkinder eines Elements (fo:flow, fo:static-content,
// fo:block-container).
func (c *column) blocks(n *tree.Node, p *props.Computed) error {
	for _, ch := range n.Children {
		if ch.Kind != tree.Element {
			continue
		}
		if err := c.block(ch, p); err != nil {
			return err
		}
	}
	return nil
}

func (c *column) block(n *tree.Node, parent *props.Computed) error {
	p, err := props.Compute(parent, n, c.ref.W)
	if err != nil {
		return err
	}
	if n.IsFO("block-container") {
		if p.Absolute != "" {
			return c.absolute(n, p)
		}
		if p.Width != nil {
			p.EndIndent = max(c.ref.W-p.StartIndent-*p.Width, 0)
		}
	}

	ob := &openBlock{node: n, p: p}
	c.stack = append(c.stack, ob)
	c.pending = max(c.pending, p.SpaceBefore)
	if top := p.BorderWidth(areatree.Top) + p.Padding[areatree.Top]; top > 0 {
		if err := c.ensureRoom(top); err != nil {
			return err
		}
		c.y += top
		c.atTop = false
	}

	if n.IsFO("block-container") {
		err = c.blocks(n, p)
	} else {
		err = c.mixed(n, p)
	}
	if err != nil {
		return err
	}

	// Leere Blöcke mit Rahmen oder Padding erzeugen trotzdem einen Bereich.
	bottom := p.BorderWidth(areatree.Bottom) + p.Padding[areatree.Bottom]
	if ob.frag == nil && ob.fragments == 0 && hasBox(p) {
		c.flushSpace()
	}
	if ob.frag != nil {
		if bottom > 0 {
			if !c.atTop {
				c.y += c.pending
			}
			c.pending = 0
			c.y += p.Padding[areatree.Bottom]
			ob.frag.H = c.y - ob.frag.Y
			c.y += p.BorderWidth(areatree.Bottom)
			c.atTop = false
		} else {
			ob.frag.H = c.y - ob.frag.Y
		}
	}
	c.stack = c.stack[:len(c.stack)-1]
	c.pending = max(c.pending, p.SpaceAfter)
	return nil
}

// mixed setzt den gemischten Inhalt eines fo:block: Absätze aus Text und
// Inlines, unterbrochen von Kindblöcken.
func (c *column) mixed(n *tree.Node, p *props.Computed) error {
	para := newParagraph(c, p, n)
	first := true
	flush := func() error {
		if para.empty() {
			para = newParagraph(c, p, n)
			return nil
		}
		err := c.setParagraph(para, first)
		first = false
		para = newParagraph(c, p, n)
		return err
	}
	for _, ch := range n.Children {
		if ch.Kind == tree.Element && (ch.IsFO("block") || ch.IsFO("block-container")) {
			if err := flush(); err != nil {
				return err
			}
			if err := c.block(ch, p); err != nil {
				return err
			}
			continue
		}
		if err := para.add(ch, p); err != nil {
			return err
		}
	}
	return flush()
}

// absolute setzt einen absolut positionierten fo:block-container. Er nimmt
// keinen Platz im Fluss ein. "absolute" bezieht sich auf den Referenzbereich
// (die Region), "fixed" auf die Seite.
func (c *column) absolute(n *tree.Node, p *props.Computed) error {
	ref := c.ref
	page := c.seq.page
	if p.Absolute == "fixed" {
		ref = areatree.Rect{W: page.Width, H: page.Height}
	}
	left, top := areatree.Length(0), areatree.Length(0)
	if p.Left != nil {
		left = *p.Left
	}
	if p.Top != nil {
		top = *p.Top
	}
	w := ref.W - left
	if p.Right != nil {
		w -= *p.Right
	}
	if p.Width != nil {
		w = *p.Width
	}
	bl, br := p.BorderWidth(areatree.Left), p.BorderWidth(areatree.Right)
	bt, bb := p.BorderWidth(areatree.Top), p.BorderWidth(areatree.Bottom)
	// Das Rechteck des Containers ist die Border-Box. Inhalt liegt innerhalb
	// von Rahmen und Padding.
	outer := areatree.Rect{X: ref.X + left, Y: ref.Y + top, W: w}
	inner := areatree.Rect{
		X: outer.X + bl + p.Padding[areatree.Left],
		Y: outer.Y + bt + p.Padding[areatree.Top],
		W: max(w-bl-br-p.Padding[areatree.Left]-p.Padding[areatree.Right], 0),
	}
	if p.Height != nil {
		inner.H = max(*p.Height-bt-bb-p.Padding[areatree.Top]-p.Padding[areatree.Bottom], 0)
	}
	kind := areatree.KindBlock
	if v, _ := n.Attr("overflow"); v == "hidden" || v == "error-if-overflow" {
		kind = areatree.KindViewport
	}
	cont := &areatree.Container{Kind: kind, Background: p.Background, Border: p.Border, Source: n.Label()}
	cont.X = outer.X + bl
	cont.Y = outer.Y + bt
	cont.W = inner.W + p.Padding[areatree.Left] + p.Padding[areatree.Right]
	page.Areas = append(page.Areas, cont)

	sub := &column{e: c.e, seq: c.seq}
	sub.reset(inner, cont)
	// Der Container ist ein neuer Referenzbereich, Einzüge beginnen bei 0.
	inside := *p
	inside.StartIndent, inside.EndIndent = 0, 0
	if err := sub.blocks(n, &inside); err != nil {
		return err
	}
	contentH := sub.y - inner.Y
	if p.Height != nil {
		contentH = inner.H
	}
	cont.H = contentH + p.Padding[areatree.Top] + p.Padding[areatree.Bottom]
	return nil
}

// hasBox meldet sichtbare Rahmen oder Padding.
func hasBox(p *props.Computed) bool {
	for i := 0; i < 4; i++ {
		if p.BorderWidth(i) > 0 || p.Padding[i] > 0 {
			return true
		}
	}
	return false
}
