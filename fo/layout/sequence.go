package layout

import (
	"strconv"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/fo/props"
	"github.com/tim-riep/pdx/fo/tree"
)

// sequence is the state of an fo:page-sequence.
type sequence struct {
	e       *engine
	node    *tree.Node
	sel     *selector
	props   *props.Computed
	statics map[string]*tree.Node
	flow    *tree.Node
	page    *areatree.Page
	m       *master
	count   int // pages of this sequence
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

// galleyFor generates the galley for a body width. skip omits all boxes
// before this position so that after a width change the content can be
// laid out again from the break.
func (s *sequence) galleyFor(m *master, skip ordinal, resume []resumePoint) ([]item, error) {
	if name, _ := s.flow.Attr("flow-name"); name != m.bodyName {
		return nil, tree.Errorf(s.flow, "flow-name %q passt nicht zur Region %q", name, m.bodyName)
	}
	fp, err := props.Compute(s.props, s.flow, m.body.W)
	if err != nil {
		return nil, err
	}
	g := newGalley(s.e, m.body.W, 0)
	g.skip, g.resume = skip, resume
	if err := g.blocks(s.flow, fp); err != nil {
		return nil, err
	}
	return g.items, nil
}

// nextBox returns the first box from pos.
func nextBox(items []item, pos int) (*item, bool) {
	for i := pos; i < len(items); i++ {
		if items[i].kind == itBox {
			return &items[i], true
		}
	}
	return nil, false
}

// paginate distributes the flow over pages. The master is chosen per page.
// If the whole rest fits on one page with the master for the last page,
// that one is taken (page-position="last" and "only").
func (s *sequence) paginate() error {
	var items []item
	var width areatree.Length = -1
	pos, empty := 0, 0
	for {
		if err := s.e.opts.Context.Err(); err != nil {
			return err
		}
		cond := pageCond{number: s.e.pageNo + 1, first: s.count == 0}
		m, err := s.sel.pick(cond, false)
		if err != nil {
			return err
		}
		// No content fits into a region without area, nor into a
		// page without a region with the name of the flow (XSL 1.1 7.27.5). The
		// page stays empty, the content continues on the next one. After
		// some empty pages in a row the content is placed so that the sequence ends.
		name, _ := s.flow.Attr("flow-name")
		unmapped := name != m.bodyName
		if (m.body.H <= 0 || m.body.W <= 0 || unmapped) && empty < maxEmptyPages {
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
		if unmapped {
			return tree.Errorf(s.flow, "flow-name %q passt nicht zur Region %q", name, m.bodyName)
		}
		if items, pos, err = s.adapt(m, items, pos, &width); err != nil {
			return err
		}
		// break-before="odd-page" or "even-page" on the first content of the
		// sequence: if needed a blank page before it.
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
		// If another blank page follows because of force-page-count, this page is
		// not the last of the sequence.
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
		// If the page overflows because a keep must not be broken, and
		// the next page has more space, this page stays empty. This way the keep
		// is satisfied without overflow (XSL 1.1 4.8 allows both).
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
		// odd-page and even-page force a blank page if needed.
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

// overflows reports whether the content items[pos:end] is taller than avail.
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

// maxEmptyPages limits empty pages in a row for regions without area.
const maxEmptyPages = 8

// adapt returns the galley for the width of the master. If the
// width changes, it is regenerated from the current position.
func (s *sequence) adapt(m *master, items []item, pos int, width *areatree.Length) ([]item, int, error) {
	if m.body.W == *width {
		return items, pos, nil
	}
	skip := ordinal{}
	var resume []resumePoint
	if items != nil {
		b, ok := nextBox(items, pos)
		if !ok {
			*width = m.body.W
			return nil, 0, nil
		}
		skip, resume = b.ord, b.resume
	}
	ni, err := s.galleyFor(m, skip, resume)
	if err != nil {
		return nil, 0, err
	}
	*width = m.body.W
	return ni, 0, nil
}

// newPage starts a new page and returns the container of the region body.
func (s *sequence) newPage(m *master) *areatree.Container {
	s.e.pageNo++
	s.count++
	s.m = m
	s.page = &areatree.Page{Number: s.e.pageNo, Width: m.width, Height: m.height}
	s.e.doc.Pages = append(s.e.doc.Pages, s.page)
	// The pages belong to the page sequence and to its ancestors up to fo:root,
	// which per XSL 1.1 6.4.2 may also carry an id.
	for n := s.node; n != nil; n = n.Parent {
		if id, ok := n.Attr("id"); ok && id != "" {
			s.e.recordID(id, formatNumber(s.page.Number, s.e.format))
			s.e.recordDest(id, s.page, areatree.Point{}, "")
		}
	}
	region := &areatree.Container{Kind: areatree.KindRegion, Rect: m.body, Source: m.bodyNode.Label(), ID: m.bodyNode.TemplateID()}
	s.page.Areas = append(s.page.Areas, region)
	return region
}

// finishPage lays out the static content of the current page.
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
		cont := &areatree.Container{Kind: areatree.KindRegion, Rect: r.rect, Source: r.node.Label(), ID: r.node.TemplateID()}
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

// blankPage appends a blank page, for example for force-page-count.
func (s *sequence) blankPage(last bool) error {
	cond := pageCond{number: s.e.pageNo + 1, first: s.count == 0, last: last, blank: true}
	m, err := s.sel.pick(cond, true)
	if err != nil {
		return err
	}
	s.newPage(m)
	return s.finishPage()
}

// forcePageCount adds a blank page per XSL 1.1 7.27.6 if the
// condition is not met. For "auto" a fixed initial-page-number of the
// next page sequence decides.
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

// needsBlank reports whether after count pages with the last page number last
// a blank page must follow. The blank page is then the last page of the
// sequence, not the page before it.
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
			// The last page should be odd if the next sequence
			// starts even, and vice versa.
			return (last%2 == 0) == (n%2 == 0), nil
		}
		return false, nil
	case "no-force":
		return false, nil
	default:
		return false, tree.Errorf(s.node, "force-page-count %q ist ungültig", v)
	}
}

// nextInitial returns a fixed initial-page-number of the next
// fo:page-sequence or 0. For auto-odd and auto-even it is 1 or 2, only
// the parity counts.
func (s *sequence) nextInitial() int {
	sib := s.node.Parent.Children
	for i, c := range sib {
		if c != s.node {
			continue
		}
		for _, n := range sib[i+1:] {
			if n.IsFO("page-sequence") {
				// For auto-odd and auto-even the parity is fixed (7.27.7).
				switch v := attrOr(n, "initial-page-number", ""); v {
				case "auto-odd":
					return 1
				case "auto-even":
					return 2
				default:
					k, _ := strconv.Atoi(v)
					return k
				}
			}
		}
	}
	return 0
}
