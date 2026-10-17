package layout

import (
	"strings"

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

// ordinal ist die Stelle einer Box im Fluss, unabhängig von der Breite:
// major zählt Kanten, Anker und Absätze, minor ist bei Zeilen das erste
// Stück des Absatzes in der Zeile.
type ordinal struct {
	major, minor int
}

func (o ordinal) less(p ordinal) bool {
	return o.major < p.major || o.major == p.major && o.minor < p.minor
}

// item ist ein Eintrag der Galley.
type item struct {
	ord  ordinal
	kind itemKind
	h    areatree.Length
	// keep ist bei Abständen der Anteil, der am Anfang einer Seite erhalten
	// bleibt (Konditionalität retain).
	keep    areatree.Length
	penalty int
	// path sind die umschließenden Blöcke von außen nach innen. Bei Kanten
	// gehört der Block der Kante dazu.
	path []*blockCtx
	edge edgeKind
	// brk ist bei erzwungenen Umbrüchen page, odd-page oder even-page.
	brk string
	// seq ist der Index der Box in der Galley.
	seq    int
	line   *lineBox
	anchor *absAnchor
	// row und slice sind bei Scheiben von Tabellenzeilen und Listenpunkten gesetzt.
	row   *rowLayout
	slice int
}

// blockCtx beschreibt einen Block unabhängig von der Seite. x und w sind die
// Padding-Box relativ zum linken Rand des Referenzbereichs.
type blockCtx struct {
	node *tree.Node
	p    *props.Computed
	x, w areatree.Length
	// keep ist die Stärke von keep-together als Penalty.
	keep int
	// markers sind die fo:marker des Blocks, first und last die Indizes
	// seiner ersten und letzten Box in der Galley.
	markers     markerSet
	first, last int
	// ids sind IDs leerer Inline-Elemente im Block. Sie gelten für die Seite,
	// auf der der Block beginnt.
	ids []string
}

// lineBox ist eine gesetzte Zeile. Die Bereiche haben x relativ zum linken
// Rand des Referenzbereichs und y relativ zur Oberkante der Zeile.
type lineBox struct {
	h        areatree.Length
	w        areatree.Length
	x        areatree.Length
	areas    []areatree.Area
	deferred []deferredRun
	// ids sind die IDs von Inline-Elementen mit Inhalt in dieser Zeile.
	ids []string
}

// deferredRun ist eine Seitenzahl im Fluss, die erst beim Platzieren der
// Seite feststeht. index verweist auf den Glyph-Run in lineBox.areas.
type deferredRun struct {
	index    int
	node     *tree.Node
	st       *style
	reserved areatree.Length
	// ref ist bei Seitenverweisen die Ziel-ID, last wählt die letzte Seite.
	ref  string
	last bool
}

// absAnchor trägt einen absolut positionierten Container durch die Galley.
// Sein Inhalt wird erst auf der Seite gesetzt, auf der der Anker landet.
type absAnchor struct {
	node *tree.Node
	p    *props.Computed
}

// galley erzeugt die Einträge für einen Referenzbereich der Breite refW.
type galley struct {
	e    *engine
	refW areatree.Length
	// refX ist der Inhaltsanfang der Referenzfläche, refProps ihre
	// Eigenschaften, solange ein fo:block-container im Fluss gesetzt wird.
	refX     areatree.Length
	refProps *props.Computed
	pageNo   int // 0, solange die Seite nicht feststeht (Fluss)
	items    []item
	stack    []*blockCtx
	pending  areatree.Length // angesammelter Abstand
	// pendingKeep ist der Teil des Abstands, der am Seitenanfang bleibt.
	pendingKeep areatree.Length
	major       int     // Zähler für ordinal
	skip        ordinal // Boxen davor entfallen (Neuaufbau nach Breitenwechsel)

	// Für die nächste Penalty vorgemerkte Keeps und Umbrüche.
	keepNext int
	keepPrev int
	brk      string
	lastPath []*blockCtx // Pfad der letzten Box

	// page ist die Seite bei statischen Inhalten (für fo:retrieve-marker),
	// tbl die Tabelle beim Neusetzen von Kopf und Fuß (fo:retrieve-table-marker).
	page *areatree.Page
	tbl  *tableCtx
}

// push öffnet einen Block: keep-together, keep-with-previous und
// break-before wirken ab hier.
func (g *galley) push(ctx *blockCtx) {
	n := ctx.node
	ctx.keep = keepValue(n, "keep-together")
	ctx.markers = collectMarkers(n)
	ctx.first, ctx.last = -1, -1
	g.keepPrev = max(g.keepPrev, keepValue(n, "keep-with-previous"))
	if b := breakValue(n, "break-before"); b != "" {
		g.brk = b
	}
	g.stack = append(g.stack, ctx)
}

// pop schließt den innersten Block: keep-with-next und break-after wirken
// auf die nächste Penalty.
func (g *galley) pop() {
	ctx := g.stack[len(g.stack)-1]
	if noContent(ctx.node) {
		// Ein Block ohne Inhalt erzeugt keine Box. Trägt er eine ID, Marker
		// oder IDs leerer Inline-Elemente, hält eine Box der Höhe 0 seine
		// Lage fest, damit Verweise auf ihn aufgelöst werden.
		if id, _ := ctx.node.Attr("id"); id != "" || len(ctx.markers) > 0 || len(ctx.ids) > 0 {
			g.box(item{path: g.path(), ord: g.next()}, g.breakBefore(edgeNone))
		}
	}
	g.stack = g.stack[:len(g.stack)-1]
	g.keepNext = max(g.keepNext, keepValue(ctx.node, "keep-with-next"))
	if b := breakValue(ctx.node, "break-after"); b != "" {
		g.brk = b
	}
}

// noContent meldet einen Block ohne Text und ohne Elemente außer leeren
// Inline-Elementen und Markern. Er erzeugt keine Zeile.
func noContent(n *tree.Node) bool {
	for _, c := range n.Children {
		switch {
		case c.Kind == tree.Text:
			if strings.TrimSpace(c.Data) != "" {
				return false
			}
		case c.IsFO("marker"):
		case c.IsFO("inline") || c.IsFO("wrapper"):
			if !noContent(c) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// spaceBefore sammelt space-before eines Blocks. Der erhaltene Anteil gilt
// auch am Anfang einer Seite.
func (g *galley) spaceBefore(p *props.Computed) {
	g.pending = max(g.pending, p.SpaceBefore)
	if p.SpaceBeforeRetain {
		g.pendingKeep = max(g.pendingKeep, p.SpaceBefore)
	}
}

// breakValue liest break-before oder break-after. column gilt wie page, weil
// es in Phase 1 keine mehrspaltigen Regionen gibt.
func breakValue(n *tree.Node, prop string) string {
	switch v, _ := n.Attr(prop); v {
	case "page", "column":
		return "page"
	case "odd-page", "even-page":
		return v
	}
	return ""
}

// keepInside liefert die stärkste keep-together-Bindung der Blöcke, die die
// vorige und die neue Box gemeinsam umschließen.
func keepInside(a, b []*blockCtx) int {
	k := 0
	for i := 0; i < len(a) && i < len(b) && a[i] == b[i]; i++ {
		k = max(k, a[i].keep)
	}
	return k
}

func newGalley(e *engine, refW areatree.Length, pageNo int) *galley {
	return &galley{e: e, refW: refW, pageNo: pageNo}
}

// contentX liefert den Inhaltsanfang eines Blocks. start-indent bezieht
// sich auf die nächste umgebende Referenzfläche (XSL 1.1 5.3.2), deren
// Inhaltsanfang refX ist.
func (g *galley) contentX(p *props.Computed) areatree.Length { return g.refX + p.StartIndent }

// percentBase liefert die Bezugsbreite für Prozentangaben eines Kindes: die
// Inhaltsbreite des umgebenden Blocks, bei der Referenzfläche selbst deren
// Breite.
func (g *galley) percentBase(parent *props.Computed) areatree.Length {
	if parent == g.refProps {
		return g.refW
	}
	return g.contentW(parent)
}
func (g *galley) contentW(p *props.Computed) areatree.Length {
	return max(g.refW-p.StartIndent-p.EndIndent, 0)
}

func (g *galley) path() []*blockCtx {
	return append([]*blockCtx(nil), g.stack...)
}

// box hängt eine Box an, davor eine Penalty und den angesammelten Abstand.
func (g *galley) box(it item, before int) {
	// Ein break-after des letzten Kindes wirkt erst nach den Unterkanten der
	// umschließenden Blöcke, deren Rahmen gehört noch auf die Seite.
	if it.edge == edgeBottom && g.brk != "" {
		held := g.brk
		g.brk = ""
		defer func() { g.brk = held }()
	}
	pen := max(before, g.keepNext, g.keepPrev, keepInside(g.lastPath, it.path))
	brk := g.brk
	g.keepNext, g.keepPrev, g.brk = 0, 0, ""
	if it.ord.less(g.skip) {
		g.pending, g.pendingKeep = 0, 0
		return
	}
	if brk != "" {
		pen = penaltyForced
	}
	if len(g.items) > 0 {
		g.items = append(g.items, item{kind: itPenalty, penalty: pen, brk: brk})
	} else if brk != "" {
		// Ein Umbruch vor dem ersten Inhalt zählt nur bei odd-page und
		// even-page, die Seitenfolge setzt dann eine Leerseite davor.
		g.items = append(g.items, item{kind: itPenalty, penalty: penaltyForced, brk: brk})
	}
	g.lastPath = it.path
	if g.pending > 0 || g.pendingKeep > 0 {
		g.items = append(g.items, item{kind: itGlue, h: g.pending, keep: g.pendingKeep})
	}
	g.pending, g.pendingKeep = 0, 0
	it.kind = itBox
	it.seq = len(g.items)
	for _, ctx := range it.path {
		if ctx.first < 0 {
			ctx.first = it.seq
		}
		ctx.last = it.seq
	}
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

func (g *galley) addLine(l *lineBox, ord ordinal, pen int) {
	g.box(item{h: l.h, path: g.path(), line: l, ord: ord}, max(pen, g.breakBefore(edgeNone)))
}

func (g *galley) next() ordinal {
	g.major++
	return ordinal{major: g.major}
}

// blocks erzeugt die Einträge der Blockkinder eines Elements.
func (g *galley) blocks(n *tree.Node, p *props.Computed) error {
	for _, ch := range n.Children {
		if ch.Kind != tree.Element || ch.IsFO("marker") {
			continue
		}
		if m, isRetrieve, err := g.retrieve(ch); isRetrieve {
			if err != nil {
				return err
			}
			if m != nil {
				if err := g.mixed(m, p); err != nil {
					return err
				}
			}
			continue
		}
		if err := g.block(ch, p); err != nil {
			return err
		}
	}
	return nil
}

// retrieve löst fo:retrieve-marker und fo:retrieve-table-marker auf. Der
// zweite Rückgabewert meldet, ob n ein solches Element ist.
func (g *galley) retrieve(n *tree.Node) (*tree.Node, bool, error) {
	switch {
	case n.IsFO("retrieve-marker"):
		if g.page == nil {
			return nil, true, tree.Errorf(n, "fo:retrieve-marker ist nur in fo:static-content erlaubt")
		}
		m, err := g.e.retrieveMarker(n, g.page)
		return m, true, err
	case n.IsFO("retrieve-table-marker"):
		if g.tbl == nil {
			return nil, true, tree.Errorf(n, "fo:retrieve-table-marker ist nur in fo:table-header und fo:table-footer erlaubt")
		}
		if g.page == nil {
			// Beim ersten Setzen steht die Seite noch nicht fest. Für die
			// reservierte Höhe gilt ein Marker derselben Klasse aus der Tabelle,
			// der echte Inhalt folgt beim Zeichnen.
			cls, _ := n.Attr("retrieve-class-name")
			return g.tbl.sample(cls), true, nil
		}
		m, err := g.tbl.retrieveTableMarker(n, g.page)
		return m, true, err
	}
	return nil, false, nil
}

func (g *galley) block(n *tree.Node, parent *props.Computed) error {
	switch {
	case n.IsFO("table"):
		return g.table(n, parent)
	case n.IsFO("list-block"):
		return g.list(n, parent)
	}
	// Prozentangaben beziehen sich auf den umgebenden Blockbereich.
	p, err := props.Compute(parent, n, g.percentBase(parent))
	if err != nil {
		return err
	}
	childParent := p
	if n.IsFO("block-container") {
		if p.Absolute != "" {
			g.box(item{path: g.path(), anchor: &absAnchor{node: n, p: p}, ord: g.next()}, g.breakBefore(edgeNone))
			return nil
		}
		if p.Width != nil {
			// Die Breite legt das Inhaltsrechteck fest, end-indent wird
			// angepasst (Überbestimmung, XSL 1.1 5.3.4). Die Kinder erben den
			// berechneten Wert der Eigenschaft, nicht die Anpassung.
			inherited := *p
			p.EndIndent = max(g.refW-p.StartIndent-*p.Width, 0)
			childParent = &inherited
		}
	}
	ctx := &blockCtx{node: n, p: p}
	ctx.x = g.contentX(p) - p.Padding[areatree.Left]
	ctx.w = g.contentW(p) + p.Padding[areatree.Left] + p.Padding[areatree.Right]
	g.push(ctx)
	g.spaceBefore(p)
	edges := hasBox(p)
	if edges {
		h := p.BorderWidth(areatree.Top) + p.Padding[areatree.Top]
		g.box(item{h: h, path: g.path(), edge: edgeTop, ord: g.next()}, g.breakBefore(edgeTop))
	}

	// Ein fo:block-container mit fester Höhe wird nicht umbrochen, fehlende
	// Höhe füllt eine Box auf (block-progression-dimension, XSL 1.1 7.15.3).
	fixedH := n.IsFO("block-container") && p.Height != nil
	if fixedH {
		ctx.keep = penaltyInf
	}
	start := len(g.items)
	if n.IsFO("block-container") {
		// fo:block-container ist eine Referenzfläche (XSL 1.1 6.5.3): Die
		// Einzüge der Kinder zählen ab ihrem Inhaltsrechteck.
		saveX, saveW, saveP := g.refX, g.refW, g.refProps
		g.refX, g.refW, g.refProps = g.contentX(p), g.contentW(p), childParent
		err = g.blocks(n, childParent)
		g.refX, g.refW, g.refProps = saveX, saveW, saveP
	} else {
		err = g.mixed(n, p)
	}
	if err != nil {
		return err
	}
	if fixedH {
		var used areatree.Length
		for _, it := range g.items[start:] {
			if it.kind == itBox || it.kind == itGlue {
				used += it.h
			}
		}
		if *p.Height > used {
			g.box(item{h: *p.Height - used, path: g.path(), ord: g.next()}, g.breakBefore(edgeNone))
		}
	}

	if edges {
		// Der Abstand nach dem letzten Kind steht innerhalb des Paddings.
		h := p.Padding[areatree.Bottom] + p.BorderWidth(areatree.Bottom)
		g.box(item{h: h, path: g.path(), edge: edgeBottom, ord: g.next()}, g.breakBefore(edgeBottom))
	}
	g.pop()
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
	var walk func(children []*tree.Node) error
	walk = func(children []*tree.Node) error {
		for _, ch := range children {
			if ch.IsFO("marker") {
				continue
			}
			if m, isRetrieve, err := g.retrieve(ch); isRetrieve {
				if err != nil {
					return err
				}
				if m != nil {
					if err := walk(m.Children); err != nil {
						return err
					}
				}
				continue
			}
			if ch.Kind == tree.Element && isBlockLevel(ch) {
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
		return nil
	}
	if err := walk(n.Children); err != nil {
		return err
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

func isBlockLevel(n *tree.Node) bool {
	return n.IsFO("block") || n.IsFO("block-container") || n.IsFO("table") || n.IsFO("list-block")
}
