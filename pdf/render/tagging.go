package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/pdf"
)

// Tagged PDF (ISO 32000-1 14.7, 14.8): A logical structure tree is built
// from the area tree. A planning pass runs before writing in
// exactly the order in which the renderer draws, and decides for every
// area whether it is content of a structure element (with MCID) or an
// artifact. When drawing, the renderer encloses every area with the
// matching marked-content sequence, afterwards the planner writes the tree.

// structElem is a structure element.
type structElem struct {
	typ    string
	parent *structElem
	kids   []structKid
	alt    string
	lang   string
	scope  string
	// colSpan and rowSpan are the spans of a table cell.
	colSpan, rowSpan int
	// label is the accessible name of a form field, form marks the
	// Form element of a widget.
	label string
	form  bool
	id    string // template ID, for messages
	ref   pdf.Ref
	// generic is a block without a role: P with own content, otherwise Div.
	generic bool
	hasMC   bool
	// Position for the reading order of positioned elements.
	positioned bool
	order      int
	located    bool
	page       int
	y, x       areatree.Length
}

// structKid is a kid of a structure element: element, marked
// content (page and MCID) or annotation.
type structKid struct {
	elem *structElem
	page int
	mcid int
	// annot is an annotation: link or widget.
	annot areatree.Area
}

// mark defines how an area is drawn.
type mark struct {
	tag      pdf.Name
	mcid     int
	artifact bool
	// actual is a replacement text for the whole sequence, for example for
	// flattened check boxes.
	actual string
	// hyphen: the last glyph is a hyphen from the line break.
	hyphen bool
}

// tagCtx is the state during the descent into the area tree.
type tagCtx struct {
	parent   *structElem
	artifact bool
	content  bool
	// id is the template ID of the next container, for messages.
	id string
}

type tableState struct {
	sections map[string]*structElem
	rows     map[int]*structElem
}

// tagger plans the structure tree of a document.
type tagger struct {
	doc    *areatree.Document
	root   *structElem
	marks  map[areatree.Area]mark
	pages  [][]*structElem // per page the structure element per MCID
	annots map[areatree.Area]int
	// annotRefs are the annotations in the order of their keys starting at
	// the page count.
	annotRefs []pdf.Ref
	// state remembers per source of a container the state for kids
	// and positioned descendants, last the last instance for
	// continuations on following pages.
	state  map[string]tagCtx
	last   map[string]*structElem
	tables map[*structElem]*tableState
	cells  map[string]*structElem
	links  map[string]*structElem
	inline map[*structElem]map[string]*structElem
	// linkText collects the text per link for /Contents of the annotation.
	linkText map[string]*strings.Builder
	// artifactLinks and artifactWidgets are the template IDs of
	// links and form fields in artifacts.
	artifactLinks, artifactWidgets []string
	// labels collects the text of the list labels per list for ListNumbering.
	labels map[*structElem]string
	// pdf2 puts the elements into the namespace of PDF 2.0
	// (ISO 32000-2 14.8.6), as PDF/UA-2 requires for Document.
	pdf2 bool
	// root is the object of the StructTreeRoot, ns that of the namespace.
	rootRef, ns pdf.Ref
	// ids are the assigned element IDs in tree order for the IDTree.
	ids []idEntry
	// texts collects the text per element for Analyze.
	texts map[*structElem]*strings.Builder
	// flatten: widgets are fixed page content instead of annotations.
	flatten bool
}

type idEntry struct {
	id  string
	ref pdf.Ref
}

func newTagger(doc *areatree.Document) *tagger {
	return &tagger{
		doc:      doc,
		root:     &structElem{typ: "Document", lang: doc.Meta.Lang},
		marks:    map[areatree.Area]mark{},
		annots:   map[areatree.Area]int{},
		state:    map[string]tagCtx{},
		last:     map[string]*structElem{},
		tables:   map[*structElem]*tableState{},
		cells:    map[string]*structElem{},
		links:    map[string]*structElem{},
		inline:   map[*structElem]map[string]*structElem{},
		linkText: map[string]*strings.Builder{},
		labels:   map[*structElem]string{},
		texts:    map[*structElem]*strings.Builder{},
	}
}

// plan goes through all pages and builds the structure tree.
func (t *tagger) plan() {
	t.pages = make([][]*structElem, len(t.doc.Pages))
	for pi, p := range t.doc.Pages {
		for _, a := range p.Areas {
			t.walk(pi, a, tagCtx{parent: t.root}, true)
		}
	}
	t.finish(t.root)
}

// foName returns the local name of the FO element from the source of a
// container, for example "table-cell" from "fo:table-cell@12:5".
func foName(src string) string {
	s := strings.TrimPrefix(src, "fo:")
	if i := strings.IndexByte(s, '@'); i >= 0 {
		s = s[:i]
	}
	return s
}

func (t *tagger) walk(pi int, a areatree.Area, c tagCtx, top bool) {
	switch v := a.(type) {
	case *areatree.Container:
		t.container(pi, v, c, top)
	case *areatree.GlyphRun:
		t.text(pi, v, c)
	case *areatree.Image:
		t.figure(pi, a, v.Rect, v.ID, c)
	case *areatree.Path:
		t.figure(pi, a, v.Rect, v.ID, c)
	case *areatree.Line, *areatree.Box:
		t.marks[a] = mark{artifact: true}
	case *areatree.Link:
		if c.artifact {
			t.artifactLinks = append(t.artifactLinks, c.id)
			return
		}
		le := t.linkElem(c.parent, v.Key, v.Rect, pi)
		le.kids = append(le.kids, structKid{annot: v, page: pi})
		t.annots[v] = 0
	case *areatree.Widget:
		if t.flatten {
			t.flatWidget(pi, v, c)
			return
		}
		if c.artifact {
			t.artifactWidgets = append(t.artifactWidgets, v.ID)
			return
		}
		// One Form element per widget with exactly one object reference
		// (ISO 14289-1 7.18.4, ISO 14289-2 8.10.1).
		se := &structElem{typ: "Form", parent: c.parent, id: v.ID, label: v.Field.Label, form: true}
		se.located, se.page, se.y, se.x = true, pi, v.Y, v.X
		c.parent.kids = append(c.parent.kids, structKid{elem: se})
		se.kids = append(se.kids, structKid{annot: v, page: pi})
		t.annots[v] = 0
	}
}

func (t *tagger) container(pi int, v *areatree.Container, c tagCtx, top bool) {
	var tg areatree.Tag
	if v.Tag != nil {
		tg = *v.Tag
	}
	// Positioned descendants continue the state of their ancestor.
	if tg.Parent != "" {
		if st, ok := t.state[tg.Parent]; ok {
			c = st
		}
	}
	art := c.artifact
	if !art {
		switch {
		case tg.Artifact, tg.Repeat, v.Role == "Artifact":
			art = true
		case tg.Static && !tg.Content && !c.content:
			art = true
		}
	}
	content := c.content || tg.Content
	if v.ID != "" {
		c.id = v.ID
	}
	if v.Background != nil || hasBorder(v.Border) {
		t.marks[v] = mark{artifact: true} // background and borders are decoration
	}
	if art {
		st := tagCtx{parent: c.parent, artifact: true, id: c.id}
		t.state[v.Source] = st
		for _, ch := range v.Children {
			t.walk(pi, ch, st, false)
		}
		return
	}
	se := t.elemFor(pi, v, c.parent, tg)
	if se != c.parent && !se.located {
		se.located, se.page, se.y, se.x = true, pi, v.Y, v.X
		// Positioned is a container at page level outside the
		// regions, and every one with a positioned ancestor.
		if (top && v.Kind != areatree.KindRegion) || tg.Parent != "" {
			se.positioned, se.order = true, tg.Order
		}
	}
	st := tagCtx{parent: se, content: content, id: c.id}
	if v.Source != "" {
		t.state[v.Source] = st
	}
	for _, ch := range v.Children {
		t.walk(pi, ch, st, false)
	}
}

func hasBorder(b [4]areatree.Border) bool {
	for _, e := range b {
		if e.Visible() {
			return true
		}
	}
	return false
}

// roleTypes maps the roles of the template to structure types
// (ISO 32000-1 14.8.4).
var roleTypes = map[string]string{
	"H1": "H1", "H2": "H2", "H3": "H3", "H4": "H4", "H5": "H5", "H6": "H6",
	"P": "P", "Section": "Sect", "Table": "Table", "Figure": "Figure",
	"List": "L", "L": "L", "LI": "LI", "Lbl": "Lbl", "LBody": "LBody",
	"Note": "Note", "Caption": "Caption", "Link": "Link", "Formula": "Formula",
	"Reference": "Reference",
}

// elemFor returns the structure element of a container. Lines without a source
// and regions are transparent.
func (t *tagger) elemFor(pi int, v *areatree.Container, parent *structElem, tg areatree.Tag) *structElem {
	if v.Source == "" || v.Kind == areatree.KindLine || v.Kind == areatree.KindRegion {
		return parent
	}
	fo := foName(v.Source)
	switch fo {
	case "table-cell", "list-item-label", "list-item-body":
		if tg.Row != 0 && parent.typ != "LI" {
			if se := t.cell(v, parent, tg, fo); se != nil {
				return se
			}
		}
	}
	key := v.Source + "|" + v.ID
	if tg.Continued {
		if se, ok := t.last[key]; ok {
			return se
		}
	}
	// A block without a role in a list label belongs to the label.
	if parent.typ == "Lbl" && fo == "block" && v.Role == "" && tg.Alt == "" && tg.Lang == "" {
		return parent
	}
	se := &structElem{parent: parent, id: v.ID, alt: tg.Alt, lang: tg.Lang}
	switch {
	case roleTypes[v.Role] != "" && fo != "table-cell":
		se.typ = roleTypes[v.Role]
	case fo == "table":
		se.typ = "Table"
	case fo == "list-block":
		se.typ = "L"
	case fo == "list-item":
		se.typ = "LI"
	case fo == "list-item-label" && parent.typ == "LI":
		se.typ = "Lbl"
	case fo == "list-item-body" && parent.typ == "LI":
		se.typ = "LBody"
	case fo == "inline":
		se.typ = "Span"
	case tg.Alt != "":
		se.typ = "Figure"
	case fo == "block":
		se.typ, se.generic = "P", true
	default:
		se.typ, se.generic = "Div", true
	}
	parent.kids = append(parent.kids, structKid{elem: se})
	t.last[key] = se
	return se
}

var sectionTypes = map[string]string{"header": "THead", "body": "TBody", "footer": "TFoot"}

// cell places a cell into the section and row of its table or its
// list item.
func (t *tagger) cell(v *areatree.Container, parent *structElem, tg areatree.Tag, fo string) *structElem {
	ts := t.tables[parent]
	if ts == nil {
		ts = &tableState{sections: map[string]*structElem{}, rows: map[int]*structElem{}}
		t.tables[parent] = ts
	}
	key := v.Source + "|" + v.ID + "|" + itoa(tg.Row)
	if se, ok := t.cells[key]; ok {
		return se
	}
	row := ts.rows[tg.Row]
	switch {
	case parent.typ == "Table":
		if row == nil {
			sec := ts.sections[tg.Section]
			if sec == nil {
				sec = &structElem{typ: sectionTypes[tg.Section], parent: parent}
				parent.kids = append(parent.kids, structKid{elem: sec})
				ts.sections[tg.Section] = sec
			}
			row = &structElem{typ: "TR", parent: sec}
			sec.kids = append(sec.kids, structKid{elem: row})
			ts.rows[tg.Row] = row
		}
		se := &structElem{typ: "TD", parent: row, id: v.ID, lang: tg.Lang, alt: tg.Alt, colSpan: tg.ColSpan, rowSpan: tg.RowSpan}
		if tg.Section == "header" || tg.Scope != "" {
			se.typ, se.scope = "TH", tg.Scope
			if se.scope == "" {
				se.scope = "Column"
			}
		}
		row.kids = append(row.kids, structKid{elem: se})
		t.cells[key] = se
		return se
	case parent.typ == "L":
		if row == nil {
			row = &structElem{typ: "LI", parent: parent}
			parent.kids = append(parent.kids, structKid{elem: row})
			ts.rows[tg.Row] = row
		}
		typ := "LBody"
		if fo == "list-item-label" {
			typ = "Lbl"
		}
		se := &structElem{typ: typ, parent: row, id: v.ID, lang: tg.Lang}
		row.kids = append(row.kids, structKid{elem: se})
		t.cells[key] = se
		return se
	}
	return nil
}

func itoa(n int) string {
	var b [20]byte
	i := len(b)
	if n == 0 {
		return "0"
	}
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// text plans a glyph run: content of its element, of a link or
// of an inline element with its own role or language.
func (t *tagger) text(pi int, v *areatree.GlyphRun, c tagCtx) {
	if c.artifact || v.Decor {
		t.marks[v] = mark{artifact: true}
		return
	}
	target := c.parent
	switch {
	case v.Link != "":
		target = t.linkElem(c.parent, v.Link, areatree.Rect{X: v.Origin.X, Y: v.Origin.Y - v.Ascent}, pi)
		b := t.linkText[v.Link]
		if b == nil {
			b = &strings.Builder{}
			t.linkText[v.Link] = b
		}
		for _, g := range v.Glyphs {
			if !g.Hyphen {
				b.WriteString(g.Text)
			}
		}
	case v.ID != "":
		if it, ok := t.doc.Inline[v.ID]; ok {
			target = t.inlineElem(c.parent, v.ID, it)
		}
	}
	m := t.content(pi, target, areatree.Rect{X: v.Origin.X, Y: v.Origin.Y - v.Ascent})
	tb := t.texts[target]
	if tb == nil {
		tb = &strings.Builder{}
		t.texts[target] = tb
	}
	for _, g := range v.Glyphs {
		if !g.Hyphen {
			tb.WriteString(g.Text)
		}
	}
	// The text of a label determines the numbering of its list.
	for e := target; e != nil; e = e.parent {
		if e.typ == "Lbl" {
			if li := e.parent; li != nil && li.parent != nil && li.parent.typ == "L" {
				for _, g := range v.Glyphs {
					t.labels[li.parent] += g.Text
				}
			}
			break
		}
	}
	n := len(v.Glyphs)
	m.hyphen = n > 0 && v.Glyphs[n-1].Hyphen
	t.marks[v] = m
}

// figure plans an image or a barcode as content of a figure.
func (t *tagger) figure(pi int, a areatree.Area, r areatree.Rect, id string, c tagCtx) {
	if c.artifact {
		t.marks[a] = mark{artifact: true}
		return
	}
	target := c.parent
	if target.typ != "Figure" {
		if id == "" {
			id = c.id
		}
		target = &structElem{typ: "Figure", parent: c.parent, id: id}
		c.parent.kids = append(c.parent.kids, structKid{elem: target})
	}
	t.marks[a] = t.content(pi, target, r)
}

// content assigns the next MCID of the page to an element.
func (t *tagger) content(pi int, se *structElem, r areatree.Rect) mark {
	mcid := len(t.pages[pi])
	t.pages[pi] = append(t.pages[pi], se)
	se.kids = append(se.kids, structKid{page: pi, mcid: mcid})
	se.hasMC = true
	for e := se; e != nil && !e.located; e = e.parent {
		e.located, e.page, e.y, e.x = true, pi, r.Y, r.X
	}
	return mark{tag: pdf.Name(se.typ), mcid: mcid}
}

// linkElem returns the Link element of a link, which connects text and
// annotation.
func (t *tagger) linkElem(parent *structElem, key string, r areatree.Rect, pi int) *structElem {
	if se, ok := t.links[key]; ok && key != "" {
		return se
	}
	se := &structElem{typ: "Link", parent: parent}
	parent.kids = append(parent.kids, structKid{elem: se})
	if key != "" {
		t.links[key] = se
	}
	return se
}

// inlineElem returns the element of an inline element with role or
// language. Consecutive runs of the same element share it.
func (t *tagger) inlineElem(parent *structElem, id string, it areatree.InlineTag) *structElem {
	if it.Role == "" && it.Tag.Lang == "" && it.Tag.Alt == "" {
		return parent
	}
	m := t.inline[parent]
	if m == nil {
		m = map[string]*structElem{}
		t.inline[parent] = m
	}
	if se, ok := m[id]; ok && len(parent.kids) > 0 && parent.kids[len(parent.kids)-1].elem == se {
		return se
	}
	typ := "Span"
	if r := roleTypes[it.Role]; r != "" {
		typ = r
	}
	se := &structElem{typ: typ, parent: parent, id: id, lang: it.Tag.Lang, alt: it.Tag.Alt}
	parent.kids = append(parent.kids, structKid{elem: se})
	m[id] = se
	return se
}

// finish sets the types of generic blocks, removes empty elements
// and places positioned kids into the reading order.
func (t *tagger) finish(se *structElem) bool {
	var kids []structKid
	for _, k := range se.kids {
		if k.elem != nil && !t.finish(k.elem) {
			continue
		}
		kids = append(kids, k)
	}
	se.kids = kids
	if se.generic {
		if se.hasMC {
			se.typ = "P"
		} else {
			se.typ = "Div"
		}
	}
	se.kids = readingOrder(se.kids)
	if se.typ == "Table" {
		// The header is drawn after the body but belongs before it
		// (ISO 32000-1 14.8.4.3.4).
		sort.SliceStable(se.kids, func(i, j int) bool {
			return sectionRank(se.kids[i]) < sectionRank(se.kids[j])
		})
	}
	return len(se.kids) > 0 || se.typ == "Document"
}

func sectionRank(k structKid) int {
	if k.elem == nil {
		return 2
	}
	switch k.elem.typ {
	case "Caption":
		return 0
	case "THead":
		return 1
	case "TFoot":
		return 3
	}
	return 2
}

// readingOrder places positioned kids between the others: by
// readingOrder, otherwise top to bottom and left to right, each before
// the first flowing kid of the same page that starts lower.
func readingOrder(kids []structKid) []structKid {
	var flow, pos []structKid
	for _, k := range kids {
		if k.elem != nil && k.elem.positioned {
			pos = append(pos, k)
		} else {
			flow = append(flow, k)
		}
	}
	if len(pos) == 0 {
		return kids
	}
	sort.SliceStable(pos, func(i, j int) bool {
		a, b := pos[i].elem, pos[j].elem
		if a.page != b.page {
			return a.page < b.page
		}
		if (a.order != 0) != (b.order != 0) {
			return a.order != 0
		}
		if a.order != b.order {
			return a.order < b.order
		}
		if a.y != b.y {
			return a.y < b.y
		}
		return a.x < b.x
	})
	out := make([]structKid, 0, len(kids))
	fi := 0
	for _, p := range pos {
		for fi < len(flow) {
			f := flow[fi]
			page, y := f.page, areatree.Length(0)
			if f.elem != nil {
				page, y = f.elem.page, f.elem.y
			}
			if page > p.elem.page || (page == p.elem.page && f.elem != nil && y > p.elem.y) {
				break
			}
			out = append(out, f)
			fi++
		}
		out = append(out, p)
	}
	return append(out, flow[fi:]...)
}

// markOf returns the marking of an area, false without tagging.
func (t *tagger) markOf(a areatree.Area) (mark, bool) {
	if t == nil {
		return mark{}, false
	}
	return t.marks[a], true
}

// annotation gives a link annotation its key in the
// ParentTree and the link text as /Contents (ISO 14289-1 7.18.1).
func (t *tagger) annotation(l *areatree.Link, a *pdf.Dict, ref pdf.Ref) {
	if !t.register(l, a, ref) {
		return
	}
	if b := t.linkText[l.Key]; b != nil && strings.TrimSpace(b.String()) != "" {
		a.Set("Contents", pdf.TextString(strings.TrimSpace(b.String())))
	}
}

// register assigns the key of an annotation in the ParentTree and sets
// StructParent. false if the annotation is not in the tree.
func (t *tagger) register(area areatree.Area, a *pdf.Dict, ref pdf.Ref) bool {
	if _, ok := t.annots[area]; !ok {
		return false
	}
	t.annots[area] = len(t.doc.Pages) + len(t.annotRefs)
	t.annotRefs = append(t.annotRefs, ref)
	a.Set("StructParent", pdf.Int(t.annots[area]))
	return true
}

// write writes the structure tree and attaches it to the catalog
// (ISO 32000-1 14.7.2, 14.7.4.4).
// prepare reserves the objects of the tree before drawing so that
// annotations and bookmarks can refer to elements.
func (t *tagger) prepare(r *renderer) {
	t.rootRef = r.d.Reserve()
	if t.pdf2 {
		t.ns = r.d.Add(pdf.D("Type", pdf.Name("Namespace"), "NS", pdf.String(ns20)))
	}
	t.reserve(r, t.root)
}

// ns20 is the namespace of the structure types of PDF 2.0 (ISO 32000-2 14.8.6).
const ns20 = "http://iso.org/pdf2/ssn"

// destElem returns the structure element at the destination of a link, nil if
// the destination is a page or an artifact.
func (t *tagger) destElem(d areatree.Dest) *structElem {
	st, ok := t.state[d.Source]
	if !ok || st.artifact || st.parent == nil || st.parent == t.root {
		return nil
	}
	for se := st.parent; se != nil; se = se.parent {
		if se.ref.Num != 0 {
			return se
		}
	}
	return nil
}

func (t *tagger) write(r *renderer) {
	root := t.rootRef
	docRef := t.root.ref
	t.writeElem(r, t.root, root)
	var nums pdf.Array
	for pi, ses := range t.pages {
		if len(ses) == 0 {
			continue
		}
		arr := make(pdf.Array, len(ses))
		for i, se := range ses {
			arr[i] = se.ref
		}
		nums = append(nums, pdf.Int(pi), arr)
	}
	for _, key := range t.sortedAnnots() {
		nums = append(nums, pdf.Int(key.key), key.elem.ref)
	}
	str := pdf.D(
		"Type", pdf.Name("StructTreeRoot"),
		"K", docRef,
		"ParentTree", pdf.D("Nums", nums),
		"ParentTreeNextKey", pdf.Int(len(t.doc.Pages)+len(t.annotRefs)),
	)
	if t.pdf2 {
		str.Set("Namespaces", pdf.Array{t.ns})
	}
	if len(t.ids) > 0 {
		// The name tree requires keys sorted by bytes.
		sort.Slice(t.ids, func(i, j int) bool { return t.ids[i].id < t.ids[j].id })
		var arr pdf.Array
		for _, e := range t.ids {
			arr = append(arr, pdf.String(e.id), e.ref)
		}
		str.Set("IDTree", pdf.D("Names", arr))
	}
	r.d.Set(root, str)
	r.catalog.Set("StructTreeRoot", root)
	r.catalog.Set("MarkInfo", pdf.D("Marked", pdf.Bool(true)))
}

type annotKey struct {
	key  int
	elem *structElem
}

// sortedAnnots returns the annotations by key with their
// Link element.
func (t *tagger) sortedAnnots() []annotKey {
	var out []annotKey
	var visit func(se *structElem)
	visit = func(se *structElem) {
		for _, k := range se.kids {
			switch {
			case k.elem != nil:
				visit(k.elem)
			case k.annot != nil:
				if key, ok := t.annots[k.annot]; ok && key >= len(t.doc.Pages) {
					out = append(out, annotKey{key, se})
				}
			}
		}
	}
	visit(t.root)
	sort.Slice(out, func(i, j int) bool { return out[i].key < out[j].key })
	return out
}

func (t *tagger) reserve(r *renderer, se *structElem) {
	se.ref = r.d.Reserve()
	for _, k := range se.kids {
		if k.elem != nil {
			t.reserve(r, k.elem)
		}
	}
}

func (t *tagger) writeElem(r *renderer, se *structElem, parent pdf.Ref) {
	typ := se.typ
	if t.pdf2 && typ == "Note" {
		typ = "FENote" // Note does not exist in the namespace of PDF 2.0
	}
	d := pdf.D("Type", pdf.Name("StructElem"), "S", pdf.Name(typ), "P", parent)
	if t.pdf2 {
		d.Set("NS", t.ns)
	}
	if typ == "Note" {
		// PDF/UA-1 requires a unique ID per Note (ISO 14289-1 7.9).
		id := fmt.Sprintf("note-%d", len(t.ids)+1)
		d.Set("ID", pdf.String(id))
		t.ids = append(t.ids, idEntry{id, se.ref})
	}
	if typ == "L" {
		if lbl, ok := t.labels[se]; ok {
			d.Set("A", pdf.D("O", pdf.Name("List"), "ListNumbering", pdf.Name(listNumbering(lbl, t.pdf2))))
		}
	}
	pg := -1
	for _, k := range se.kids {
		if k.elem == nil {
			pg = k.page
			break
		}
	}
	if pg >= 0 {
		d.Set("Pg", r.pageRefs[pg])
	}
	var kids pdf.Array
	for _, k := range se.kids {
		switch {
		case k.elem != nil:
			kids = append(kids, k.elem.ref)
			t.writeElem(r, k.elem, se.ref)
		case k.annot != nil:
			key, ok := t.annots[k.annot]
			if !ok || key < len(t.doc.Pages) {
				continue
			}
			kids = append(kids, pdf.D("Type", pdf.Name("OBJR"), "Obj", t.annotRefs[key-len(t.doc.Pages)], "Pg", r.pageRefs[k.page]))
		case k.page == pg:
			kids = append(kids, pdf.Int(k.mcid))
		default:
			kids = append(kids, pdf.D("Type", pdf.Name("MCR"), "Pg", r.pageRefs[k.page], "MCID", pdf.Int(k.mcid)))
		}
	}
	if len(kids) == 1 {
		d.Set("K", kids[0])
	} else if len(kids) > 0 {
		d.Set("K", kids)
	}
	if se.lang != "" && (se.parent == nil || se.lang != se.parent.lang) {
		d.Set("Lang", pdf.TextString(se.lang))
	}
	if se.alt != "" {
		d.Set("Alt", pdf.TextString(se.alt))
	}
	// Table attributes (ISO 32000-1 14.8.5.7).
	if se.scope != "" || se.colSpan > 1 || se.rowSpan > 1 {
		a := pdf.D("O", pdf.Name("Table"))
		if se.scope != "" {
			a.Set("Scope", pdf.Name(se.scope))
		}
		if se.colSpan > 1 {
			a.Set("ColSpan", pdf.Int(se.colSpan))
		}
		if se.rowSpan > 1 {
			a.Set("RowSpan", pdf.Int(se.rowSpan))
		}
		d.Set("A", a)
	}
	r.d.Set(se.ref, d)
}

// listNumbering derives the numbering of a list from the text of its
// labels (ISO 32000-1 14.8.5.5, Table 347). Digits give Decimal,
// letters LowerAlpha or UpperAlpha, other characters unordered: Disc in
// PDF 1.7, Unordered in PDF 2.0. PDF/UA-2 forbids None for labels
// (ISO 14289-2 8.2.5.25).
func listNumbering(labels string, pdf2 bool) string {
	for _, r := range labels {
		switch {
		case r >= '0' && r <= '9':
			return "Decimal"
		case r >= 'a' && r <= 'z':
			return "LowerAlpha"
		case r >= 'A' && r <= 'Z':
			return "UpperAlpha"
		}
	}
	if pdf2 {
		return "Unordered"
	}
	return "Disc"
}

// tagWidget binds a widget into the structure tree through its Form element.
// The accessible name is TU on the field (ISO 14289-1 7.18.1) and
// Contents on the widget, which PDF/UA-2 requires without Lbl (ISO 14289-2
// 8.10.2.3). For radio buttons, Contents additionally names the option.
func (r *renderer) tagWidget(w *areatree.Widget, annot *pdf.Dict, ref pdf.Ref) {
	if r.tag == nil || !r.tag.register(w, annot, ref) {
		return
	}
	label := w.Field.Label
	if w.Field.Type == "radio" && w.Field.Export != "" {
		label = strings.TrimSpace(label + " " + w.Field.Export)
	}
	if label != "" {
		annot.Set("Contents", pdf.TextString(label))
	}
}

// flatWidget plans a flattened form field as content of its
// paragraph. Because the text is in the form XObject, the sequence carries
// the value as replacement text, buttons carry label and state. Empty fields
// and signature fields are only decoration.
func (t *tagger) flatWidget(pi int, v *areatree.Widget, c tagCtx) {
	f := v.Field
	empty := (f.Type == "text" || f.Type == "combo" || f.Type == "list") && strings.TrimSpace(f.Value) == ""
	if c.artifact || f.Type == "signature" || empty {
		t.marks[v] = mark{artifact: true}
		return
	}
	// The content belongs to the enclosing paragraph, for example together with
	// the text of an option.
	m := t.content(pi, c.parent, v.Rect)
	switch f.Type {
	case "text":
		m.actual = f.Value
	case "checkbox":
		state := "nicht angekreuzt"
		if f.Value == f.OnValue {
			state = "angekreuzt"
		}
		m.actual = strings.TrimSpace(f.Label + ": " + state)
	case "radio":
		// The text of the option follows in the same paragraph.
		m.actual = "nicht gewählt:"
		if f.Value == f.Export {
			m.actual = "gewählt:"
		}
	case "list", "combo":
		m.actual = strings.TrimSpace(f.Label + ": " + optionLabelOf(f))
	}
	t.marks[v] = m
}

// optionLabelOf returns the text of the selected option.
func optionLabelOf(f areatree.Field) string {
	for _, o := range f.Options {
		if o.Value == f.Value {
			return o.Label
		}
	}
	return f.Value
}
