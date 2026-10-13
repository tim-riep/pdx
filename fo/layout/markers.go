package layout

import (
	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/fo/tree"
)

// Marker (XSL 1.1 6.13.5 bis 6.13.7). Der Placer protokolliert je Seite die
// Bereiche mit Markern in Vorordnung: ob sie auf der Seite beginnen (is-first)
// und ob sie dort enden (is-last). fo:retrieve-marker in statischen Inhalten
// und fo:retrieve-table-marker in Tabellenkopf und -fuß wählen daraus.

// markerSet sind die fo:marker eines Elements nach Klasse.
type markerSet map[string]*tree.Node

// collectMarkers liest die fo:marker-Kinder eines Elements und, bei Zeilen,
// die ihrer Zellen.
func collectMarkers(n *tree.Node) markerSet {
	var m markerSet
	add := func(x *tree.Node) {
		for _, c := range x.Children {
			if c.IsFO("marker") {
				if m == nil {
					m = markerSet{}
				}
				if cls, _ := c.Attr("marker-class-name"); cls != "" {
					if _, dup := m[cls]; !dup {
						m[cls] = c
					}
				}
			}
		}
	}
	add(n)
	if n.IsFO("table-row") {
		for _, c := range n.Children {
			if c.IsFO("table-cell") {
				add(c)
			}
		}
	}
	return m
}

// mkEvent ist ein Bereich mit Markern auf einer Seite.
type mkEvent struct {
	markers markerSet
	start   bool // der Bereich beginnt auf dieser Seite
	end     bool // der Bereich endet auf dieser Seite
}

// pageMarks sind die Marker-Ereignisse einer Seite.
type pageMarks struct {
	page   *areatree.Page
	seq    *tree.Node
	events []*mkEvent
}

// marksFor liefert die Ereignisliste einer Seite.
func (e *engine) marksFor(p *areatree.Page) *pageMarks {
	if pm, ok := e.marks[p]; ok {
		return pm
	}
	pm := &pageMarks{page: p, seq: e.curSeq}
	e.marks[p] = pm
	e.markOrder = append(e.markOrder, pm)
	return pm
}

// note vermerkt einen Bereich mit Markern. key identifiziert den Bereich auf
// der Seite, damit Fortsetzungen nicht doppelt zählen.
func (pl *placer) note(key any, m markerSet, start, end bool) (*mkEvent, bool) {
	if m == nil {
		return nil, false
	}
	if pl.seen == nil {
		pl.seen = map[any]*mkEvent{}
	}
	if ev, ok := pl.seen[key]; ok {
		ev.end = ev.end || end
		return ev, false
	}
	ev := &mkEvent{markers: m, start: start, end: end}
	pl.seen[key] = ev
	pm := pl.e.marksFor(pl.page)
	pm.events = append(pm.events, ev)
	return ev, true
}

// pick wählt aus Ereignissen nach Position. first: vorne beginnen, starting:
// nur beginnende, ending: nur endende.
func pick(events []*mkEvent, cls, pos string) *tree.Node {
	match := func(ev *mkEvent) *tree.Node {
		m := ev.markers[cls]
		if m == nil {
			return nil
		}
		switch pos {
		case "first-starting-within-page", "first-starting", "last-starting-within-page", "last-starting":
			if !ev.start {
				return nil
			}
		case "last-ending-within-page", "last-ending":
			if !ev.end {
				return nil
			}
		}
		return m
	}
	switch pos {
	case "first-starting-within-page", "first-starting", "first-including-carryover":
		for _, ev := range events {
			if m := match(ev); m != nil {
				return m
			}
		}
	default:
		for i := len(events) - 1; i >= 0; i-- {
			if m := match(events[i]); m != nil {
				return m
			}
		}
	}
	return nil
}

// lastAny liefert den letzten Marker einer Klasse in den Ereignissen.
func lastAny(events []*mkEvent, cls string) *tree.Node {
	for i := len(events) - 1; i >= 0; i-- {
		if m := events[i].markers[cls]; m != nil {
			return m
		}
	}
	return nil
}

// retrieveMarker löst ein fo:retrieve-marker für die aktuelle Seite auf.
// Findet sich auf der Seite nichts, gilt der letzte Marker der Klasse auf den
// vorigen Seiten innerhalb von retrieve-boundary.
func (e *engine) retrieveMarker(n *tree.Node, page *areatree.Page) (*tree.Node, error) {
	cls, _ := n.Attr("retrieve-class-name")
	if cls == "" {
		return nil, tree.Errorf(n, "retrieve-class-name fehlt")
	}
	pos := attrOr(n, "retrieve-position", "first-starting-within-page")
	switch pos {
	case "first-starting-within-page", "first-including-carryover", "last-starting-within-page", "last-ending-within-page":
	default:
		return nil, tree.Errorf(n, "retrieve-position %q ist ungültig", pos)
	}
	boundary := attrOr(n, "retrieve-boundary", "page-sequence")
	cur := e.marksFor(page)
	if m := pick(cur.events, cls, pos); m != nil {
		return m, nil
	}
	if boundary == "page" {
		return nil, nil
	}
	for i := len(e.markOrder) - 1; i >= 0; i-- {
		pm := e.markOrder[i]
		if pm == cur {
			continue
		}
		if boundary == "page-sequence" && pm.seq != cur.seq {
			break
		}
		if m := lastAny(pm.events, cls); m != nil {
			return m, nil
		}
	}
	return nil, nil
}

// tableFrag sind die Marker-Ereignisse eines Tabellenfragments (einer Seite).
type tableFrag struct {
	page   *areatree.Page
	events []*mkEvent
}

// tableMarks liefert das Fragment der Tabelle auf einer Seite.
func (t *tableCtx) fragFor(p *areatree.Page) *tableFrag {
	if n := len(t.frags); n > 0 && t.frags[n-1].page == p {
		return t.frags[n-1]
	}
	f := &tableFrag{page: p}
	t.frags = append(t.frags, f)
	return f
}

// retrieveTableMarker löst ein fo:retrieve-table-marker im Fragment der
// aktuellen Seite auf.
func (t *tableCtx) retrieveTableMarker(n *tree.Node, page *areatree.Page) (*tree.Node, error) {
	cls, _ := n.Attr("retrieve-class-name")
	if cls == "" {
		return nil, tree.Errorf(n, "retrieve-class-name fehlt")
	}
	pos := attrOr(n, "retrieve-position-within-table", "first-starting")
	switch pos {
	case "first-starting", "first-including-carryover", "last-starting", "last-ending":
	default:
		return nil, tree.Errorf(n, "retrieve-position-within-table %q ist ungültig", pos)
	}
	boundary := attrOr(n, "retrieve-boundary-within-table", "table")
	cur := t.fragFor(page)
	if m := pick(cur.events, cls, pos); m != nil {
		return m, nil
	}
	if boundary == "table-fragment" || boundary == "page" {
		return nil, nil
	}
	for i := len(t.frags) - 1; i >= 0; i-- {
		if t.frags[i] == cur {
			continue
		}
		if m := lastAny(t.frags[i].events, cls); m != nil {
			return m, nil
		}
	}
	return nil, nil
}

// hasTableMarker meldet, ob ein Teilbaum ein fo:retrieve-table-marker enthält.
func hasTableMarker(n *tree.Node) bool {
	if n.IsFO("retrieve-table-marker") {
		return true
	}
	for _, c := range n.Children {
		if hasTableMarker(c) {
			return true
		}
	}
	return false
}

// sample liefert den ersten fo:marker einer Klasse in der Tabelle.
func (t *tableCtx) sample(cls string) *tree.Node {
	var found *tree.Node
	var walk func(n *tree.Node)
	walk = func(n *tree.Node) {
		if found != nil {
			return
		}
		if n.IsFO("marker") {
			if c, _ := n.Attr("marker-class-name"); c == cls {
				found = n
			}
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(t.node)
	return found
}
