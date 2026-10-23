package compile

import (
	"strconv"
	"strings"
)

// tableCtx holds the state of a table during translation.
type tableCtx struct {
	n *node
	// Variable names without $: data nodes, context of the table, groups.
	rows, ctx, groups, group string
	// sums are the distinct of expressions. Their index forms the names
	// of the running sums.
	sums []string
	// carries and subtotals carry one marker per data row.
	carries, subtotals []*node
	// labels are the remaining cells of header rows with carryOver. Their
	// content also comes through markers, so that the carry-over row stays
	// empty on the first page.
	labels []*node
	// inGroup is set while the group header or group footer is being
	// translated. total then sums over the group.
	inGroup bool
}

func (tc *tableCtx) sumIndex(of string) int {
	for i, s := range tc.sums {
		if s == of {
			return i + 1
		}
	}
	tc.sums = append(tc.sums, of)
	return len(tc.sums)
}

func (tc *tableCtx) sumVar(k int) string  { return "sum-" + tc.n.ID() + "-" + strconv.Itoa(k) }
func (tc *tableCtx) nextVar(k int) string { return "next-" + tc.n.ID() + "-" + strconv.Itoa(k) }
func (tc *tableCtx) indexVar() string     { return "index-" + tc.n.ID() }

func (c *compiler) tableElem(n *node) {
	prev := c.table
	defer func() { c.table = prev }()
	id := n.ID()
	tc := &tableCtx{n: n, rows: "rows-" + id, ctx: "context-" + id, groups: "groups-" + id, group: "group-" + id}
	c.table = tc
	c.collectSums(n)

	c.w.empty("xsl:variable", "name", tc.rows, "select", n.Get("bind", ""))
	if len(tc.labels) > 0 {
		c.w.empty("xsl:variable", "name", tc.ctx, "select", ".")
	}
	attrs := []string{"pdx:id", id, "table-layout", "fixed", "width", n.Get("width", "100%"),
		"border-collapse", n.Get("borders", "collapse")}
	if h := n.First("header"); h != nil && h.Get("repeat", "true") == "false" {
		attrs = append(attrs, "table-omit-header-at-break", "true")
	}
	if f := n.First("footer"); f != nil && f.Get("repeat", "true") == "false" {
		attrs = append(attrs, "table-omit-footer-at-break", "true")
	}
	c.w.start("fo:table", append(attrs, c.blockAttrs(n)...)...)
	c.altBind(n)
	cols := n.Elements("column")
	for _, col := range cols {
		w := col.Get("width", "1*")
		if strings.HasSuffix(w, "*") {
			w = "proportional-column-width(" + strings.TrimSuffix(w, "*") + ")"
		}
		i := c.w.enter(col.ID())
		c.w.empty("fo:table-column", "pdx:id", col.ID(), "column-width", w)
		c.w.leave(i)
	}
	for _, sec := range []struct{ name, fo string }{{"header", "fo:table-header"}, {"footer", "fo:table-footer"}} {
		if s := n.First(sec.name); s != nil {
			i := c.open(s.ID(), sec.fo, "pdx:id", s.ID())
			for _, r := range s.Elements("row") {
				c.row(r, nil)
			}
			c.close(i)
		}
	}
	rows := n.First("rows")
	b := c.open(rows.ID(), "fo:table-body", "pdx:id", rows.ID())
	c.w.start("xsl:if", "test", "empty($"+tc.rows+")")
	c.w.start("fo:table-row", "pdx:id", rows.ID())
	c.w.start("fo:table-cell", "pdx:id", rows.ID(), "number-columns-spanned", strconv.Itoa(len(cols)))
	c.w.empty("fo:block", "pdx:id", rows.ID())
	c.w.end()
	c.w.end()
	c.w.end()
	if g := n.First("group"); g != nil {
		c.groupIterate(g, rows)
	} else {
		init := make([]string, len(tc.sums))
		for i := range init {
			init[i] = "0"
		}
		c.rowsIterate(rows, "$"+tc.rows, "1", init)
	}
	c.close(b)
	c.w.end()
}

// collectSums collects sum elements, checks their position and determines
// the cells whose content comes through markers.
func (c *compiler) collectSums(n *node) {
	tc := c.table
	var walk func(x *node, section string)
	walk = func(x *node, section string) {
		for _, ch := range x.Elements("") {
			switch ch.Name {
			case "table":
				continue // separate table with its own sums
			case "carryOver":
				if section != "header" {
					c.errorf(ch, "carryOver ist nur im Tabellenkopf erlaubt")
				}
				tc.sumIndex(ch.Get("of", ""))
				tc.carries = append(tc.carries, ch)
			case "subtotal":
				if section != "footer" {
					c.errorf(ch, "subtotal ist nur im Tabellenfuß erlaubt")
				}
				tc.sumIndex(ch.Get("of", ""))
				tc.subtotals = append(tc.subtotals, ch)
			}
			walk(ch, section)
		}
	}
	for _, sec := range n.Elements("") {
		switch sec.Name {
		case "header", "footer", "rows":
			walk(sec, sec.Name)
		case "group":
			walk(sec, "group")
		}
	}
	if h := n.First("header"); h != nil {
		for _, r := range h.Elements("row") {
			if !hasDescendant(r, "carryOver") {
				continue
			}
			for _, cell := range r.Elements("cell") {
				if !hasDescendant(cell, "carryOver") {
					tc.labels = append(tc.labels, cell)
				}
			}
		}
	}
}

func hasDescendant(n *node, name string) bool {
	for _, ch := range n.Elements("") {
		if ch.Name == name || ch.Name != "table" && hasDescendant(ch, name) {
			return true
		}
	}
	return false
}

// groupIterate groups the data nodes and continues the running sums across
// the groups.
func (c *compiler) groupIterate(g, rows *node) {
	tc := c.table
	gi := c.w.enter(g.ID())
	c.w.start("xsl:variable", "name", tc.groups, "as", "array(node())*")
	c.w.start("xsl:for-each-group", "select", "$"+tc.rows, "group-by", g.Get("by", ""))
	c.w.empty("xsl:sequence", "select", "array { current-group() }")
	c.w.end()
	c.w.end()
	c.w.start("xsl:iterate", "select", "$"+tc.groups)
	c.w.empty("xsl:param", "name", "group-"+tc.indexVar(), "select", "1", "as", "xs:integer")
	init := make([]string, len(tc.sums))
	for k := range tc.sums {
		c.w.empty("xsl:param", "name", "group-"+tc.sumVar(k+1), "select", "0", "as", "xs:decimal")
		init[k] = "$group-" + tc.sumVar(k+1)
	}
	c.w.empty("xsl:variable", "name", tc.group, "select", "?*")
	tc.inGroup = true
	if h := g.First("header"); h != nil {
		c.groupSection(h)
	}
	tc.inGroup = false
	c.rowsIterate(rows, "$"+tc.group, "$group-"+tc.indexVar(), init)
	tc.inGroup = true
	if f := g.First("footer"); f != nil {
		c.groupSection(f)
	}
	tc.inGroup = false
	c.w.start("xsl:next-iteration")
	c.w.empty("xsl:with-param", "name", "group-"+tc.indexVar(), "select", "$group-"+tc.indexVar()+" + count($"+tc.group+")")
	for k, of := range tc.sums {
		c.w.empty("xsl:with-param", "name", "group-"+tc.sumVar(k+1),
			"select", "$group-"+tc.sumVar(k+1)+" + sum($"+tc.group+" ! xs:decimal("+of+"))")
	}
	c.w.end()
	c.w.end()
	c.w.leave(gi)
}

// groupSection writes header or footer rows of a group in the context of
// its first data node.
func (c *compiler) groupSection(s *node) {
	i := c.w.enter(s.ID())
	c.w.start("xsl:for-each", "select", "$"+c.table.group+"[1]")
	for _, r := range s.Elements("row") {
		c.row(r, nil)
	}
	c.w.end()
	c.w.leave(i)
}

// rowsIterate writes the data rows with running sums and markers.
func (c *compiler) rowsIterate(rows *node, sel, index string, init []string) {
	tc := c.table
	c.w.start("xsl:iterate", "select", sel)
	c.w.empty("xsl:param", "name", tc.indexVar(), "select", index, "as", "xs:integer")
	for k := range tc.sums {
		c.w.empty("xsl:param", "name", tc.sumVar(k+1), "select", init[k], "as", "xs:decimal")
	}
	for k, of := range tc.sums {
		c.w.empty("xsl:variable", "name", tc.nextVar(k+1), "select", "$"+tc.sumVar(k+1)+" + sum(xs:decimal("+of+"))", "as", "xs:decimal")
	}
	rs := rows.Elements("row")
	for i, r := range rs {
		c.row(r, func() {
			if i == 0 {
				c.carryMarkers()
			}
			if i == len(rs)-1 {
				c.subtotalMarkers()
			}
		})
	}
	c.w.start("xsl:next-iteration")
	c.w.empty("xsl:with-param", "name", tc.indexVar(), "select", "$"+tc.indexVar()+" + 1")
	for k := range tc.sums {
		c.w.empty("xsl:with-param", "name", tc.sumVar(k+1), "select", "$"+tc.nextVar(k+1))
	}
	c.w.end()
	c.w.end()
}

// carryMarkers writes the markers for carry-over rows. The first data row
// carries empty markers, there is no carry-over on the first page.
func (c *compiler) carryMarkers() {
	tc := c.table
	first := "$" + tc.indexVar() + " gt 1"
	for _, s := range tc.carries {
		i := c.w.enter(s.ID())
		c.w.start("fo:marker", "marker-class-name", s.ID())
		c.w.start("xsl:if", "test", first)
		c.w.empty("xsl:value-of", "select", c.sumValue(s, "$"+tc.sumVar(tc.sumIndex(s.Get("of", "")))))
		c.w.end()
		c.w.end()
		c.w.leave(i)
	}
	for _, cell := range tc.labels {
		i := c.w.enter(cell.ID())
		c.w.start("fo:marker", "marker-class-name", cell.ID())
		c.w.start("xsl:if", "test", first)
		c.w.start("xsl:for-each", "select", "$"+tc.ctx)
		c.labelContent(cell)
		c.w.end()
		c.w.end()
		c.w.end()
		c.w.leave(i)
	}
}

func (c *compiler) subtotalMarkers() {
	tc := c.table
	for _, s := range tc.subtotals {
		i := c.w.enter(s.ID())
		c.w.start("fo:marker", "marker-class-name", s.ID())
		c.w.empty("xsl:value-of", "select", c.sumValue(s, "$"+tc.nextVar(tc.sumIndex(s.Get("of", "")))))
		c.w.end()
		c.w.leave(i)
	}
}

// sumValue formats a sum with the pattern of the sum element.
func (c *compiler) sumValue(s *node, arg string) string {
	if f, ok := s.Attr("format"); ok {
		if p := c.pattern(s, f); p != nil {
			if p.Get("kind", "") != "number" {
				c.errorf(s, "Formatmuster %q ist kein Zahlenformat", f)
				return arg
			}
			return c.formatCall(p, arg)
		}
	}
	return "string(" + arg + ")"
}

// row writes a table row. markers writes markers into the first cell.
func (c *compiler) row(r *node, markers func()) {
	ri := c.w.enter(r.ID())
	c.presence(r, func() {
		attrs := append([]string{"pdx:id", r.ID()}, c.blockAttrs(r)...)
		// Carry-over and subtotal are consequences of the page break, not
		// invoice data. Their rows are artifacts, the sum remains content.
		if sumRow(r) && r.Get("artifact", "") == "" {
			attrs = append(attrs, "pdx:artifact", "true")
		}
		c.w.start("fo:table-row", attrs...)
		c.altBind(r)
		for i, cell := range r.Elements("cell") {
			c.cell(cell, i == 0 && markers != nil, markers)
		}
		c.w.end()
	})
	c.w.leave(ri)
}

// sumRow reports a row with carry-over or subtotal.
func sumRow(r *node) bool {
	for _, cell := range r.Elements("cell") {
		if cell.First("carryOver") != nil || cell.First("subtotal") != nil {
			return true
		}
	}
	return false
}

var displayAlign = map[string]string{"top": "before", "middle": "center", "bottom": "after"}

func (c *compiler) cell(cell *node, first bool, markers func()) {
	i := c.w.enter(cell.ID())
	defer c.w.leave(i)
	attrs := []string{"pdx:id", cell.ID()}
	if v := cell.Get("colspan", "1"); v != "1" {
		attrs = append(attrs, "number-columns-spanned", v)
	}
	if v := cell.Get("rowspan", "1"); v != "1" {
		attrs = append(attrs, "number-rows-spanned", v)
	}
	if v := cell.Get("verticalAlign", "top"); v != "top" {
		attrs = append(attrs, "display-align", displayAlign[v])
	}
	if v := cell.Get("scope", ""); v != "" {
		attrs = append(attrs, "pdx:scope", v)
	}
	c.start("fo:table-cell", cell, append(attrs, c.style(cell)...)...)
	if first {
		markers()
	}
	if c.isLabel(cell) {
		// The block around fo:retrieve-table-marker is needed because Apache
		// FOP 2.11 aborts with a NullPointerException when the marker is a
		// direct child of the cell.
		c.w.start("fo:block", "pdx:id", cell.ID())
		c.w.empty("fo:retrieve-table-marker", "pdx:id", cell.ID(), "retrieve-class-name", cell.ID(),
			"retrieve-position-within-table", "first-starting", "retrieve-boundary-within-table", "table-fragment")
		c.w.end()
	} else {
		c.cellContent(cell)
	}
	c.w.end()
}

// labelContent writes the content of a cell in a carry-over row as inline
// content. It is inserted into a block of the header row, FOP requires
// inline content there. Therefore only text and field are allowed.
func (c *compiler) labelContent(cell *node) {
	for _, k := range cell.Elements("") {
		i := c.w.enter(k.ID())
		c.presence(k, func() {
			switch k.Name {
			case "text":
				c.w.start("fo:inline", append([]string{"pdx:id", k.ID()}, c.style(k)...)...)
				c.textContent(k)
				c.w.end()
			case "field":
				c.w.start("fo:inline", append([]string{"pdx:id", k.ID()}, c.style(k)...)...)
				c.w.empty("xsl:value-of", "select", c.fieldExpr(k))
				c.w.end()
			default:
				c.errorf(k, "in einer Übertragszeile sind neben carryOver nur text und field erlaubt")
			}
		})
		c.w.leave(i)
	}
}

func (c *compiler) isLabel(cell *node) bool {
	for _, l := range c.table.labels {
		if l == cell {
			return true
		}
	}
	return false
}

// cellContent writes the content of a cell. An empty cell gets an empty
// block, FO requires at least one.
func (c *compiler) cellContent(cell *node) {
	kids := cell.Elements("")
	if len(kids) == 0 {
		c.w.empty("fo:block", "pdx:id", cell.ID())
		return
	}
	c.presence(cell, func() {
		for _, k := range kids {
			switch k.Name {
			case "carryOver", "subtotal", "total":
				c.sumElem(k)
			default:
				c.content(k, false)
			}
		}
	})
}

func (c *compiler) sumElem(s *node) {
	tc := c.table
	i := c.w.enter(s.ID())
	defer c.w.leave(i)
	c.w.start("fo:block", append([]string{"pdx:id", s.ID()}, c.style(s)...)...)
	switch s.Name {
	case "carryOver":
		c.w.empty("fo:retrieve-table-marker", "pdx:id", s.ID(), "retrieve-class-name", s.ID(),
			"retrieve-position-within-table", "first-starting", "retrieve-boundary-within-table", "table-fragment")
	case "subtotal":
		c.w.empty("fo:retrieve-table-marker", "pdx:id", s.ID(), "retrieve-class-name", s.ID(),
			"retrieve-position-within-table", "last-ending", "retrieve-boundary-within-table", "table-fragment")
	case "total":
		src := tc.rows
		if tc.inGroup {
			src = tc.group
		}
		c.w.empty("xsl:value-of", "select", c.sumValue(s, "sum($"+src+" ! xs:decimal("+s.Get("of", "")+"))"))
	}
	c.w.end()
}
