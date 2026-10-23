package layout

import (
	"strings"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/fo/props"
	"github.com/tim-riep/pdx/fo/tree"
)

// The galley is the vertical sequence of a flow in the box-glue-penalty model:
// boxes (lines, top and bottom edges of blocks, anchors of absolute
// containers), glue (spaces, discarded at the start of a page) and penalties
// (break points with costs). The page break only chooses between
// penalties. Two boxes are always separated by a penalty, a glue always
// follows a penalty so that a break discards the space after it.

type itemKind uint8

const (
	itBox itemKind = iota
	itGlue
	itPenalty
)

const (
	// penaltyForced forces a break.
	penaltyForced = -1 << 30
	// penaltyInf forbids a break as long as there is an alternative.
	penaltyInf = 1 << 30
)

type edgeKind uint8

const (
	edgeNone edgeKind = iota
	edgeTop
	edgeBottom
)

// ordinal is the position of a box in the flow, independent of the width:
// major counts edges, anchors and paragraphs, minor is, for lines, the first
// piece of the paragraph in the line.
type ordinal struct {
	major, minor int
}

// resumePoint is the position in the content of a cell at which a broken
// row continues after a width change: the ordinal of the first box in
// the galley of the cell and, if this box is itself a line, its positions.
type resumePoint struct {
	ord   ordinal
	cells []resumePoint
}

// doneOrd stands for a cell whose content has already been fully placed.
var doneOrd = ordinal{major: 1 << 30}

func (o ordinal) less(p ordinal) bool {
	return o.major < p.major || o.major == p.major && o.minor < p.minor
}

// item is an entry of the galley.
type item struct {
	ord  ordinal
	kind itemKind
	h    areatree.Length
	// keep is, for spaces, the part that is retained at the start of a
	// page (conditionality retain).
	keep areatree.Length
	// shrink is, for spaces, how much they may be shrunk down to the
	// minimum.
	shrink  areatree.Length
	penalty int
	// path holds the enclosing blocks from outside to inside. For edges
	// the block of the edge is included.
	path []*blockCtx
	edge edgeKind
	// brk is page, odd-page or even-page for forced breaks.
	brk string
	// seq is the index of the box in the galley.
	seq    int
	line   *lineBox
	anchor *absAnchor
	// row and slice are set for slices of table rows and list items,
	// resume for slices after the first.
	row    *rowLayout
	slice  int
	resume []resumePoint
}

// blockCtx describes a block independent of the page. x and w are the
// padding box relative to the left edge of the reference area.
type blockCtx struct {
	node *tree.Node
	p    *props.Computed
	x, w areatree.Length
	// keep is the strength of keep-together as a penalty.
	keep int
	// markers are the fo:marker elements of the block, first and last the indices
	// of its first and last box in the galley.
	markers     markerSet
	first, last int
	// ids are IDs of empty inline elements in the block. They apply to the page
	// on which the block starts.
	ids []string
}

// lineBox is a laid out line. The areas have x relative to the left
// edge of the reference area and y relative to the top edge of the line.
type lineBox struct {
	h        areatree.Length
	w        areatree.Length
	x        areatree.Length
	areas    []areatree.Area
	deferred []deferredRun
	// ids are the IDs of inline elements with content in this line.
	ids []string
}

// deferredRun is a page number in the flow that is only known when the
// page is placed. index refers to the glyph run in lineBox.areas.
type deferredRun struct {
	index    int
	node     *tree.Node
	st       *style
	reserved areatree.Length
	// ref is the target ID for page citations, last selects the last page.
	ref  string
	last bool
}

// absAnchor carries an absolutely positioned container through the galley.
// Its content is only laid out on the page on which the anchor lands.
type absAnchor struct {
	node *tree.Node
	p    *props.Computed
	// page is the page for fo:retrieve-marker in fo:static-content.
	page *areatree.Page
}

// galley generates the entries for a reference area of width refW.
type galley struct {
	e    *engine
	refW areatree.Length
	// refX is the content start of the reference area, refProps its
	// properties, while an fo:block-container in the flow is being laid out.
	refX     areatree.Length
	refProps *props.Computed
	pageNo   int // 0 as long as the page is not known (flow)
	items    []item
	stack    []*blockCtx
	// leadKeep is a keep-with-previous before the first content.
	leadKeep int
	// spaces are the spaces of the current sequence (space-after and
	// space-before of consecutive blocks), resolved according to XSL 1.1
	// 4.3.1 at the next content.
	spaces []spaceSpec
	major  int     // counter for ordinal
	skip   ordinal // boxes before it are dropped (rebuild after width change)
	// resume gives, on rebuild for the row at skip, per cell the position
	// from which its content is laid out again.
	resume []resumePoint

	// Keeps and breaks pending for the next penalty.
	keepNext int
	keepPrev int
	brk      string
	lastPath []*blockCtx // path of the last box

	// page is the page for static content (for fo:retrieve-marker),
	// tbl the table when re-laying out header and footer (fo:retrieve-table-marker).
	page *areatree.Page
	tbl  *tableCtx
}

// push opens a block: keep-together, keep-with-previous and
// break-before take effect from here.
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

// pop closes the innermost block: keep-with-next and break-after act
// on the next penalty.
func (g *galley) pop() {
	ctx := g.stack[len(g.stack)-1]
	if noContent(ctx.node) {
		// A block without content produces no box. If it carries an ID, markers
		// or IDs of empty inline elements, a box of height 0 records its
		// position so that references to it are resolved.
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

// noContent reports a block without text and without elements except empty
// inline elements and markers. It produces no line.
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

// spaceBefore collects space-before of a block. The retained part also
// applies at the start of a page.
func (g *galley) spaceBefore(p *props.Computed) {
	g.spaces = append(g.spaces, spaceSpec{v: p.SpaceBefore, min: p.SpaceBeforeMin, prec: p.SpaceBeforePrec, retain: p.SpaceBeforeRetain})
}

// spaceAfter sammelt space-after eines Blocks.
func (g *galley) spaceAfter(p *props.Computed) {
	g.spaces = append(g.spaces, spaceSpec{v: p.SpaceAfter, min: p.SpaceAfterMin, prec: p.SpaceAfterPrec})
}

// spaceSpec is a space with precedence and conditionality.
type spaceSpec struct {
	v, min areatree.Length
	prec   int
	retain bool
}

// resolveSpaces resolves a sequence of spaces according to XSL 1.1 4.3.1:
// forcing spaces (force) all apply and cancel the others, else the
// highest precedence wins and below that the largest optimum. keep is
// the result at the start of a page, where conditional spaces are dropped.
func resolveSpaces(specs []spaceSpec) (h, min, keep areatree.Length) {
	resolve := func(retainedOnly bool) (areatree.Length, areatree.Length) {
		var forced, forcedMin areatree.Length
		anyForce := false
		bestPrec, found := 0, false
		var best spaceSpec
		for _, s := range specs {
			if retainedOnly && !s.retain {
				continue
			}
			if s.prec == props.PrecForce {
				forced += s.v
				forcedMin += s.min
				anyForce = true
				continue
			}
			if !found || s.prec > bestPrec || s.prec == bestPrec && s.v > best.v {
				bestPrec, best, found = s.prec, s, true
			}
		}
		if anyForce {
			return forced, forcedMin
		}
		return best.v, best.min
	}
	h, min = resolve(false)
	keep, _ = resolve(true)
	return h, min, keep
}

// breakValue reads break-before or break-after. column counts like page,
// because there are no multi-column regions in phase 1.
func breakValue(n *tree.Node, prop string) string {
	switch v, _ := n.Attr(prop); v {
	case "page", "column":
		return "page"
	case "odd-page", "even-page":
		return v
	}
	// CSS2 shorthands (XSL 1.1 7.31.16 and 7.31.17).
	short := map[string]string{"break-before": "page-break-before", "break-after": "page-break-after"}[prop]
	switch v, _ := n.Attr(short); v {
	case "always":
		return "page"
	case "left":
		return "even-page"
	case "right":
		return "odd-page"
	}
	return ""
}

// keepInside returns the strongest keep-together binding of the blocks that
// jointly enclose the previous and the new box.
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

// contentX returns the content start of a block. start-indent refers
// to the nearest enclosing reference area (XSL 1.1 5.3.2), whose
// content start is refX.
func (g *galley) contentX(p *props.Computed) areatree.Length { return g.refX + p.StartIndent }

// percentBase returns the base width for percentages of a child: the
// content width of the enclosing block, for the reference area itself its
// width.
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

// box appends a box, preceded by a penalty and the accumulated space.
func (g *galley) box(it item, before int) {
	// A break-after of the last child only takes effect after the bottom edges
	// of the enclosing blocks, their border still belongs on the page.
	if it.edge == edgeBottom && g.brk != "" {
		held := g.brk
		g.brk = ""
		defer func() { g.brk = held }()
	}
	pen := max(before, g.keepNext, g.keepPrev, keepInside(g.lastPath, it.path))
	if len(g.items) == 0 {
		// Keeps before the first content apply to the enclosing element,
		// such as the row of a table.
		g.leadKeep = max(g.leadKeep, g.keepPrev)
	}
	brk := g.brk
	g.keepNext, g.keepPrev, g.brk = 0, 0, ""
	if it.ord.less(g.skip) {
		g.spaces = g.spaces[:0]
		return
	}
	if brk != "" {
		pen = penaltyForced
	}
	if len(g.items) > 0 {
		g.items = append(g.items, item{kind: itPenalty, penalty: pen, brk: brk})
	} else if brk != "" {
		// A break before the first content only counts for odd-page and
		// even-page, the page sequence then inserts a blank page before it.
		g.items = append(g.items, item{kind: itPenalty, penalty: penaltyForced, brk: brk})
	}
	g.lastPath = it.path
	if h, min, keep := resolveSpaces(g.spaces); h != 0 || keep != 0 {
		g.items = append(g.items, item{kind: itGlue, h: h, shrink: h - min, keep: keep})
	}
	g.spaces = g.spaces[:0]
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

// breakBefore returns the penalty before a new box. There is no break
// between the top edge and the first content or before a bottom edge, so
// that borders do not stand alone on a page.
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

// blocks generates the entries of the block children of an element.
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

// retrieve resolves fo:retrieve-marker and fo:retrieve-table-marker. The
// second return value reports whether n is such an element.
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
			// On the first layout the page is not known yet. For the
			// reserved height a marker of the same class from the table applies,
			// the real content follows when drawing.
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
	// Percentages refer to the enclosing block area.
	p, err := props.Compute(parent, n, g.percentBase(parent))
	if err != nil {
		return err
	}
	childParent := p
	if n.IsFO("block-container") {
		if p.Absolute != "" {
			g.box(item{path: g.path(), anchor: &absAnchor{node: n, p: p, page: g.page}, ord: g.next()}, g.breakBefore(edgeNone))
			return nil
		}
		if p.Width != nil {
			// The width fixes the content rectangle, end-indent is
			// adjusted (overconstraint, XSL 1.1 5.3.4). The children inherit the
			// computed value of the property, not the adjustment.
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

	// An fo:block-container with fixed height is not broken, missing
	// height is filled by a box (block-progression-dimension, XSL 1.1 7.15.3).
	fixedH := n.IsFO("block-container") && p.Height != nil
	if fixedH {
		ctx.keep = penaltyInf
	}
	start := len(g.items)
	if n.IsFO("block-container") {
		// fo:block-container is a reference area (XSL 1.1 6.5.3): the
		// indents of the children count from its content rectangle.
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
		// The space before the first content belongs to the space before the
		// container and does not count toward its height.
		for start < len(g.items) && g.items[start].kind != itBox {
			start++
		}
		var used areatree.Length
		for _, it := range g.items[start:] {
			if it.kind == itBox || it.kind == itGlue {
				used += it.h
			}
		}
		// Too much content first shrinks the spaces down to their minimum
		// (XSL 1.1 4.3).
		if used > *p.Height {
			var shrink areatree.Length
			for _, it := range g.items[start:] {
				if it.kind == itGlue {
					shrink += it.shrink
				}
			}
			if shrink > 0 {
				f := min(1, float64(used-*p.Height)/float64(shrink))
				for i := start; i < len(g.items); i++ {
					it := &g.items[i]
					if it.kind == itGlue && it.shrink > 0 {
						d := areatree.Length(f * float64(it.shrink))
						it.h -= d
						it.shrink -= d
						it.keep = min(it.keep, it.h)
						used -= d
					}
				}
			}
		}
		// Missing height is filled, excess content overflows
		// (7.21.2), the following content starts after the fixed height.
		if *p.Height != used {
			g.box(item{h: *p.Height - used, path: g.path(), ord: g.next()}, g.breakBefore(edgeNone))
		}
	}

	if edges {
		// The space after the last child lies inside the padding.
		h := p.Padding[areatree.Bottom] + p.BorderWidth(areatree.Bottom)
		g.box(item{h: h, path: g.path(), edge: edgeBottom, ord: g.next()}, g.breakBefore(edgeBottom))
	}
	g.pop()
	g.spaceAfter(p)
	return nil
}

// mixed generates the entries of the mixed content of an fo:block: paragraphs
// of text and inlines, interrupted by child blocks.
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

// hasBox reports visible borders or padding.
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
