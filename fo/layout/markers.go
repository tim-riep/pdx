package layout

import (
	"strings"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/fo/tree"
)

// Markers (XSL 1.1 6.13.5 to 6.13.7). The placer records per page the
// areas with markers in pre-order: whether they start on the page (is-first)
// and whether they end there (is-last). fo:retrieve-marker in static content
// and fo:retrieve-table-marker in table header and footer choose from them.

// markerSet holds the fo:marker elements of an element by class.
type markerSet map[string]*tree.Node

// collectMarkers reads the fo:marker children of an element and, for rows,
// those of their cells.
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

// mkEvent is an area with markers on a page.
type mkEvent struct {
	markers markerSet
	start   bool // the area starts on this page
	end     bool // the area ends on this page
}

// pageMarks are the marker events of a page.
type pageMarks struct {
	page   *areatree.Page
	seq    *tree.Node
	events []*mkEvent
}

// marksFor returns the event list of a page.
func (e *engine) marksFor(p *areatree.Page) *pageMarks {
	if pm, ok := e.marks[p]; ok {
		return pm
	}
	pm := &pageMarks{page: p, seq: e.curSeq}
	e.marks[p] = pm
	e.markOrder = append(e.markOrder, pm)
	return pm
}

// note records an area with markers. key identifies the area on
// the page so that continuations are not counted twice.
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

// pick chooses from events by position. first: start at the front, starting:
// only starting ones, ending: only ending ones.
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

// lastAny returns the last marker of a class in the events.
func lastAny(events []*mkEvent, cls string) *tree.Node {
	for i := len(events) - 1; i >= 0; i-- {
		if m := events[i].markers[cls]; m != nil {
			return m
		}
	}
	return nil
}

// retrieveMarker resolves an fo:retrieve-marker for the current page.
// If nothing is found on the page, the last marker of the class on the
// previous pages within retrieve-boundary applies.
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

// tableFrag holds the marker events of a table fragment (one page).
type tableFrag struct {
	page   *areatree.Page
	events []*mkEvent
}

// tableMarks returns the fragment of the table on a page.
func (t *tableCtx) fragFor(p *areatree.Page) *tableFrag {
	if n := len(t.frags); n > 0 && t.frags[n-1].page == p {
		return t.frags[n-1]
	}
	f := &tableFrag{page: p}
	t.frags = append(t.frags, f)
	return f
}

// retrieveTableMarker resolves an fo:retrieve-table-marker in the fragment of
// the current page.
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

// hasTableMarker reports whether a subtree contains an fo:retrieve-table-marker.
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

// sample returns the first fo:marker of a class in the table, when
// measuring repeated headers the first non-empty one.
func (t *tableCtx) sample(cls string) *tree.Node {
	var found *tree.Node
	var walk func(n *tree.Node)
	walk = func(n *tree.Node) {
		if found != nil {
			return
		}
		if n.IsFO("marker") {
			if c, _ := n.Attr("marker-class-name"); c == cls && (!t.representative || !emptyMarker(n)) {
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

// emptyMarker reports an fo:marker without elements and without visible text.
func emptyMarker(n *tree.Node) bool {
	for _, c := range n.Children {
		if c.Kind == tree.Element || strings.TrimSpace(c.Data) != "" {
			return false
		}
	}
	return true
}
