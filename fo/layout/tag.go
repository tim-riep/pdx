package layout

import (
	"strconv"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/fo/tree"
)

// pdxAttr is the key of an attribute in the pdx namespace.
func pdxAttr(name string) string { return "{" + tree.NSPDX + "}" + name }

// tagOf reads role and details for Tagged PDF of an FO node: role,
// xml:lang, pdx:alt, pdx:scope, pdx:reading-order and pdx:artifact. Content
// of a retrieved marker is a repetition.
func tagOf(n *tree.Node) (string, *areatree.Tag) {
	if n == nil {
		return "", nil
	}
	t := areatree.Tag{
		Alt:      n.Attrs[pdxAttr("alt")],
		Lang:     n.Attrs["xml:lang"],
		Scope:    n.Attrs[pdxAttr("scope")],
		Artifact: n.Attrs[pdxAttr("artifact")] == "true",
		Content:  n.Attrs[pdxAttr("artifact")] == "false",
		Repeat:   inMarker(n),
		Static:   inStatic(n),
	}
	if v := n.Attrs[pdxAttr("reading-order")]; v != "" {
		t.Order, _ = strconv.Atoi(v)
	}
	role := n.Attrs["role"]
	if t == (areatree.Tag{}) {
		return role, nil
	}
	return role, &t
}

// setTag transfers role and details of a node to its container.
func setTag(c *areatree.Container, n *tree.Node) { c.Role, c.Tag = tagOf(n) }

// inStatic reports a node in fo:static-content.
func inStatic(n *tree.Node) bool {
	for m := n.Parent; m != nil; m = m.Parent {
		if m.IsFO("static-content") {
			return true
		}
	}
	return false
}

// absParent returns the source of the nearest absolutely positioned
// fo:block-container above a node, else empty.
func absParent(n *tree.Node) string {
	for m := n.Parent; m != nil; m = m.Parent {
		if m.IsFO("block-container") {
			if v, _ := m.Attr("absolute-position"); v == "absolute" || v == "fixed" {
				return m.Label()
			}
		}
	}
	return ""
}

// ensureTag returns the tag of a container and creates it if needed.
func ensureTag(c *areatree.Container) *areatree.Tag {
	if c.Tag == nil {
		c.Tag = &areatree.Tag{}
	}
	return c.Tag
}

// inMarker reports a node in the content of an fo:marker.
func inMarker(n *tree.Node) bool {
	for m := n.Parent; m != nil; m = m.Parent {
		if m.IsFO("marker") {
			return true
		}
	}
	return false
}

// rowRef connects drawing a row with its identity: fragments
// of the same row on several pages and re-laid-out rows with markers
// carry the same number. repeat is set for repeated headers and
// footers.
type rowRef struct {
	key    *rowLayout
	repeat bool
}

// rowNumber assigns row numbers in the order of placement.
func (e *engine) rowNumber(r *rowLayout) int {
	if e.rowNums == nil {
		e.rowNums = map[*rowLayout]int{}
	}
	n, ok := e.rowNums[r]
	if !ok {
		n = len(e.rowNums) + 1
		e.rowNums[r] = n
	}
	return n
}

var sectionNames = map[section]string{secHeader: "header", secBody: "body", secFooter: "footer"}

// cellTag adds row, section and repetition to the container of a cell.
// An artifact on the row applies to its cells.
func (pl *placer) cellTag(c *areatree.Container, cell *tree.Node, r *rowLayout, ref rowRef) {
	setTag(c, cell)
	ensureTag(c)
	c.Tag.Row = pl.e.rowNumber(ref.key)
	c.Tag.Section = "item"
	if r.tbl != nil {
		c.Tag.Section = sectionNames[r.sec]
	}
	c.Tag.Repeat = c.Tag.Repeat || ref.repeat
	if n := spanAttr(cell, "number-columns-spanned"); n > 1 {
		c.Tag.ColSpan = n
	}
	if n := spanAttr(cell, "number-rows-spanned"); n > 1 {
		c.Tag.RowSpan = n
	}
	if r.node != nil {
		if _, rt := tagOf(r.node); rt != nil && rt.Artifact {
			c.Tag.Artifact = true
		}
	}
}

// noteInline remembers role and details of an inline element with a
// template ID for the structure tree.
func (e *engine) noteInline(id string, n *tree.Node) {
	role, t := tagOf(n)
	if role == "" && t == nil && !n.IsFO("basic-link") {
		return
	}
	if role == "" && n.IsFO("basic-link") {
		role = "Link"
	}
	if e.doc.Inline == nil {
		e.doc.Inline = map[string]areatree.InlineTag{}
	}
	it := areatree.InlineTag{Role: role}
	if t != nil {
		it.Tag = *t
	}
	e.doc.Inline[id] = it
}

// started reports whether a fragment of a block has already been placed, and
// records the current one. Later fragments continue the block.
func (e *engine) started(ctx *blockCtx) bool {
	if e.begun == nil {
		e.begun = map[*blockCtx]bool{}
	}
	if e.begun[ctx] {
		return true
	}
	e.begun[ctx] = true
	return false
}
