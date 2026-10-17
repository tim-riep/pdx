package layout

import (
	"strconv"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/fo/props"
	"github.com/tim-riep/pdx/fo/tree"
)

// sequence ist der Zustand einer fo:page-sequence.
type sequence struct {
	e       *engine
	node    *tree.Node
	sel     *selector
	props   *props.Computed
	statics map[string]*tree.Node
	flow    *tree.Node
	page    *areatree.Page
	m       *master
	count   int // Seiten dieser Folge
}

func (e *engine) pageSequence(ps *tree.Node, rootProps *props.Computed) error {
	ref, _ := ps.Attr("master-reference")
	sel, err := e.newSelector(ps, ref)
	if err != nil {
		return err
	}
	switch v, _ := ps.Attr("initial-page-number"); v {
	case "", "auto":
	case "auto-odd":
		if (e.pageNo+1)%2 == 0 {
			e.pageNo++
		}
	case "auto-even":
		if (e.pageNo+1)%2 != 0 {
			e.pageNo++
		}
	default:
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return tree.Errorf(ps, "initial-page-number %q wird nicht unterstützt", v)
		}
		e.pageNo = n - 1
	}
	e.format = attrOr(ps, "format", "1")
	e.curSeq = ps
	p, err := props.Compute(rootProps, ps, 0)
	if err != nil {
		return err
	}
	s := &sequence{e: e, node: ps, sel: sel, props: p, statics: map[string]*tree.Node{}}
	for _, c := range ps.Children {
		switch {
		case c.IsFO("static-content"):
			name, _ := c.Attr("flow-name")
			if _, dup := s.statics[name]; dup {
				return tree.Errorf(c, "flow-name %q ist doppelt", name)
			}
			s.statics[name] = c
		case c.IsFO("flow"):
			if s.flow != nil {
				return tree.Errorf(c, "mehr als ein fo:flow")
			}
			s.flow = c
		}
	}
	if s.flow == nil {
		return tree.Errorf(ps, "fo:flow fehlt")
	}
	if err := s.paginate(); err != nil {
		return err
	}
	return s.forcePageCount()
}

// galleyFor erzeugt die Galley für eine Body-Breite. skip lässt alle Boxen
// vor dieser Stelle aus, damit nach einem Wechsel der Breite ab dem Umbruch
// neu gesetzt werden kann.
func (s *sequence) galleyFor(m *master, skip ordinal) ([]item, error) {
	if name, _ := s.flow.Attr("flow-name"); name != m.bodyName {
		return nil, tree.Errorf(s.flow, "flow-name %q passt nicht zur Region %q", name, m.bodyName)
	}
	fp, err := props.Compute(s.props, s.flow, m.body.W)
	if err != nil {
		return nil, err
	}
	g := newGalley(s.e, m.body.W, 0)
	g.skip = skip
	if err := g.blocks(s.flow, fp); err != nil {
		return nil, err
	}
	return g.items, nil
}

// nextOrd liefert die Stelle der ersten Box ab pos.
func nextOrd(items []item, pos int) (ordinal, bool) {
	for i := pos; i < len(items); i++ {
		if items[i].kind == itBox {
			return items[i].ord, true
		}
	}
	return ordinal{}, false
}

// paginate verteilt den Fluss auf Seiten. Je Seite wird die Vorlage gewählt.
// Passt der ganze Rest auf eine Seite mit der Vorlage für die letzte Seite,
// wird sie genommen (page-position="last" und "only").
func (s *sequence) paginate() error {
	var items []item
	var width areatree.Length = -1
	pos, empty := 0, 0
	for {
		cond := pageCond{number: s.e.pageNo + 1, first: s.count == 0}
		m, err := s.sel.pick(cond, false)
		if err != nil {
			return err
		}
		// In eine Region ohne Fläche passt kein Inhalt. Die Seite bleibt
		// leer, der Inhalt läuft auf der nächsten weiter. Nach einigen leeren
		// Seiten in Folge wird gesetzt, damit die Folge endet.
		if (m.body.H <= 0 || m.body.W <= 0) && empty < maxEmptyPages {
			if _, err := s.sel.pick(cond, true); err != nil {
				return err
			}
			s.newPage(m)
			if err := s.finishPage(); err != nil {
				return err
			}
			empty++
			continue
		}
		if items, pos, err = s.adapt(m, items, pos, &width); err != nil {
			return err
		}
		// break-before="odd-page" oder "even-page" am ersten Inhalt der
		// Folge: notfalls eine Leerseite davor.
		if s.count == 0 && pos < len(items) && items[pos].kind == itPenalty {
			if brk := items[pos].brk; brk == "odd-page" || brk == "even-page" {
				if next := s.e.pageNo + 1; (brk == "odd-page") != (next%2 == 1) {
					if err := s.blankPage(false); err != nil {
						return err
					}
					continue
				}
			}
		}
		condLast := cond
		condLast.last = true
		mLast, err := s.sel.pick(condLast, false)
		if err != nil {
			return err
		}
		last := false
		// Folgt wegen force-page-count noch eine Leerseite, ist diese Seite
		// nicht die letzte der Folge.
		blankFollows, err := s.needsBlank(s.count+1, cond.number)
		if err != nil {
			return err
		}
		if blankFollows {
			mLast = nil
		}
		if mLast == nil {
		} else if mLast == m {
			last = breakPage(items, pos, m.body.H) >= len(items)
		} else {
			w := width
			li, lp, err := s.adapt(mLast, items, pos, &w)
			if err != nil {
				return err
			}
			if breakPage(li, lp, mLast.body.H) >= len(li) {
				m, items, pos, width, last = mLast, li, lp, w, true
			}
		}
		cond.last = last
		if _, err := s.sel.pick(cond, true); err != nil {
			return err
		}
		end := breakPage(items, pos, m.body.H)
		// Läuft die Seite über, weil ein Keep nicht gebrochen werden darf, und
		// hat die nächste Seite mehr Platz, bleibt diese Seite leer. So erfüllt
		// der Keep ohne Überlauf (XSL 1.1 4.8 lässt beides zu).
		if empty < maxEmptyPages && overflows(items, pos, end, m.body.H) && !last {
			next := pageCond{number: s.e.pageNo + 2}
			if nm, err := s.sel.pick(next, false); err == nil && nm != nil && nm.body.H > m.body.H {
				s.newPage(m)
				if err := s.finishPage(); err != nil {
					return err
				}
				empty++
				continue
			}
		}
		region := s.newPage(m)
		if err := newPlacer(s.e, s.page, m.body, region).place(items[pos:end]); err != nil {
			return err
		}
		empty = 0
		if err := s.finishPage(); err != nil {
			return err
		}
		if end >= len(items) {
			return nil
		}
		// odd-page und even-page erzwingen notfalls eine Leerseite.
		if brk := items[end].brk; brk == "odd-page" || brk == "even-page" {
			if next := s.e.pageNo + 1; (brk == "odd-page") != (next%2 == 1) {
				if err := s.blankPage(false); err != nil {
					return err
				}
			}
		}
		pos = end + 1
	}
}

// overflows meldet, ob der Inhalt items[pos:end] höher ist als avail.
func overflows(items []item, pos, end int, avail areatree.Length) bool {
	var h areatree.Length
	seen := false
	for _, it := range items[pos:min(end, len(items))] {
		switch it.kind {
		case itBox:
			h += it.h
			seen = true
		case itGlue:
			if seen {
				h += it.h
			} else {
				h += it.keep
			}
		}
	}
	return h > avail
}

// maxEmptyPages begrenzt leere Seiten in Folge bei Regionen ohne Fläche.
const maxEmptyPages = 8

// adapt liefert die Galley für die Breite der Vorlage. Ändert sich die
// Breite, wird ab der aktuellen Stelle neu erzeugt.
func (s *sequence) adapt(m *master, items []item, pos int, width *areatree.Length) ([]item, int, error) {
	if m.body.W == *width {
		return items, pos, nil
	}
	skip := ordinal{}
	if items != nil {
		o, ok := nextOrd(items, pos)
		if !ok {
			*width = m.body.W
			return nil, 0, nil
		}
		skip = o
	}
	ni, err := s.galleyFor(m, skip)
	if err != nil {
		return nil, 0, err
	}
	*width = m.body.W
	return ni, 0, nil
}

// newPage beginnt eine neue Seite und liefert den Container der Region body.
func (s *sequence) newPage(m *master) *areatree.Container {
	s.e.pageNo++
	s.count++
	s.m = m
	s.page = &areatree.Page{Number: s.e.pageNo, Width: m.width, Height: m.height}
	s.e.doc.Pages = append(s.e.doc.Pages, s.page)
	// Die Seiten gehören zur Seitenfolge und zu ihren Vorfahren bis fo:root,
	// die nach XSL 1.1 6.4.2 ebenfalls eine id tragen dürfen.
	for n := s.node; n != nil; n = n.Parent {
		if id, ok := n.Attr("id"); ok && id != "" {
			s.e.recordID(id, formatNumber(s.page.Number, s.e.format))
			s.e.recordDest(id, s.page, areatree.Point{})
		}
	}
	region := &areatree.Container{Kind: areatree.KindRegion, Rect: m.body, Source: m.bodyNode.Label()}
	s.page.Areas = append(s.page.Areas, region)
	return region
}

// finishPage setzt die statischen Inhalte der aktuellen Seite.
func (s *sequence) finishPage() error {
	for _, r := range s.m.regions {
		sc, ok := s.statics[r.name]
		if !ok {
			continue
		}
		p, err := props.Compute(s.props, sc, r.rect.W)
		if err != nil {
			return err
		}
		cont := &areatree.Container{Kind: areatree.KindRegion, Rect: r.rect, Source: r.node.Label()}
		s.page.Areas = append(s.page.Areas, cont)
		g := newGalley(s.e, r.rect.W, s.page.Number)
		g.page = s.page
		if err := g.blocks(sc, p); err != nil {
			return err
		}
		if err := newPlacer(s.e, s.page, r.rect, cont).place(g.items); err != nil {
			return err
		}
	}
	return nil
}

// blankPage fügt eine Leerseite an, etwa für force-page-count.
func (s *sequence) blankPage(last bool) error {
	cond := pageCond{number: s.e.pageNo + 1, first: s.count == 0, last: last, blank: true}
	m, err := s.sel.pick(cond, true)
	if err != nil {
		return err
	}
	s.newPage(m)
	return s.finishPage()
}

// forcePageCount ergänzt nach XSL 1.1 7.27.6 eine Leerseite, falls die
// Bedingung nicht erfüllt ist. Bei "auto" entscheidet eine feste
// initial-page-number der nächsten Seitenfolge.
func (s *sequence) forcePageCount() error {
	need, err := s.needsBlank(s.count, s.e.pageNo)
	if err != nil {
		return err
	}
	if need {
		return s.blankPage(true)
	}
	return nil
}

// needsBlank meldet, ob nach count Seiten mit der letzten Seitenzahl last
// eine Leerseite folgen muss. Die Leerseite ist dann die letzte Seite der
// Folge, nicht die Seite davor.
func (s *sequence) needsBlank(count, last int) (bool, error) {
	switch v := attrOr(s.node, "force-page-count", "auto"); v {
	case "even":
		return count%2 != 0, nil
	case "odd":
		return count%2 == 0, nil
	case "end-on-even":
		return last%2 != 0, nil
	case "end-on-odd":
		return last%2 == 0, nil
	case "auto":
		if n := s.nextInitial(); n > 0 {
			// Die letzte Seite soll ungerade sein, wenn die nächste Folge
			// gerade beginnt, und umgekehrt.
			return (last%2 == 0) == (n%2 == 0), nil
		}
		return false, nil
	case "no-force":
		return false, nil
	default:
		return false, tree.Errorf(s.node, "force-page-count %q ist ungültig", v)
	}
}

// nextInitial liefert eine feste initial-page-number der nächsten
// fo:page-sequence oder 0.
func (s *sequence) nextInitial() int {
	sib := s.node.Parent.Children
	for i, c := range sib {
		if c != s.node {
			continue
		}
		for _, n := range sib[i+1:] {
			if n.IsFO("page-sequence") {
				v, _ := strconv.Atoi(attrOr(n, "initial-page-number", ""))
				return v
			}
		}
	}
	return 0
}
