package layout

import (
	"strconv"
	"strings"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/fo/props"
	"github.com/tim-riep/pdx/fo/tree"
)

// Tabellen und Listen teilen sich das Zeilenmodell: Eine Zeile besteht aus
// Zellen nebeneinander, deren Inhalt jeweils in einer eigenen Galley gesetzt
// wird. In der Galley des Flusses steht eine Zeile als Folge von Scheiben.
// Zwischen zwei Scheiben darf umbrochen werden, an ihren Grenzen hat jede
// Zelle eine erlaubte Umbruchstelle oder keinen Inhalt mehr.

type section uint8

const (
	secBody section = iota
	secHeader
	secFooter
)

// tableCtx hält die Daten einer Tabelle für Wiederholung und Seitenumbruch.
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
	// dynamic ist gesetzt, wenn Kopf oder Fuß fo:retrieve-table-marker
	// enthalten. Sie werden dann je Seite neu gesetzt.
	dynamic bool
	frags   []*tableFrag
	// representative ist gesetzt, solange wiederholte Köpfe gemessen werden.
	// sample überspringt dann leere Marker.
	representative bool
}

// continuesAfter meldet, ob nach dieser Scheibe noch Zeilen des Rumpfs folgen.
func (t *tableCtx) continuesAfter(r *rowLayout, slice int) bool {
	return !(r == t.lastBody && slice == len(r.slices)-2)
}

// rowLayout ist eine gesetzte Zeile einer Tabelle oder ein Listenpunkt.
type rowLayout struct {
	node     *tree.Node
	tbl      *tableCtx // nil bei Listen
	sec      section
	cells    []*cellLayout // Zellen, die in dieser Zeile beginnen
	h        areatree.Length
	sepV     areatree.Length // halbe border-separation über und unter den Zellen
	bg       *areatree.Color
	x, w     areatree.Length   // Zeilenrechteck relativ zum Referenzbereich
	slices   []areatree.Length // Grenzen der Scheiben, beginnend bei 0, endend bei h
	last     bool              // letzte Zeile ihres Abschnitts
	keep     int               // Penalty zwischen den Scheiben
	markers  markerSet
	collapse bool
}

// cellLayout ist eine gesetzte Zelle.
type cellLayout struct {
	node     *tree.Node
	p        *props.Computed
	col      int
	span     int
	rowSpan  int
	x, w     areatree.Length // Zellrechteck (separate: Rahmenbox, collapse: Raster)
	boxH     areatree.Length // Höhe des Zellrechtecks über alle überspannten Zeilen
	border   [4]areatree.Border
	inset    [4]areatree.Length // Rahmen (ganz oder halb) plus Padding
	items    []item
	contentH areatree.Length
	bounds   []bound
	align    string
}

// bound ist eine erlaubte Umbruchstelle im Inhalt einer Zelle.
type bound struct {
	off   areatree.Length // Abstand vom Inhaltsanfang
	index int             // Index der Penalty in items
}

// measure berechnet die Inhaltshöhe und die erlaubten Umbruchstellen einer
// Galley so, wie der Placer sie setzen würde.
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
				bs = append(bs, bound{off: y, index: i})
			}
		}
	}
	return y, bs
}

// cellRange liefert die Einträge einer Zelle für den Inhaltsbereich [a, b).
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

// cellBreaks liefert die Umbruchstellen einer Zelle in Zeilenkoordinaten.
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

// layoutCell setzt den Inhalt einer Zelle in einer eigenen Galley. Die Zelle
// ist ein Referenzbereich, Einzüge beginnen darin bei 0.
func (g *galley) layoutCell(c *cellLayout, content *tree.Node, tbl *tableCtx) error {
	w := max(c.w-c.inset[areatree.Left]-c.inset[areatree.Right], 0)
	inside := *c.p
	inside.StartIndent, inside.EndIndent = 0, 0
	sg := newGalley(g.e, w, g.pageNo)
	sg.tbl = tbl
	if err := sg.blocks(content, &inside); err != nil {
		return err
	}
	c.items = sg.items
	c.contentH, c.bounds = measure(sg.items)
	return nil
}

// repeatRows misst Kopfzeilen mit Tabellenmarkern für die Wiederholung neu.
// Die Zeilen werden nicht niedriger als beim ersten Setzen.
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

// finishRow berechnet Höhe und Scheiben einer Zeile.
func finishRow(r *rowLayout, minH areatree.Length, spanned bool) {
	h := minH
	for _, c := range r.cells {
		if c.rowSpan > 1 {
			continue
		}
		h = max(h, c.inset[areatree.Top]+c.contentH+c.inset[areatree.Bottom])
	}
	r.h = r.sepV + h + r.sepV
	r.slices = []areatree.Length{0}
	if !spanned && r.keep < penaltyInf {
		// Kandidaten sind alle Umbruchstellen aller Zellen.
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
			}
		}
	}
	r.slices = append(r.slices, r.h)
}

func sortLengths(v []areatree.Length) {
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && v[j] < v[j-1]; j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
}

// emitRow hängt die Scheiben einer Zeile an die Galley.
func (g *galley) emitRow(r *rowLayout, before int) {
	if r.node.IsFO("table-row") {
		g.keepPrev = max(g.keepPrev, keepValue(r.node, "keep-with-previous"))
		if b := breakValue(r.node, "break-before"); b != "" {
			g.brk = b
		}
		defer func() {
			g.keepNext = max(g.keepNext, keepValue(r.node, "keep-with-next"))
			if b := breakValue(r.node, "break-after"); b != "" {
				g.brk = b
			}
		}()
	}
	ord := g.next()
	for k := 0; k+1 < len(r.slices); k++ {
		pen := before
		if k > 0 {
			pen = r.keep
		}
		o := ord
		o.minor = k
		g.box(item{h: r.slices[k+1] - r.slices[k], path: g.path(), ord: o, row: r, slice: k}, pen)
	}
}

// keepValue übersetzt einen keep-Wert in eine Penalty.
func keepValue(n *tree.Node, prop string) int {
	v, ok := n.Attr(prop)
	if !ok {
		v, ok = n.Attr(prop + ".within-page")
	}
	if !ok {
		v, _ = n.Attr(prop + ".within-column")
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

// table erzeugt die Galley-Einträge einer fo:table.
func (g *galley) table(n *tree.Node, parent *props.Computed) error {
	p, err := props.Compute(parent, n, g.percentBase(parent))
	if err != nil {
		return err
	}
	collapse := attrOr(n, "border-collapse", "collapse") != "separate"
	tableBorder := p.Border
	if collapse {
		// Im Modell collapse gehören die Tabellenkanten zu den Zellrahmen,
		// Padding gilt für die Tabelle nicht (XSL 1.1 7.26.2).
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

	// Abschnitte und Zeilen sammeln.
	type rowSrc struct {
		node  *tree.Node // fo:table-row oder Abschnitt bei Zellen ohne Zeile
		cells []*tree.Node
		sec   section
		props *props.Computed
		secBg *areatree.Color // Hintergrund des Abschnitts
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
				rows = append(rows, rowSrc{node: r, cells: cells, sec: sec, props: rp, secBg: sp.Background})
				continue
			}
			if r.IsFO("table-cell") {
				// Zellen direkt im Abschnitt: starts-row und ends-row teilen Zeilen.
				if attrOr(r, "starts-row", "false") == "true" && len(loose) > 0 {
					rows = append(rows, rowSrc{node: ch, cells: loose, sec: sec, props: sp, secBg: sp.Background})
					loose = nil
				}
				loose = append(loose, r)
				if attrOr(r, "ends-row", "false") == "true" {
					rows = append(rows, rowSrc{node: ch, cells: loose, sec: sec, props: sp, secBg: sp.Background})
					loose = nil
				}
			}
		}
		if len(loose) > 0 {
			rows = append(rows, rowSrc{node: ch, cells: loose, sec: sec, props: sp, secBg: sp.Background})
		}
	}

	// Spaltenbreiten.
	// Spaltenzahl aus den fo:table-column, auch mit column-number (XSL 1.1
	// 7.28.8: Anfangswert ist die Nummer der vorigen Spalte plus 1).
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
	// Spaltenbreiten sind Rasterbreiten. Im Modell separate erhält jede Zelle
	// auf jeder Seite die halbe border-separation (XSL 1.1 6.7.10).
	avail := tw
	widths := make([]areatree.Length, ncols)
	fracs := make([]float64, ncols)
	set := make([]bool, ncols)
	ci := 0
	for _, c := range columns {
		v := attrOr(c, "column-width", "proportional-column-width(1)")
		fixed, prop, err := props.EvalColumnWidth(v, p.FontSize, tw)
		if err != nil {
			return tree.Errorf(c, "column-width: %v", err)
		}
		if num, ok := c.Attr("column-number"); ok {
			if k, err := strconv.Atoi(num); err == nil && k >= 1 && k <= ncols {
				ci = k - 1
			}
		}
		for k := 0; k < repeated(c) && ci < ncols; k++ {
			widths[ci], fracs[ci], set[ci] = fixed, prop, true
			ci++
		}
	}
	// Spalten ohne fo:table-column erhalten einen Anteil. XSL 1.1 7.28.9
	// verlangt column-width für jede Spalte, pdx ist hier nachsichtig.
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

	// Raster belegen und Zellen anlegen.
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
			// Hintergrund des Abschnitts liegt unter seinen Zeilen.
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

	// Rahmen und Einzüge der Zellen.
	for ri, rl := range layouts {
		for _, c := range rl.cells {
			if !collapse {
				c.border = c.p.Border
				for s := 0; s < 4; s++ {
					c.inset[s] = c.p.BorderWidth(s) + c.p.Padding[s]
				}
				continue
			}
			rowP := rows[ri].props
			lastRow := ri + c.rowSpan - 1
			neighbour := func(r, k int) *cellLayout {
				if r < 0 || r >= len(grid) || k < 0 || k >= ncols || grid[r][k] == nil {
					return nil
				}
				return grid[r][k].c
			}
			edge := func(own areatree.Border, other *cellLayout, otherSide int, outer areatree.Border, isOuter bool, rowB areatree.Border) areatree.Border {
				cands := []areatree.Border{own, rowB}
				if other != nil {
					cands = append(cands, other.p.Border[otherSide])
				}
				if isOuter {
					cands = append(cands, outer)
				}
				return winner(cands)
			}
			top := neighbour(ri-1, c.col)
			bottom := neighbour(lastRow+1, c.col)
			left := neighbour(ri, c.col-1)
			right := neighbour(ri, c.col+c.span)
			c.border[areatree.Top] = edge(c.p.Border[areatree.Top], top, areatree.Bottom, tableBorder[areatree.Top], ri == 0 || rows[ri-1].sec != rows[ri].sec && top == nil, rowP.Border[areatree.Top])
			c.border[areatree.Bottom] = edge(c.p.Border[areatree.Bottom], bottom, areatree.Top, tableBorder[areatree.Bottom], lastRow == len(rows)-1, rows[lastRow].props.Border[areatree.Bottom])
			c.border[areatree.Left] = edge(c.p.Border[areatree.Left], left, areatree.Right, tableBorder[areatree.Left], c.col == 0, areatree.Border{})
			c.border[areatree.Right] = edge(c.p.Border[areatree.Right], right, areatree.Left, tableBorder[areatree.Right], c.col+c.span == ncols, areatree.Border{})
			for s := 0; s < 4; s++ {
				w := areatree.Length(0)
				if c.border[s].Visible() {
					w = c.border[s].Width / 2
				}
				c.inset[s] = w + c.p.Padding[s]
			}
		}
	}

	// Inhalte setzen, Zeilenhöhen und Überspannungen.
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
			need := c.inset[areatree.Top] + c.contentH + c.inset[areatree.Bottom]
			if need > have {
				last := span[len(span)-1]
				last.h += need - have
				last.slices[len(last.slices)-1] = last.h
				have = need
			}
			c.boxH = have
		}
	}

	// Abschnitte ordnen: Kopf, Rumpf, Fuß.
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
	// Die Seite, auf der die Tabelle beginnt, zeigt im Kopf die Marker der
	// ersten Rumpfzeile, gemessen wurde daher mit dem ersten Marker der
	// Klasse. Wiederholte Köpfe auf Folgeseiten reservieren die Höhe des
	// ersten nicht leeren Markers, so bleibt ein Übertrag, der auf der ersten
	// Seite leer ist, auf den Folgeseiten nicht ohne Platz.
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

	// Tabelle als Block mit Kanten in die Galley.
	ctx := &blockCtx{node: n, p: p}
	ctx.x = x0 - p.Padding[areatree.Left]
	ctx.w = tw + p.Padding[areatree.Left] + p.Padding[areatree.Right]
	g.push(ctx)
	g.spaceBefore(p)
	edges := hasBox(p)
	if edges {
		h := p.BorderWidth(areatree.Top) + p.Padding[areatree.Top]
		g.box(item{h: h, path: g.path(), edge: edgeTop, ord: g.next()}, g.breakBefore(edgeTop))
	}
	keepRows := keepValue(n, "keep-together")
	glueNext := false // Zeilen eines Überspannungsbereichs bleiben zusammen
	emit := func(rs []*rowLayout, sec section) {
		for i, r := range rs {
			before := keepRows
			if sec != secBody || glueNext || i == 0 && sec == secBody && len(header) > 0 {
				before = penaltyInf
			}
			if sec == secFooter && i == 0 {
				before = penaltyInf
			}
			before = max(before, g.breakBefore(edgeNone))
			g.emitRow(r, before)
			glueNext = hasSpanStart(r) || spansInto(layouts, r)
		}
	}
	emit(header, secHeader)
	emit(body, secBody)
	emit(footer, secFooter)
	if edges {
		h := p.Padding[areatree.Bottom] + p.BorderWidth(areatree.Bottom)
		g.box(item{h: h, path: g.path(), edge: edgeBottom, ord: g.next()}, penaltyInf)
	}
	g.pop()
	g.pending = max(g.pending, p.SpaceAfter)
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

// spansInto meldet, ob eine Zelle einer früheren Zeile über r hinausreicht.
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

// rowHeight liest height oder block-progression-dimension(.minimum) einer
// fo:table-row als Mindesthöhe.
func rowHeight(n *tree.Node, fs areatree.Length) (areatree.Length, error) {
	if !n.IsFO("table-row") {
		return 0, nil
	}
	for _, name := range []string{"height", "block-progression-dimension", "block-progression-dimension.minimum"} {
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

// separation liest border-separation (XSL 1.1 7.26.5).
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
	if s, ok := n.Attr("border-separation"); ok {
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

// winner wählt nach XSL 1.1 6.7.10 (Modell collapse) die stärkere Kante: die
// breitere, bei gleicher Breite die mit höherem Stilrang, sonst die erste.
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

// list erzeugt die Galley-Einträge eines fo:list-block. Jeder Listenpunkt ist
// eine Zeile mit den Zellen Label und Body, ihre Lage ergibt sich aus den
// Einzügen, typischerweise über label-end() und body-start().
func (g *galley) list(n *tree.Node, parent *props.Computed) error {
	p, err := props.Compute(parent, n, g.percentBase(parent))
	if err != nil {
		return err
	}
	// Prozentangaben beziehen sich auf die Breite des nächsten umgebenden
	// Blockbereichs (XSL 1.1 7.30.11 und 7.30.12).
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
		// Rahmen und Padding des Listenpunkts oben und unten wie bei fo:block.
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
		g.emitRow(row, g.breakBefore(edgeNone))
		if itemEdges {
			h := ip.Padding[areatree.Bottom] + ip.BorderWidth(areatree.Bottom)
			g.box(item{h: h, path: g.path(), edge: edgeBottom, ord: g.next()}, g.breakBefore(edgeBottom))
		}
		g.pop()
		g.pending = max(g.pending, ip.SpaceAfter)
	}
	if hasBox(p) {
		h := p.Padding[areatree.Bottom] + p.BorderWidth(areatree.Bottom)
		g.box(item{h: h, path: g.path(), edge: edgeBottom, ord: g.next()}, g.breakBefore(edgeBottom))
	}
	g.pop()
	g.pending = max(g.pending, p.SpaceAfter)
	return nil
}

// listLength liest eine geerbte Listeneigenschaft vom Element oder seinen
// Vorfahren.
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

// relayout setzt eine Zeile von Kopf oder Fuß für eine Seite neu, damit
// fo:retrieve-table-marker die Marker dieser Seite einsetzen. Die Höhe der
// Zeile bleibt wie beim Seitenumbruch reserviert.
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
