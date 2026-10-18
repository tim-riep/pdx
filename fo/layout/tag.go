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
		Repeat:   inMarker(n),
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
	if c.Tag == nil {
		c.Tag = &areatree.Tag{}
	}
	c.Tag.Row = pl.e.rowNumber(ref.key)
	c.Tag.Section = "item"
	if r.tbl != nil {
		c.Tag.Section = sectionNames[r.sec]
	}
	c.Tag.Repeat = c.Tag.Repeat || ref.repeat
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
