package layout

import (
	"sort"

	"strconv"
	"strings"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/fo/props"
	"github.com/tim-riep/pdx/fo/tree"
)

// Tables and lists share the row model: a row consists of
// cells side by side whose content is each laid out in its own galley.
// In the galley of the flow a row stands as a sequence of slices.
// A break is allowed between two slices, at their boundaries each
// cell has an allowed break point or no more content.

type section uint8

const (
	secBody section = iota
	secHeader
	secFooter
)

// tableCtx holds the data of a table for repetition and page breaking.
type tableCtx struct {
	node       *tree.Node
	header     []*rowLayout
	footer     []*rowLayout
	headerH    areatree.Length
	footerH    areatree.Length
	omitHeader bool
	omitFooter bool
	firstBody  *rowLayout
	lastBody   *rowLayout
	// dynamic is set if header or footer contain fo:retrieve-table-marker.
	// They are then laid out again per page.
	dynamic bool
	frags   []*tableFrag
	// representative is set while repeated headers are being measured.
	// sample then skips empty markers.
	representative bool
}

// continuesAfter reports whether more body rows follow after this slice.
func (t *tableCtx) continuesAfter(r *rowLayout, slice int) bool {
	return !(r == t.lastBody && slice == len(r.slices)-2)
}

// rowLayout is a laid out row of a table or a list item.
type rowLayout struct {
	node   *tree.Node
	tbl    *tableCtx // nil for lists
	sec    section
	cells  []*cellLayout // cells that start in this row
	h      areatree.Length
	sepV   areatree.Length // half border-separation above and below the cells
	bg     *areatree.Color
	x, w   areatree.Length   // row rectangle relative to the reference area
	slices []areatree.Length // slice boundaries, starting at 0, ending at h
	last   bool              // last row of its section
	keep   int               // penalty between the slices
	// forced are forced breaks from the cell content at slice boundaries
	// (index of the boundary), leadBrk and trailBrk breaks before and after the
	// row (break-before at the start, break-after at the end of a cell).
	forced              map[int]string
	leadBrk, trailBrk   string
	leadKeep, trailKeep int
	// sliceKeep holds keeps at slice boundaries after the content end of a
	// cell with keep-with-next.
	sliceKeep map[int]int
	// base is the number of the first slice if the row was laid out again
	// from a later slice after a width change.
	base int
	// skip is, per slice, the space at its start that is dropped after a
	// break. Between two slices it stands as glue.
	skip map[int]areatree.Length
	// skipBreakBefore is set if break-before of the first body row
	// already takes effect before the table.
	skipBreakBefore bool
	// skipBreakAfter is set if break-after of the last body row
	// only takes effect after the table.
	skipBreakAfter bool
	markers        markerSet
	collapse       bool
}

// cellLayout is a laid out cell.
type cellLayout struct {
	node     *tree.Node
	p        *props.Computed
	col      int
	span     int
	rowSpan  int
	x, w     areatree.Length // cell rectangle (separate: border box, collapse: grid)
	boxH     areatree.Length // height of the cell rectangle over all spanned rows
	border   [4]areatree.Border
	inset    [4]areatree.Length // border (full or half) plus padding
	items    []item
	contentH areatree.Length
	// minH is the minimum height of the content rectangle from height or
	// block-progression-dimension of the cell.
	minH   areatree.Length
	bounds []bound
	align  string
	// leadBrk and trailBrk are break-before at the start and break-after at the
	// end of the content, leadKeep and trailKeep keep-with-previous and
	// keep-with-next.
	leadBrk, trailBrk   string
	leadKeep, trailKeep int
}

// bound is an allowed break point in the content of a cell.
type bound struct {
	off   areatree.Length // offset from the content start
	index int             // index of the penalty in items
	brk   string          // page, odd-page or even-page for forced breaks
	// skip is the space after the break point that is dropped at the page start.
	skip areatree.Length
}

// measure computes the content height and the allowed break points of a
// galley the way the placer would lay them out.
func measure(items []item) (areatree.Length, []bound) {
	var y areatree.Length
	started := false
	var bs []bound
	for i, it := range items {
		switch it.kind {
		case itGlue:
			if started {
				y += it.h
			}
		case itBox:
			y += it.h
			started = true
		case itPenalty:
			if it.penalty < penaltyInf && started {
				b := bound{off: y, index: i}
				for _, nx := range items[i+1:] {
					if nx.kind != itGlue {
						break
					}
					b.skip += nx.h - nx.keep
				}
				if it.penalty <= penaltyForced {
					b.brk = it.brk
					if b.brk == "" {
						b.brk = "page"
					}
				}
				bs = append(bs, b)
			}
		}
	}
	return y, bs
}

// cellRange returns the entries of a cell for the content range [a, b).
func (c *cellLayout) cellRange(a, b areatree.Length) []item {
	start, end := 0, len(c.items)
	if a > 0 {
		start = len(c.items)
		for _, bd := range c.bounds {
			if bd.off <= a {
				start = bd.index + 1
			}
		}
		if a >= c.contentH {
			start = len(c.items)
		}
	}
	if b < c.contentH {
		end = start
		for _, bd := range c.bounds {
			if bd.off <= b && bd.index >= start {
				end = bd.index
			}
		}
	}
	if start > end {
		return nil
	}
	return c.items[start:end]
}

// cellBreaks returns the break points of a cell in row coordinates.
func (c *cellLayout) breakable(off areatree.Length, top areatree.Length) bool {
	rel := off - top
	if rel <= 0 || rel >= c.contentH {
		return true
	}
	for _, bd := range c.bounds {
		if bd.off == rel {
			return true
		}
	}
	return false
}

// layoutCell lays out the content of a cell in its own galley. The cell
// is a reference area, indents start at 0 inside it.
func (g *galley) layoutCell(c *cellLayout, content *tree.Node, tbl *tableCtx) error {
	return g.layoutCellAt(c, content, tbl, nil)
}

// layoutCellAt lays out the content of a cell from position rp, without rp
// from the beginning.
func (g *galley) layoutCellAt(c *cellLayout, content *tree.Node, tbl *tableCtx, rp *resumePoint) error {
	w := max(c.w-c.inset[areatree.Left]-c.inset[areatree.Right], 0)
	inside := *c.p
	inside.StartIndent, inside.EndIndent = 0, 0
	sg := newGalley(g.e, w, g.pageNo)
	sg.tbl = tbl
	if rp != nil {
		sg.skip, sg.resume = rp.ord, rp.cells
	}
	if err := sg.blocks(content, &inside); err != nil {
		return err
	}
	c.items = sg.items
	c.contentH, c.bounds = measure(sg.items)
	// Breaks at the start and end of the cell content apply before and after the
	// row (XSL 1.1 7.20.1, 7.20.2).
	c.leadBrk, c.trailBrk = "", sg.brk
	if len(sg.items) > 0 && sg.items[0].kind == itPenalty {
		c.leadBrk = sg.items[0].brk
	}
	// Likewise keeps at the start and end of the cell content (XSL 1.1 4.8).
	c.leadKeep, c.trailKeep = sg.leadKeep, sg.keepNext
	return nil
}

// repeatRows re-measures header rows with table markers for repetition.
// The rows do not get lower than at the first layout.
func (g *galley) repeatRows(tc *tableCtx, rows []*rowLayout, fs areatree.Length) ([]*rowLayout, error) {
	tc.representative = true
	defer func() { tc.representative = false }()
	out := make([]*rowLayout, len(rows))
	for i, r := range rows {
		nr := *r
		nr.cells = make([]*cellLayout, len(r.cells))
		for j, c := range r.cells {
			nc := *c
			if hasTableMarker(c.node) {
				if err := g.layoutCell(&nc, nc.node, tc); err != nil {
					return nil, err
				}
			}
			nr.cells[j] = &nc
		}
		minH, err := rowHeight(r.node, fs)
		if err != nil {
			return nil, err
		}
		finishRow(&nr, max(minH, r.h-2*r.sepV), true)
		for _, c := range nr.cells {
			if c.rowSpan == 1 {
				c.boxH = nr.h - 2*nr.sepV
			}
		}
		out[i] = &nr
	}
	return out, nil
}

// finishRow computes height and slices of a row.
func finishRow(r *rowLayout, minH areatree.Length, spanned bool) {
	h := minH
	for _, c := range r.cells {
		if c.rowSpan > 1 {
			continue
		}
		h = max(h, c.inset[areatree.Top]+max(c.contentH, c.minH)+c.inset[areatree.Bottom])
	}
	r.h = r.sepV + h + r.sepV
	r.slices = []areatree.Length{0}
	if !spanned && r.keep < penaltyInf {
		// Candidates are all break points of all cells.
		var cands []areatree.Length
		for _, c := range r.cells {
			top := r.sepV + c.inset[areatree.Top]
			for _, bd := range c.bounds {
				cands = append(cands, top+bd.off)
			}
		}
		sortLengths(cands)
		for _, off := range cands {
			if off <= r.slices[len(r.slices)-1] || off >= r.h {
				continue
			}
			ok := true
			for _, c := range r.cells {
				if !c.breakable(off, r.sepV+c.inset[areatree.Top]) {
					ok = false
					break
				}
			}
			if ok {
				r.slices = append(r.slices, off)
				if sk := rowSkip(r, off); sk > 0 {
					if r.skip == nil {
						r.skip = map[int]areatree.Length{}
					}
					r.skip[len(r.slices)-1] = sk
				}
				for _, c := range r.cells {
					top := r.sepV + c.inset[areatree.Top]
					for _, bd := range c.bounds {
						if bd.brk != "" && top+bd.off == off {
							if r.forced == nil {
								r.forced = map[int]string{}
							}
							r.forced[len(r.slices)-1] = bd.brk
						}
					}
				}
			}
		}
	}
	r.slices = append(r.slices, r.h)
	for _, c := range r.cells {
		if r.leadBrk == "" {
			r.leadBrk = c.leadBrk
		}
		if r.trailBrk == "" {
			r.trailBrk = c.trailBrk
		}
		r.leadKeep = max(r.leadKeep, c.leadKeep)
		r.trailKeep = max(r.trailKeep, c.trailKeep)
		if c.trailKeep > 0 {
			end := r.sepV + c.inset[areatree.Top] + c.contentH
			for k := 1; k+1 < len(r.slices); k++ {
				if r.slices[k] >= end {
					if r.sliceKeep == nil {
						r.sliceKeep = map[int]int{}
					}
					r.sliceKeep[k] = max(r.sliceKeep[k], c.trailKeep)
				}
			}
		}
	}
}

// rowResume returns, per cell, the position at which its content starts in
// slice k.
func rowResume(r *rowLayout, k int) []resumePoint {
	out := make([]resumePoint, len(r.cells))
	for i, c := range r.cells {
		out[i].ord = doneOrd
		a := r.slices[k] + r.skip[k] - r.sepV - c.inset[areatree.Top]
		if c.rowSpan != 1 || a >= c.contentH {
			continue
		}
		for _, it := range c.cellRange(max(a, 0), c.contentH) {
			if it.kind == itBox {
				out[i] = resumePoint{ord: it.ord, cells: it.resume}
				break
			}
		}
	}
	return out
}

// rowSkip returns the space that is dropped at a break at off in all cells
// with further content.
func rowSkip(r *rowLayout, off areatree.Length) areatree.Length {
	skip, found := areatree.Length(0), false
	for _, c := range r.cells {
		top := r.sepV + c.inset[areatree.Top]
		if off >= top+c.contentH {
			continue
		}
		s := areatree.Length(0)
		for _, bd := range c.bounds {
			if top+bd.off == off {
				s = bd.skip
			}
		}
		if !found || s < skip {
			skip, found = s, true
		}
	}
	return skip
}

func sortLengths(v []areatree.Length) {
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && v[j] < v[j-1]; j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
}

// emitRow appends the slices of a row to the galley.
func (g *galley) emitRow(r *rowLayout, before int) error {
	if r.node.IsFO("table-row") {
		g.keepPrev = max(g.keepPrev, keepValue(r.node, "keep-with-previous"))
		if b := breakValue(r.node, "break-before"); b != "" && !r.skipBreakBefore {
			g.brk = b
		}
		defer func() {
			g.keepNext = max(g.keepNext, keepValue(r.node, "keep-with-next"))
			if b := breakValue(r.node, "break-after"); b != "" && !r.skipBreakAfter {
				g.brk = b
			}
		}()
	}
	if g.resume != nil && g.major+1 == g.skip.major {
		// Rebuild after width change in the middle of this row: the cells
		// are laid out from the position of the new page with the new width.
		for i, c := range r.cells {
			if i < len(g.resume) && c.rowSpan == 1 {
				if err := g.layoutCellAt(c, c.node, r.tbl, &g.resume[i]); err != nil {
					return err
				}
			}
		}
		finishRow(r, 0, false)
		r.base = g.skip.minor
		g.resume = nil
	}
	ord := g.next()
	if r.leadBrk != "" {
		g.brk = r.leadBrk
	}
	g.keepPrev = max(g.keepPrev, r.leadKeep)
	for k := 0; k+1 < len(r.slices); k++ {
		pen := before
		if k > 0 {
			pen = max(r.keep, r.sliceKeep[k])
			if b, ok := r.forced[k]; ok {
				g.brk = b
			}
		}
		o := ord
		o.minor = r.base + k
		h := r.slices[k+1] - r.slices[k]
		if sk := r.skip[k]; sk > 0 {
			g.spaces = append(g.spaces, spaceSpec{v: sk, min: sk})
			h -= sk
		}
		var rs []resumePoint
		if k > 0 {
			rs = rowResume(r, k)
		}
		g.box(item{h: h, path: g.path(), ord: o, row: r, slice: k, resume: rs}, pen)
	}
	if r.trailBrk != "" {
		g.brk = r.trailBrk
	}
	g.keepNext = max(g.keepNext, r.trailKeep)
	return nil
}

// keepValue translates a keep value into a penalty.
func keepValue(n *tree.Node, prop string) int {
	v, ok := n.Attr(prop)
	if !ok {
		v, ok = n.Attr(prop + ".within-page")
	}
	if !ok {
		v, ok = n.Attr(prop + ".within-column")
	}
	if !ok {
		// CSS2 shorthands: page-break-inside, page-break-before and
		// page-break-after with avoid (XSL 1.1 7.31.16 to 7.31.18).
		short := map[string]string{"keep-together": "page-break-inside",
			"keep-with-previous": "page-break-before", "keep-with-next": "page-break-after"}[prop]
		if sv, _ := n.Attr(short); sv == "avoid" {
			v = "always"
		}
	}
	switch v {
	case "", "auto":
		return 0
	case "always":
		return penaltyInf
	}
	if k, err := strconv.Atoi(v); err == nil && k > 0 {
		return k
	}
	return 0
}

// table generates the galley entries of an fo:table.
func (g *galley) table(n *tree.Node, parent *props.Computed) error {
	p, err := props.Compute(parent, n, g.percentBase(parent))
	if err != nil {
		return err
	}
	collapse := attrOr(n, "border-collapse", "collapse") != "separate"
	tableBorder := p.Border
	if collapse {
		// In the collapse model the table edges belong to the cell borders,
		// padding does not apply to the table (XSL 1.1 7.26.2).
		p.Border = [4]areatree.Border{}
		p.Padding = [4]areatree.Length{}
	}
	var sepH, sepV areatree.Length
	if !collapse {
		if sepH, sepV, err = separation(n, p.FontSize); err != nil {
			return err
		}
	}
	tw := g.contentW(p)
	if p.Width != nil && *p.Width > 0 {
		tw = *p.Width
		p.EndIndent = max(g.refW-p.StartIndent-tw, 0)
	}
	x0 := g.contentX(p)

	tc := &tableCtx{node: n,
		omitHeader: attrOr(n, "table-omit-header-at-break", "false") == "true",
		omitFooter: attrOr(n, "table-omit-footer-at-break", "false") == "true"}

	// Collect sections and rows.
	type rowSrc struct {
		node  *tree.Node // fo:table-row or section for cells without a row
		cells []*tree.Node
		sec   section
		props *props.Computed
		secBg *areatree.Color // background of the section
		// secProps and secNode describe the section (table-header,
		// table-body, table-footer) for the collapse border model.
		secProps *props.Computed
		secNode  *tree.Node
	}
	var columns []*tree.Node
	var rows []rowSrc
	for _, ch := range n.Children {
		if ch.Kind != tree.Element {
			continue
		}
		var sec section
		switch {
		case ch.IsFO("table-column"):
			columns = append(columns, ch)
			continue
		case ch.IsFO("table-header"):
			sec = secHeader
		case ch.IsFO("table-footer"):
			sec = secFooter
		case ch.IsFO("table-body"):
			sec = secBody
		default:
			continue
		}
		sp, err := props.Compute(p, ch, tw)
		if err != nil {
			return err
		}
		var loose []*tree.Node
		for _, r := range ch.Children {
			if r.Kind != tree.Element {
				continue
			}
			if r.IsFO("table-row") {
				rp, err := props.Compute(sp, r, tw)
				if err != nil {
					return err
				}
				var cells []*tree.Node
				for _, c := range r.Children {
					if c.IsFO("table-cell") {
						cells = append(cells, c)
					}
				}
				rows = append(rows, rowSrc{node: r, cells: cells, sec: sec, props: rp, secBg: sp.Background, secProps: sp, secNode: ch})
				continue
			}
			if r.IsFO("table-cell") {
				// Cells directly in the section: starts-row and ends-row split rows.
				if attrOr(r, "starts-row", "false") == "true" && len(loose) > 0 {
					rows = append(rows, rowSrc{node: ch, cells: loose, sec: sec, props: sp, secBg: sp.Background, secProps: sp, secNode: ch})
					loose = nil
				}
				loose = append(loose, r)
				if attrOr(r, "ends-row", "false") == "true" {
					rows = append(rows, rowSrc{node: ch, cells: loose, sec: sec, props: sp, secBg: sp.Background, secProps: sp, secNode: ch})
					loose = nil
				}
			}
		}
		if len(loose) > 0 {
			rows = append(rows, rowSrc{node: ch, cells: loose, sec: sec, props: sp, secBg: sp.Background, secProps: sp, secNode: ch})
		}
	}

	// Rows in display order: header, body, footer. In the source the
	// footer comes before the body, for adjacency (such as borders in the
	// collapse model) the display order counts.
	order := map[section]int{secHeader: 0, secBody: 1, secFooter: 2}
	sort.SliceStable(rows, func(i, j int) bool { return order[rows[i].sec] < order[rows[j].sec] })

	// Column widths.
	// Number of columns from the fo:table-column elements, also with column-number
	// (XSL 1.1 7.28.8: the initial value is the number of the previous column plus 1).
	ncols, next := 0, 1
	for _, c := range columns {
		k := next
		if num, ok := c.Attr("column-number"); ok {
			if v, err := strconv.Atoi(strings.TrimSpace(num)); err == nil && v >= 1 {
				k = v
			}
		}
		next = k + repeated(c)
		ncols = max(ncols, next-1)
	}
	for _, r := range rows {
		n := 0
		for _, c := range r.cells {
			if num, ok := c.Attr("column-number"); ok {
				if v, err := strconv.Atoi(strings.TrimSpace(num)); err == nil && v-1 > n {
					n = v - 1
				}
			}
			n += spanAttr(c, "number-columns-spanned")
		}
		if len(columns) > 0 {
			ncols = max(ncols, n)
		}
	}
	if ncols == 0 {
		for _, r := range rows {
			n := 0
			for _, c := range r.cells {
				n += spanAttr(c, "number-columns-spanned")
			}
			ncols = max(ncols, n)
		}
	}
	if ncols == 0 {
		return tree.Errorf(n, "fo:table ohne Spalten und Zellen")
	}
	// Column widths are grid widths. In the separate model each cell gets
	// half the border-separation on each side (XSL 1.1 6.7.10).
	avail := tw
	widths := make([]areatree.Length, ncols)
	fracs := make([]float64, ncols)
	set := make([]bool, ncols)
	ci := 0
	for _, c := range columns {
		if num, ok := c.Attr("column-number"); ok {
			if k, err := strconv.Atoi(num); err == nil && k >= 1 && k <= ncols {
				ci = k - 1
			}
		}
		v, ok := c.Attr("column-width")
		if !ok {
			ci += repeated(c)
			continue
		}
		fixed, prop, err := props.EvalColumnWidth(v, p.FontSize, tw)
		if err != nil {
			return tree.Errorf(c, "column-width: %v", err)
		}
		for k := 0; k < repeated(c) && ci < ncols; k++ {
			widths[ci], fracs[ci], set[ci] = fixed, prop, true
			ci++
		}
	}
	// Properties of the columns, such as borders for the collapse model.
	colProps := make([]*props.Computed, ncols)
	cj := 0
	for _, c := range columns {
		if num, ok := c.Attr("column-number"); ok {
			if k, err := strconv.Atoi(strings.TrimSpace(num)); err == nil && k >= 1 {
				cj = k - 1
			}
		}
		cp, err := props.Compute(p, c, tw)
		if err != nil {
			return err
		}
		for k := 0; k < repeated(c) && cj < ncols; k++ {
			colProps[cj] = cp
			cj++
		}
	}
	// Columns without column-width take, as in the fixed table layout of
	// CSS2 (17.5.2.1, XSL 1.1 7.28.16), the width of a cell of the first
	// row, otherwise they get a share. XSL 1.1 7.28.9 requires
	// column-width for every column, pdx is lenient here.
	// Cells of the first row of the table, header before body.
	var first []*tree.Node
	for _, want := range []section{secHeader, secBody} {
		for _, r := range rows {
			if r.sec == want && first == nil {
				first = r.cells
			}
		}
	}
	if first != nil {
		col := 0
		for _, cell := range first {
			if num, ok := cell.Attr("column-number"); ok {
				if k, err := strconv.Atoi(strings.TrimSpace(num)); err == nil && k >= 1 {
					col = k - 1
				}
			}
			span := spanAttr(cell, "number-columns-spanned")
			if col < ncols && span == 1 && !set[col] {
				v, ok := cell.Attr("width")
				if !ok {
					v, ok = cell.Attr("inline-progression-dimension")
				}
				if ok && v != "auto" {
					if l, err := props.EvalLength(v, p.FontSize, tw, nil); err == nil {
						widths[col], set[col] = l, true
					}
				}
			}
			col += span
		}
	}
	for i := range set {
		if !set[i] {
			fracs[i] = 1
		}
	}
	var fixedSum areatree.Length
	var propSum float64
	for i := range widths {
		fixedSum += widths[i]
		propSum += fracs[i]
	}
	if propSum > 0 {
		unit := float64(max(avail-fixedSum, 0)) / propSum
		for i := range widths {
			if fracs[i] > 0 {
				widths[i] += areatree.Length(unit * fracs[i])
			}
		}
	}
	colX := make([]areatree.Length, ncols+1)
	colX[0] = x0
	for i := range widths {
		colX[i+1] = colX[i] + widths[i]
	}

	// Fill the grid and create cells.
	type gridCell struct {
		c   *cellLayout
		row int
	}
	grid := make([][]*gridCell, len(rows))
	for i := range grid {
		grid[i] = make([]*gridCell, ncols)
	}
	layouts := make([]*rowLayout, len(rows))
	for ri, rs := range rows {
		rl := &rowLayout{node: rs.node, tbl: tc, sec: rs.sec, sepV: sepV / 2, x: x0, w: tw, collapse: collapse,
			keep: keepValue(rs.node, "keep-together"), bg: rs.props.Background}
		if rs.node.IsFO("table-row") {
			rl.markers = collectMarkers(rs.node)
		}
		if !rs.node.IsFO("table-row") || rl.bg == nil {
			// The background of the section lies below its rows.
			rl.bg = rs.secBg
		}
		col := 0
		for _, cn := range rs.cells {
			if v, ok := cn.Attr("column-number"); ok {
				if k, err := strconv.Atoi(v); err == nil && k >= 1 {
					col = k - 1
				}
			}
			for col < ncols && grid[ri][col] != nil {
				col++
			}
			if col >= ncols {
				return tree.Errorf(cn, "Zelle passt nicht in die %d Spalten der Tabelle", ncols)
			}
			span := min(spanAttr(cn, "number-columns-spanned"), ncols-col)
			rspan := min(spanAttr(cn, "number-rows-spanned"), len(rows)-ri)
			cp, err := props.Compute(rs.props, cn, tw)
			if err != nil {
				return err
			}
			c := &cellLayout{node: cn, p: cp, col: col, span: span, rowSpan: rspan, align: attrOr(cn, "display-align", "before")}
			if c.minH, err = minHeight(cn, cp.FontSize); err != nil {
				return err
			}
			c.x = colX[col] + sepH/2
			c.w = colX[col+span] - colX[col] - sepH
			gc := &gridCell{c: c, row: ri}
			for r := ri; r < ri+rspan; r++ {
				for k := col; k < col+span; k++ {
					if grid[r][k] != nil {
						return tree.Errorf(cn, "Zelle überlappt eine andere Zelle")
					}
					grid[r][k] = gc
				}
			}
			rl.cells = append(rl.cells, c)
			col += span
		}
		layouts[ri] = rl
	}

	// Borders and indents of the cells.
	for ri, rl := range layouts {
		for _, c := range rl.cells {
			if !collapse {
				c.border = c.p.Border
				for s := 0; s < 4; s++ {
					c.inset[s] = c.p.BorderWidth(s) + c.p.Padding[s]
				}
				continue
			}
			lastRow := ri + c.rowSpan - 1
			neighbour := func(r, k int) *cellLayout {
				if r < 0 || r >= len(grid) || k < 0 || k >= ncols || grid[r][k] == nil {
					return nil
				}
				return grid[r][k].c
			}
			// Candidates of an edge per XSL 1.1 6.7.10 with the precedence from
			// the initial values of border-*-precedence (7.28.1).
			var cands []cand
			add := func(b areatree.Border, prec int) { cands = append(cands, cand{b, prec}) }
			secPrec := map[section]int{secBody: precBody, secHeader: precHeader, secFooter: precFooter}
			colB := func(k, side int) {
				if k >= 0 && k < len(colProps) && colProps[k] != nil {
					add(colProps[k].Border[side], precColumn)
				}
			}
			// Top.
			cands = cands[:0]
			add(c.p.Border[areatree.Top], precCell)
			add(rows[ri].props.Border[areatree.Top], precRow)
			for k := c.col; k < c.col+c.span; k++ {
				if o := neighbour(ri-1, k); o != nil {
					add(o.p.Border[areatree.Bottom], precCell)
				}
			}
			if ri > 0 {
				add(rows[ri-1].props.Border[areatree.Bottom], precRow)
			}
			if ri == 0 || rows[ri-1].secNode != rows[ri].secNode {
				add(rows[ri].secProps.Border[areatree.Top], secPrec[rows[ri].sec])
				if ri > 0 {
					add(rows[ri-1].secProps.Border[areatree.Bottom], secPrec[rows[ri-1].sec])
				}
			}
			if ri == 0 {
				add(tableBorder[areatree.Top], precTable)
				for k := c.col; k < c.col+c.span; k++ {
					colB(k, areatree.Top)
				}
			}
			c.border[areatree.Top] = winnerPrec(cands)
			// Bottom.
			cands = cands[:0]
			add(c.p.Border[areatree.Bottom], precCell)
			add(rows[lastRow].props.Border[areatree.Bottom], precRow)
			for k := c.col; k < c.col+c.span; k++ {
				if o := neighbour(lastRow+1, k); o != nil {
					add(o.p.Border[areatree.Top], precCell)
				}
			}
			if lastRow+1 < len(rows) {
				add(rows[lastRow+1].props.Border[areatree.Top], precRow)
			}
			if lastRow == len(rows)-1 || rows[lastRow+1].secNode != rows[lastRow].secNode {
				add(rows[lastRow].secProps.Border[areatree.Bottom], secPrec[rows[lastRow].sec])
				if lastRow+1 < len(rows) {
					add(rows[lastRow+1].secProps.Border[areatree.Top], secPrec[rows[lastRow+1].sec])
				}
			}
			if lastRow == len(rows)-1 {
				add(tableBorder[areatree.Bottom], precTable)
				for k := c.col; k < c.col+c.span; k++ {
					colB(k, areatree.Bottom)
				}
			}
			c.border[areatree.Bottom] = winnerPrec(cands)
			// Left and right.
			// Resolve the sides per spanned row, the widest edge
			// determines the indent of the cell.
			for _, side := range []int{areatree.Left, areatree.Right} {
				other, k, otherSide := c.col-1, c.col, areatree.Right
				outer := c.col == 0
				if side == areatree.Right {
					other, k, otherSide = c.col+c.span, c.col+c.span-1, areatree.Left
					outer = c.col+c.span == ncols
				}
				var best areatree.Border
				for r := ri; r <= lastRow; r++ {
					cands = cands[:0]
					add(c.p.Border[side], precCell)
					colB(k, side)
					if o := neighbour(r, other); o != nil {
						add(o.p.Border[otherSide], precCell)
					}
					colB(other, otherSide)
					if outer {
						add(rows[r].props.Border[side], precRow)
						add(rows[r].secProps.Border[side], secPrec[rows[r].sec])
						add(tableBorder[side], precTable)
					}
					if w := winnerPrec(cands); w.Visible() && (!best.Visible() || w.Width > best.Width) {
						best = w
					}
				}
				c.border[side] = best
			}
			// At top and bottom, for spanned columns the widest edge of the
			// individual columns determines the indent, so that the content lies in
			// no border.
			seg := func(side int) areatree.Length {
				var best areatree.Length
				nb, nbSide, outer := ri-1, areatree.Bottom, ri == 0
				if side == areatree.Bottom {
					nb, nbSide, outer = lastRow+1, areatree.Top, lastRow == len(rows)-1
				}
				for k := c.col; k < c.col+c.span; k++ {
					cands = cands[:0]
					add(c.p.Border[side], precCell)
					if side == areatree.Top {
						add(rows[ri].props.Border[side], precRow)
					} else {
						add(rows[lastRow].props.Border[side], precRow)
					}
					if o := neighbour(nb, k); o != nil {
						add(o.p.Border[nbSide], precCell)
					}
					if nb >= 0 && nb < len(rows) {
						add(rows[nb].props.Border[nbSide], precRow)
					}
					if outer {
						add(tableBorder[side], precTable)
						colB(k, side)
					}
					if w := winnerPrec(cands); w.Visible() {
						best = max(best, w.Width)
					}
				}
				return best
			}
			for s := 0; s < 4; s++ {
				w := areatree.Length(0)
				if c.border[s].Visible() {
					w = c.border[s].Width
				}
				if c.span > 1 && (s == areatree.Top || s == areatree.Bottom) {
					w = max(w, seg(s))
				}
				c.inset[s] = w/2 + c.p.Padding[s]
			}
		}
	}

	// Lay out content, row heights and spans.
	spannedUntil := -1
	for ri, rl := range layouts {
		for _, c := range rl.cells {
			var tbl *tableCtx
			if rl.sec != secBody {
				tbl = tc
			}
			if err := g.layoutCell(c, c.node, tbl); err != nil {
				return err
			}
			if c.rowSpan > 1 {
				spannedUntil = max(spannedUntil, ri+c.rowSpan-1)
			}
		}
		minH, err := rowHeight(rows[ri].node, p.FontSize)
		if err != nil {
			return err
		}
		finishRow(rl, minH, ri < spannedUntil || hasSpanStart(rl))
		rl.last = ri == len(rows)-1 || rows[ri+1].sec != rows[ri].sec
	}
	for ri, rl := range layouts {
		for _, c := range rl.cells {
			span := layouts[ri : ri+c.rowSpan]
			var have areatree.Length
			for _, r := range span {
				have += r.h
			}
			have -= span[0].sepV + span[len(span)-1].sepV
			need := c.inset[areatree.Top] + max(c.contentH, c.minH) + c.inset[areatree.Bottom]
			if need > have {
				last := span[len(span)-1]
				last.h += need - have
				last.slices[len(last.slices)-1] = last.h
				have = need
			}
			c.boxH = have
		}
	}

	// Order sections: header, body, footer.
	var header, body, footer []*rowLayout
	for _, rl := range layouts {
		switch rl.sec {
		case secHeader:
			header = append(header, rl)
		case secFooter:
			footer = append(footer, rl)
		default:
			body = append(body, rl)
		}
	}
	tc.header, tc.footer = header, footer
	for _, r := range append(append([]*rowLayout(nil), header...), footer...) {
		for _, c := range r.cells {
			if hasTableMarker(c.node) {
				tc.dynamic = true
			}
		}
	}
	if len(body) > 0 {
		tc.firstBody, tc.lastBody = body[0], body[len(body)-1]
	}
	// The page on which the table starts shows in the header the markers of the
	// first body row, so measuring used the first marker of the
	// class. Repeated headers on following pages reserve the height of the
	// first non-empty marker, so a carry-over that is empty on the first
	// page does not lack space on the following pages.
	if tc.dynamic && len(header) > 0 {
		rep, err := g.repeatRows(tc, header, p.FontSize)
		if err != nil {
			return err
		}
		tc.header = rep
	}
	for _, r := range tc.header {
		tc.headerH += r.h
	}
	for _, r := range footer {
		tc.footerH += r.h
	}

	// Table as a block with edges into the galley.
	ctx := &blockCtx{node: n, p: p}
	ctx.x = x0 - p.Padding[areatree.Left]
	ctx.w = tw + p.Padding[areatree.Left] + p.Padding[areatree.Right]
	g.push(ctx)
	g.spaceBefore(p)
	// A forced break at the start of the first body row applies before the
	// table, otherwise the header would stay alone on the page.
	// If a break before the table is already pending, both coincide.
	if len(body) > 0 {
		if body[0].leadBrk != "" {
			if g.brk == "" {
				g.brk = body[0].leadBrk
			}
			body[0].leadBrk = ""
		} else if b := breakValue(body[0].node, "break-before"); b != "" && body[0].node.IsFO("table-row") {
			if g.brk == "" {
				g.brk = b
			}
			body[0].skipBreakBefore = true
		}
	}
	edges := hasBox(p)
	if edges {
		h := p.BorderWidth(areatree.Top) + p.Padding[areatree.Top]
		g.box(item{h: h, path: g.path(), edge: edgeTop, ord: g.next()}, g.breakBefore(edgeTop))
	}
	keepRows := keepValue(n, "keep-together")
	glueNext := false // rows of a spanning range stay together
	emit := func(rs []*rowLayout, sec section) error {
		for i, r := range rs {
			before := keepRows
			if sec != secBody || glueNext || i == 0 && sec == secBody && len(header) > 0 {
				before = penaltyInf
			}
			if sec == secFooter && i == 0 {
				before = penaltyInf
			}
			before = max(before, g.breakBefore(edgeNone))
			if err := g.emitRow(r, before); err != nil {
				return err
			}
			glueNext = hasSpanStart(r) || spansInto(layouts, r)
		}
		return nil
	}
	// A forced break at the end of the last body row takes effect after the
	// table, otherwise the footer would stand alone on the next page.
	var after string
	if len(body) > 0 {
		last := body[len(body)-1]
		if last.trailBrk != "" {
			after, last.trailBrk = last.trailBrk, ""
		}
		if b := breakValue(last.node, "break-after"); b != "" && last.node.IsFO("table-row") {
			after = b
			last.skipBreakAfter = true
		}
	}
	for _, part := range []struct {
		rs  []*rowLayout
		sec section
	}{{header, secHeader}, {body, secBody}, {footer, secFooter}} {
		if err := emit(part.rs, part.sec); err != nil {
			return err
		}
	}
	if edges {
		h := p.Padding[areatree.Bottom] + p.BorderWidth(areatree.Bottom)
		g.box(item{h: h, path: g.path(), edge: edgeBottom, ord: g.next()}, penaltyInf)
	}
	g.pop()
	if after != "" && g.brk == "" {
		g.brk = after
	}
	g.spaceAfter(p)
	return nil
}

func hasSpanStart(r *rowLayout) bool {
	for _, c := range r.cells {
		if c.rowSpan > 1 {
			return true
		}
	}
	return false
}

// spansInto reports whether a cell of an earlier row extends beyond r.
func spansInto(all []*rowLayout, r *rowLayout) bool {
	for i, x := range all {
		if x == r {
			for j := 0; j < i; j++ {
				for _, c := range all[j].cells {
					if j+c.rowSpan-1 > i {
						return true
					}
				}
			}
		}
	}
	return false
}

func repeated(c *tree.Node) int {
	n, err := strconv.Atoi(attrOr(c, "number-columns-repeated", "1"))
	if err != nil || n < 1 {
		return 1
	}
	return n
}

func spanAttr(c *tree.Node, name string) int {
	n, err := strconv.Atoi(attrOr(c, name, "1"))
	if err != nil || n < 1 {
		return 1
	}
	return n
}

// rowHeight reads height or block-progression-dimension(.minimum) of an
// fo:table-row as the minimum height.
func rowHeight(n *tree.Node, fs areatree.Length) (areatree.Length, error) {
	if !n.IsFO("table-row") {
		return 0, nil
	}
	return minHeight(n, fs)
}

// minHeight reads the minimum height from height or block-progression-dimension
// (XSL 1.1 7.15.3, 7.15.6).
func minHeight(n *tree.Node, fs areatree.Length) (areatree.Length, error) {
	for _, name := range []string{"height", "block-progression-dimension", "block-progression-dimension.optimum", "block-progression-dimension.minimum"} {
		if v, ok := n.Attr(name); ok && v != "auto" {
			l, err := props.EvalLength(v, fs, -1, nil)
			if err != nil {
				return 0, tree.Errorf(n, "%s: %v", name, err)
			}
			return l, nil
		}
	}
	return 0, nil
}

// separation reads border-separation (XSL 1.1 7.28.5) and border-spacing.
func separation(n *tree.Node, fs areatree.Length) (areatree.Length, areatree.Length, error) {
	parse := func(v string) (areatree.Length, error) {
		l, err := props.EvalLength(v, fs, -1, nil)
		if err != nil {
			return 0, tree.Errorf(n, "border-separation: %v", err)
		}
		return l, nil
	}
	var h, v areatree.Length
	var err error
	// border-spacing is the CSS2 shorthand (XSL 1.1 7.31.9), the
	// longhand properties take precedence.
	s, ok := n.Attr("border-separation")
	if !ok {
		s, ok = n.Attr("border-spacing")
	}
	if ok {
		parts := strings.Fields(s)
		if h, err = parse(parts[0]); err != nil {
			return 0, 0, err
		}
		v = h
		if len(parts) > 1 {
			if v, err = parse(parts[1]); err != nil {
				return 0, 0, err
			}
		}
	}
	if s, ok := n.Attr("border-separation.inline-progression-direction"); ok {
		if h, err = parse(s); err != nil {
			return 0, 0, err
		}
	}
	if s, ok := n.Attr("border-separation.block-progression-direction"); ok {
		if v, err = parse(s); err != nil {
			return 0, 0, err
		}
	}
	return h, v, nil
}

var stylePriority = map[areatree.BorderStyle]int{
	areatree.Double: 5, areatree.Solid: 4, areatree.Dashed: 3, areatree.Dotted: 2, areatree.None: 0,
}

// winner chooses the stronger edge per XSL 1.1 6.7.10 (collapse model): the
// wider one, at equal width the one with higher style rank, else the first.
// Precedences per the initial values of border-*-precedence (XSL 1.1 7.28.1).
const (
	precFooter = iota
	precHeader
	precBody
	precRow
	precColumn
	precCell
	precTable
)

// cand is a border with its precedence.
type cand struct {
	b    areatree.Border
	prec int
}

// winnerPrec resolves an edge in the collapse model: the wider border,
// then the stronger style, then the higher precedence (XSL 1.1 6.7.10, CSS2
// 17.6.2.1).
func winnerPrec(cands []cand) areatree.Border {
	var best cand
	found := false
	for _, c := range cands {
		if c.b.Style == areatree.Hidden {
			return areatree.Border{}
		}
	}
	for _, c := range cands {
		if !c.b.Visible() {
			continue
		}
		if !found || c.b.Width > best.b.Width ||
			c.b.Width == best.b.Width && stylePriority[c.b.Style] > stylePriority[best.b.Style] ||
			c.b.Width == best.b.Width && stylePriority[c.b.Style] == stylePriority[best.b.Style] && c.prec > best.prec {
			best, found = c, true
		}
	}
	return best.b
}

func winner(cands []areatree.Border) areatree.Border {
	var best areatree.Border
	for _, c := range cands {
		if !c.Visible() {
			continue
		}
		if !best.Visible() || c.Width > best.Width || c.Width == best.Width && stylePriority[c.Style] > stylePriority[best.Style] {
			best = c
		}
	}
	return best
}

// list generates the galley entries of an fo:list-block. Each list item is
// a row with the cells label and body, their position results from the
// indents, typically via label-end() and body-start().
func (g *galley) list(n *tree.Node, parent *props.Computed) error {
	p, err := props.Compute(parent, n, g.percentBase(parent))
	if err != nil {
		return err
	}
	// Percentages refer to the width of the nearest enclosing
	// block area (XSL 1.1 7.30.11 and 7.30.12).
	pdbs, err := listLength(n, "provisional-distance-between-starts", "24pt", p.FontSize, g.refW)
	if err != nil {
		return err
	}
	pls, err := listLength(n, "provisional-label-separation", "6pt", p.FontSize, g.refW)
	if err != nil {
		return err
	}
	env := &props.Env{
		BodyStart: func() (areatree.Length, error) { return p.StartIndent + pdbs, nil },
		LabelEnd: func() (areatree.Length, error) {
			return g.refW - (p.StartIndent + pdbs - pls), nil
		},
	}
	ctx := &blockCtx{node: n, p: p}
	ctx.x = g.contentX(p) - p.Padding[areatree.Left]
	ctx.w = g.contentW(p) + p.Padding[areatree.Left] + p.Padding[areatree.Right]
	g.push(ctx)
	g.spaceBefore(p)
	if hasBox(p) {
		h := p.BorderWidth(areatree.Top) + p.Padding[areatree.Top]
		g.box(item{h: h, path: g.path(), edge: edgeTop, ord: g.next()}, g.breakBefore(edgeTop))
	}
	for _, it := range n.Children {
		if !it.IsFO("list-item") {
			continue
		}
		ip, err := props.Compute(p, it, g.contentW(p))
		if err != nil {
			return err
		}
		ictx := &blockCtx{node: it, p: ip}
		ictx.x = g.contentX(ip) - ip.Padding[areatree.Left]
		ictx.w = g.contentW(ip) + ip.Padding[areatree.Left] + ip.Padding[areatree.Right]
		g.push(ictx)
		g.spaceBefore(ip)
		// Border and padding of the list item at top and bottom as for fo:block.
		itemEdges := hasBox(ip)
		if itemEdges {
			h := ip.BorderWidth(areatree.Top) + ip.Padding[areatree.Top]
			g.box(item{h: h, path: g.path(), edge: edgeTop, ord: g.next()}, g.breakBefore(edgeTop))
		}
		row := &rowLayout{node: it, keep: keepValue(it, "keep-together"), w: g.refW}
		for _, part := range it.Children {
			if !part.IsFO("list-item-label") && !part.IsFO("list-item-body") {
				continue
			}
			pp, err := props.ComputeEnv(ip, part, g.contentW(ip), env)
			if err != nil {
				return err
			}
			c := &cellLayout{node: part, p: pp, span: 1, rowSpan: 1, align: "before"}
			c.x, c.w = g.contentX(pp), g.contentW(pp)
			if err := g.layoutCell(c, part, nil); err != nil {
				return err
			}
			row.cells = append(row.cells, c)
		}
		if len(row.cells) != 2 {
			return tree.Errorf(it, "fo:list-item braucht fo:list-item-label und fo:list-item-body")
		}
		finishRow(row, 0, false)
		if err := g.emitRow(row, g.breakBefore(edgeNone)); err != nil {
			return err
		}
		if itemEdges {
			h := ip.Padding[areatree.Bottom] + ip.BorderWidth(areatree.Bottom)
			g.box(item{h: h, path: g.path(), edge: edgeBottom, ord: g.next()}, g.breakBefore(edgeBottom))
		}
		g.pop()
		g.spaceAfter(ip)
	}
	if hasBox(p) {
		h := p.Padding[areatree.Bottom] + p.BorderWidth(areatree.Bottom)
		g.box(item{h: h, path: g.path(), edge: edgeBottom, ord: g.next()}, g.breakBefore(edgeBottom))
	}
	g.pop()
	g.spaceAfter(p)
	return nil
}

// listLength reads an inherited list property from the element or its
// ancestors.
func listLength(n *tree.Node, name, def string, fs, base areatree.Length) (areatree.Length, error) {
	v := def
	for a := n; a != nil; a = a.Parent {
		if x, ok := a.Attr(name); ok {
			v = x
			break
		}
	}
	l, err := props.EvalLength(v, fs, base, nil)
	if err != nil {
		return 0, tree.Errorf(n, "%s: %v", name, err)
	}
	return l, nil
}

// relayout lays out a header or footer row again for a page so that
// fo:retrieve-table-marker inserts the markers of this page. The height of
// the row stays reserved as at page breaking.
func (r *rowLayout) relayout(e *engine, page *areatree.Page) (*rowLayout, error) {
	nr := *r
	nr.cells = make([]*cellLayout, len(r.cells))
	for i, c := range r.cells {
		nc := *c
		w := max(c.w-c.inset[areatree.Left]-c.inset[areatree.Right], 0)
		inside := *c.p
		inside.StartIndent, inside.EndIndent = 0, 0
		sg := newGalley(e, w, page.Number)
		sg.page, sg.tbl = page, r.tbl
		if err := sg.blocks(c.node, &inside); err != nil {
			return nil, err
		}
		nc.items = sg.items
		nc.contentH, nc.bounds = measure(sg.items)
		nr.cells[i] = &nc
	}
	return &nr, nil
}
