package layout

import (
	"strconv"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/fo/tree"
)

// pdxAttr ist der Schlüssel eines Attributs im pdx-Namespace.
func pdxAttr(name string) string { return "{" + tree.NSPDX + "}" + name }

// tagOf liest Rolle und Angaben für Tagged PDF eines FO-Knotens: role,
// xml:lang, pdx:alt, pdx:scope, pdx:reading-order und pdx:artifact. Inhalt
// eines abgerufenen Markers ist eine Wiederholung.
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

// setTag überträgt Rolle und Angaben eines Knotens auf seinen Container.
func setTag(c *areatree.Container, n *tree.Node) { c.Role, c.Tag = tagOf(n) }

// inStatic meldet einen Knoten in fo:static-content.
func inStatic(n *tree.Node) bool {
	for m := n.Parent; m != nil; m = m.Parent {
		if m.IsFO("static-content") {
			return true
		}
	}
	return false
}

// absParent liefert die Quelle des nächsten absolut positionierten
// fo:block-container über einem Knoten, sonst leer.
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

// ensureTag liefert das Tag eines Containers und legt es bei Bedarf an.
func ensureTag(c *areatree.Container) *areatree.Tag {
	if c.Tag == nil {
		c.Tag = &areatree.Tag{}
	}
	return c.Tag
}

// inMarker meldet einen Knoten im Inhalt eines fo:marker.
func inMarker(n *tree.Node) bool {
	for m := n.Parent; m != nil; m = m.Parent {
		if m.IsFO("marker") {
			return true
		}
	}
	return false
}

// rowRef verbindet das Zeichnen einer Zeile mit ihrer Identität: Fragmente
// derselben Zeile auf mehreren Seiten und neu gesetzte Zeilen mit Markern
// tragen dieselbe Nummer. repeat ist bei wiederholten Köpfen und Füßen
// gesetzt.
type rowRef struct {
	key    *rowLayout
	repeat bool
}

// rowNumber vergibt Zeilennummern in der Reihenfolge des Platzierens.
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

// cellTag ergänzt den Container einer Zelle um Zeile, Abschnitt und
// Wiederholung. Ein Artefakt auf der Zeile gilt für ihre Zellen.
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

// noteInline merkt sich Rolle und Angaben eines Inline-Elements mit
// Vorlagen-ID für den Strukturbaum.
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

// started meldet, ob von einem Block schon ein Fragment gesetzt ist, und
// vermerkt das aktuelle. Spätere Fragmente setzen den Block fort.
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
