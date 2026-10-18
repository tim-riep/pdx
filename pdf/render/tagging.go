package render

import (
	"sort"
	"strings"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/pdf"
)

// Tagged PDF (ISO 32000-1 14.7, 14.8): Aus dem Area Tree entsteht ein
// logischer Strukturbaum. Ein Planungsdurchgang läuft vor dem Schreiben in
// genau der Reihenfolge, in der der Renderer zeichnet, und legt für jeden
// Bereich fest, ob er Inhalt eines Strukturelements (mit MCID) oder ein
// Artefakt ist. Beim Zeichnen umschließt der Renderer jeden Bereich mit der
// passenden markierten Inhaltsfolge, danach schreibt der Planer den Baum.

// structElem ist ein Strukturelement.
type structElem struct {
	typ    string
	parent *structElem
	kids   []structKid
	alt    string
	lang   string
	scope  string
	// colSpan und rowSpan sind die Spannen einer Tabellenzelle.
	colSpan, rowSpan int
	id     string // Vorlagen-ID, für Meldungen
	ref    pdf.Ref
	// generic ist ein Block ohne Rolle: P mit eigenem Inhalt, sonst Div.
	generic bool
	hasMC   bool
	// Lage für die Lesereihenfolge positionierter Elemente.
	positioned bool
	order      int
	located    bool
	page       int
	y, x       areatree.Length
}

// structKid ist ein Kind eines Strukturelements: Element, markierter
// Inhalt (Seite und MCID) oder Annotation.
type structKid struct {
	elem  *structElem
	page  int
	mcid  int
	annot *areatree.Link
}

// mark legt fest, wie ein Bereich gezeichnet wird.
type mark struct {
	tag      pdf.Name
	mcid     int
	artifact bool
	// hyphen: die letzte Glyphe ist ein Trennstrich aus dem Zeilenumbruch.
	hyphen bool
}

// tagCtx ist der Zustand beim Abstieg in den Area Tree.
type tagCtx struct {
	parent   *structElem
	artifact bool
	content  bool
	// id ist die Vorlagen-ID des nächsten Containers, für Meldungen.
	id string
}

type tableState struct {
	sections map[string]*structElem
	rows     map[int]*structElem
}

// tagger plant den Strukturbaum eines Dokuments.
type tagger struct {
	doc    *areatree.Document
	root   *structElem
	marks  map[areatree.Area]mark
	pages  [][]*structElem // je Seite das Strukturelement je MCID
	annots map[*areatree.Link]int
	// annotRefs sind die Annotationen in der Reihenfolge ihrer Schlüssel ab
	// der Seitenzahl.
	annotRefs []pdf.Ref
	// state merkt sich je Quelle eines Containers den Zustand für Kinder
	// und positionierte Nachfahren, last die letzte Instanz für
	// Fortsetzungen auf Folgeseiten.
	state  map[string]tagCtx
	last   map[string]*structElem
	tables map[*structElem]*tableState
	cells  map[string]*structElem
	links  map[string]*structElem
	inline map[*structElem]map[string]*structElem
	// linkText sammelt den Text je Verweis für /Contents der Annotation.
	linkText map[string]*strings.Builder
	// artifactLinks sind die Vorlagen-IDs von Verweisen in Artefakten.
	artifactLinks []string
}

func newTagger(doc *areatree.Document) *tagger {
	return &tagger{
		doc:      doc,
		root:     &structElem{typ: "Document", lang: doc.Meta.Lang},
		marks:    map[areatree.Area]mark{},
		annots:   map[*areatree.Link]int{},
		state:    map[string]tagCtx{},
		last:     map[string]*structElem{},
		tables:   map[*structElem]*tableState{},
		cells:    map[string]*structElem{},
		links:    map[string]*structElem{},
		inline:   map[*structElem]map[string]*structElem{},
		linkText: map[string]*strings.Builder{},
	}
}

// plan geht alle Seiten durch und baut den Strukturbaum.
func (t *tagger) plan() {
	t.pages = make([][]*structElem, len(t.doc.Pages))
	for pi, p := range t.doc.Pages {
		for _, a := range p.Areas {
			t.walk(pi, a, tagCtx{parent: t.root}, true)
		}
	}
	t.finish(t.root)
}

// foName liefert den lokalen Namen des FO-Elements aus der Quelle eines
// Containers, etwa "table-cell" aus "fo:table-cell@12:5".
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
	}
}

func (t *tagger) container(pi int, v *areatree.Container, c tagCtx, top bool) {
	var tg areatree.Tag
	if v.Tag != nil {
		tg = *v.Tag
	}
	// Positionierte Nachfahren setzen den Zustand ihres Vorfahren fort.
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
		t.marks[v] = mark{artifact: true} // Hintergrund und Rahmen sind Gestaltung
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
		// Positioniert ist ein Container auf Seitenebene außerhalb der
		// Regionen sowie jeder mit positioniertem Vorfahren.
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

// roleTypes bildet die Rollen der Vorlage auf Strukturtypen ab
// (ISO 32000-1 14.8.4).
var roleTypes = map[string]string{
	"H1": "H1", "H2": "H2", "H3": "H3", "H4": "H4", "H5": "H5", "H6": "H6",
	"P": "P", "Section": "Sect", "Table": "Table", "Figure": "Figure",
	"List": "L", "L": "L", "LI": "LI", "Lbl": "Lbl", "LBody": "LBody",
	"Note": "Note", "Caption": "Caption", "Link": "Link", "Formula": "Formula",
	"Reference": "Reference",
}

// elemFor liefert das Strukturelement eines Containers. Zeilen ohne Quelle
// und Regionen sind durchlässig.
func (t *tagger) elemFor(pi int, v *areatree.Container, parent *structElem, tg areatree.Tag) *structElem {
	if v.Source == "" || v.Kind == areatree.KindLine || v.Kind == areatree.KindRegion {
		return parent
	}
	fo := foName(v.Source)
	switch fo {
	case "table-cell", "list-item-label", "list-item-body":
		if tg.Row != 0 {
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
	se := &structElem{parent: parent, id: v.ID, alt: tg.Alt, lang: tg.Lang}
	switch {
	case roleTypes[v.Role] != "" && fo != "table-cell":
		se.typ = roleTypes[v.Role]
	case fo == "table":
		se.typ = "Table"
	case fo == "list-block":
		se.typ = "L"
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

// cell ordnet eine Zelle in Abschnitt und Zeile ihrer Tabelle oder ihres
// Listenpunkts ein.
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

// text plant einen Glyph-Run: Inhalt seines Elements, eines Verweises oder
// eines Inline-Elements mit eigener Rolle oder Sprache.
func (t *tagger) text(pi int, v *areatree.GlyphRun, c tagCtx) {
	if c.artifact {
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
	n := len(v.Glyphs)
	m.hyphen = n > 0 && v.Glyphs[n-1].Hyphen
	t.marks[v] = m
}

// figure plant ein Bild oder einen Barcode als Inhalt einer Abbildung.
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

// content vergibt die nächste MCID der Seite an ein Element.
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

// linkElem liefert das Link-Element eines Verweises, das Text und
// Annotation verbindet.
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

// inlineElem liefert das Element eines Inline-Elements mit Rolle oder
// Sprache. Aufeinanderfolgende Runs desselben Elements teilen es.
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

// finish legt die Typen allgemeiner Blöcke fest, entfernt leere Elemente
// und ordnet positionierte Kinder in die Lesereihenfolge ein.
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
		// Der Kopf wird nach dem Rumpf gezeichnet, gehört aber davor
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

// readingOrder ordnet positionierte Kinder zwischen die übrigen ein: nach
// readingOrder, sonst von oben nach unten und links nach rechts, jeweils vor
// dem ersten fließenden Kind derselben Seite, das tiefer beginnt.
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

// markOf liefert die Markierung eines Bereichs, false ohne Tagging.
func (t *tagger) markOf(a areatree.Area) (mark, bool) {
	if t == nil {
		return mark{}, false
	}
	return t.marks[a], true
}

// annotation versieht eine Link-Annotation mit ihrem Schlüssel im
// ParentTree und dem Linktext als /Contents (ISO 14289-1 7.18.1).
func (t *tagger) annotation(l *areatree.Link, a *pdf.Dict, ref pdf.Ref) {
	if _, ok := t.annots[l]; !ok {
		return
	}
	t.annots[l] = len(t.doc.Pages) + len(t.annotRefs)
	t.annotRefs = append(t.annotRefs, ref)
	a.Set("StructParent", pdf.Int(t.annots[l]))
	if b := t.linkText[l.Key]; b != nil && strings.TrimSpace(b.String()) != "" {
		a.Set("Contents", pdf.TextString(strings.TrimSpace(b.String())))
	}
}

// write schreibt den Strukturbaum und hängt ihn an den Katalog
// (ISO 32000-1 14.7.2, 14.7.4.4).
func (t *tagger) write(r *renderer) {
	root := r.d.Reserve()
	t.reserve(r, t.root)
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
	r.d.Set(root, pdf.D(
		"Type", pdf.Name("StructTreeRoot"),
		"K", docRef,
		"ParentTree", pdf.D("Nums", nums),
		"ParentTreeNextKey", pdf.Int(len(t.doc.Pages)+len(t.annotRefs)),
	))
	r.catalog.Set("StructTreeRoot", root)
	r.catalog.Set("MarkInfo", pdf.D("Marked", pdf.Bool(true)))
}

type annotKey struct {
	key  int
	elem *structElem
}

// sortedAnnots liefert die Annotationen nach Schlüssel mit ihrem
// Link-Element.
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
	d := pdf.D("Type", pdf.Name("StructElem"), "S", pdf.Name(se.typ), "P", parent)
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
	// Tabellenattribute (ISO 32000-1 14.8.5.7).
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
