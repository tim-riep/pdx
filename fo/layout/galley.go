package layout

import (
	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/fo/props"
	"github.com/tim-riep/pdx/fo/tree"
)

// Die Galley ist die vertikale Folge eines Flusses im Box-Glue-Penalty-Modell:
// Boxen (Zeilen, Ober- und Unterkanten von Blöcken, Anker absoluter
// Container), Glue (Abstände, am Seitenanfang verworfen) und Penalties
// (Umbruchstellen mit Kosten). Der Seitenumbruch wählt nur zwischen
// Penalties. Zwei Boxen trennt immer eine Penalty, ein Glue folgt stets auf
// eine Penalty, damit ein Umbruch den Abstand dahinter verwirft.

type itemKind uint8

const (
	itBox itemKind = iota
	itGlue
	itPenalty
)

const (
	// penaltyForced erzwingt einen Umbruch.
	penaltyForced = -1 << 30
	// penaltyInf verbietet einen Umbruch, solange es eine Alternative gibt.
	penaltyInf = 1 << 30
)

type edgeKind uint8

const (
	edgeNone edgeKind = iota
	edgeTop
	edgeBottom
)

// item ist ein Eintrag der Galley.
type item struct {
	kind    itemKind
	h       areatree.Length
	penalty int
	// path sind die umschließenden Blöcke von außen nach innen. Bei Kanten
	// gehört der Block der Kante dazu.
	path   []*blockCtx
	edge   edgeKind
	line   *lineBox
	anchor *absAnchor
}

// blockCtx beschreibt einen Block unabhängig von der Seite. x und w sind die
// Padding-Box relativ zum linken Rand des Referenzbereichs.
type blockCtx struct {
	node *tree.Node
	p    *props.Computed
	x, w areatree.Length
}

// lineBox ist eine gesetzte Zeile. Die Bereiche haben x relativ zum linken
// Rand des Referenzbereichs und y relativ zur Oberkante der Zeile.
type lineBox struct {
	h        areatree.Length
	w        areatree.Length
	x        areatree.Length
	areas    []areatree.Area
	deferred []deferredRun
}

// deferredRun ist eine Seitenzahl im Fluss, die erst beim Platzieren der
// Seite feststeht. index verweist auf den Glyph-Run in lineBox.areas.
type deferredRun struct {
	index    int
	node     *tree.Node
	st       *style
	reserved areatree.Length
}

// absAnchor trägt einen absolut positionierten Container durch die Galley.
// Sein Inhalt wird erst auf der Seite gesetzt, auf der der Anker landet.
type absAnchor struct {
	node *tree.Node
	p    *props.Computed
}

// galley erzeugt die Einträge für einen Referenzbereich der Breite refW.
type galley struct {
	e       *engine
	refW    areatree.Length
	pageNo  int // 0, solange die Seite nicht feststeht (Fluss)
	items   []item
	stack   []*blockCtx
	pending areatree.Length // angesammelter Abstand
}

func newGalley(e *engine, refW areatree.Length, pageNo int) *galley {
	return &galley{e: e, refW: refW, pageNo: pageNo}
}

func (g *galley) contentX(p *props.Computed) areatree.Length { return p.StartIndent }
func (g *galley) contentW(p *props.Computed) areatree.Length {
	return max(g.refW-p.StartIndent-p.EndIndent, 0)
}

func (g *galley) path() []*blockCtx {
	return append([]*blockCtx(nil), g.stack...)
}

// box hängt eine Box an, davor eine Penalty und den angesammelten Abstand.
func (g *galley) box(it item, before int) {
	if len(g.items) > 0 {
		g.items = append(g.items, item{kind: itPenalty, penalty: before})
	}
	if g.pending > 0 {
		g.items = append(g.items, item{kind: itGlue, h: g.pending})
	}
	g.pending = 0
	it.kind = itBox
	g.items = append(g.items, it)
}

// breakBefore liefert die Penalty vor einer neuen Box. Zwischen Oberkante
// und erstem Inhalt sowie vor einer Unterkante wird nicht umbrochen, damit
// Rahmen nicht allein auf einer Seite stehen.
func (g *galley) breakBefore(edge edgeKind) int {
	if edge == edgeBottom {
		return penaltyInf
	}
	for i := len(g.items) - 1; i >= 0; i-- {
		if g.items[i].kind == itBox {
			if g.items[i].edge == edgeTop {
				return penaltyInf
			}
			return 0
		}
	}
	return 0
}

func (g *galley) addLine(l *lineBox) {
	g.box(item{h: l.h, path: g.path(), line: l}, g.breakBefore(edgeNone))
}

// blocks erzeugt die Einträge der Blockkinder eines Elements.
func (g *galley) blocks(n *tree.Node, p *props.Computed) error {
	for _, ch := range n.Children {
		if ch.Kind != tree.Element {
			continue
		}
		if err := g.block(ch, p); err != nil {
			return err
		}
	}
	return nil
}

func (g *galley) block(n *tree.Node, parent *props.Computed) error {
	p, err := props.Compute(parent, n, g.refW)
	if err != nil {
		return err
	}
	if n.IsFO("block-container") {
		if p.Absolute != "" {
			g.box(item{path: g.path(), anchor: &absAnchor{node: n, p: p}}, g.breakBefore(edgeNone))
			return nil
		}
		if p.Width != nil {
			p.EndIndent = max(g.refW-p.StartIndent-*p.Width, 0)
		}
	}
	ctx := &blockCtx{node: n, p: p}
	ctx.x = g.contentX(p) - p.Padding[areatree.Left]
	ctx.w = g.contentW(p) + p.Padding[areatree.Left] + p.Padding[areatree.Right]
	g.stack = append(g.stack, ctx)
	g.pending = max(g.pending, p.SpaceBefore)
	edges := hasBox(p)
	if edges {
		h := p.BorderWidth(areatree.Top) + p.Padding[areatree.Top]
		g.box(item{h: h, path: g.path(), edge: edgeTop}, g.breakBefore(edgeTop))
	}

	if n.IsFO("block-container") {
		err = g.blocks(n, p)
	} else {
		err = g.mixed(n, p)
	}
	if err != nil {
		return err
	}

	if edges {
		// Der Abstand nach dem letzten Kind steht innerhalb des Paddings.
		h := p.Padding[areatree.Bottom] + p.BorderWidth(areatree.Bottom)
		g.box(item{h: h, path: g.path(), edge: edgeBottom}, g.breakBefore(edgeBottom))
	}
	g.stack = g.stack[:len(g.stack)-1]
	g.pending = max(g.pending, p.SpaceAfter)
	return nil
}

// mixed erzeugt die Einträge des gemischten Inhalts eines fo:block: Absätze
// aus Text und Inlines, unterbrochen von Kindblöcken.
func (g *galley) mixed(n *tree.Node, p *props.Computed) error {
	para := newParagraph(g, p, n)
	first := true
	flush := func() error {
		if para.empty() {
			para = newParagraph(g, p, n)
			return nil
		}
		err := g.setParagraph(para, first)
		first = false
		para = newParagraph(g, p, n)
		return err
	}
	for _, ch := range n.Children {
		if ch.Kind == tree.Element && (ch.IsFO("block") || ch.IsFO("block-container")) {
			if err := flush(); err != nil {
				return err
			}
			if err := g.block(ch, p); err != nil {
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

// hasBox meldet sichtbare Rahmen oder Padding.
func hasBox(p *props.Computed) bool {
	for i := 0; i < 4; i++ {
		if p.BorderWidth(i) > 0 || p.Padding[i] > 0 {
			return true
		}
	}
	return false
}
