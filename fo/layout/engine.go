// Package layout setzt einen FO-Baum in einen Area Tree um: Seitenvorlagen,
// Regionen, Fluss mit Seitenumbruch, statische Inhalte, Blöcke mit Rahmen und
// Hintergrund, Zeilenumbruch, absolute Positionierung und Bilder.
//
// Die Engine arbeitet mit einem Cursor je Referenzbereich. Blöcke werden
// geöffnet, ihre Zeilen auf die aktuelle Seite gesetzt und bei einem
// Seitenumbruch in Fragmente geteilt. Abstände (space-before, space-after)
// werden zusammengefasst und am Seitenanfang verworfen.
package layout

import (
	"context"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/fo/linebreak"
	"github.com/tim-riep/pdx/fo/props"
	"github.com/tim-riep/pdx/fo/tree"
	"github.com/tim-riep/pdx/font"
	"github.com/tim-riep/pdx/image"
	"strings"
)

// Options steuert das Layout.
type Options struct {
	// Fonts löst Schriftfamilien auf. nil bedeutet font.NewRegistry().
	Fonts *font.Registry
	// Resolver lädt Bilder und Anhänge. nil verweigert jeden Zugriff.
	Resolver Resolver
	// Breaker bricht Absätze um. nil bedeutet linebreak.KnuthPlass.
	Breaker linebreak.Breaker
	// Context bricht das Layout ab, geprüft je Durchlauf und je Seite. nil
	// bedeutet ohne Abbruch.
	Context context.Context
}

type engine struct {
	// rowNums nummeriert Tabellenzeilen und Listenpunkte für Tagged PDF.
	rowNums map[*rowLayout]int

	opts       Options
	doc        *areatree.Document
	masters    map[string]*master
	seqMasters map[string]*seqMaster
	fontIDs    map[*font.Face]areatree.FontID
	imgIDs     map[string]areatree.ImageID
	pageNo     int

	// prev enthält die Texte seitenabhängiger Inhalte aus dem vorigen
	// Durchlauf, next die dieses Durchlaufs. unstable ist gesetzt, wenn sich
	// eine reservierte Breite als falsch erwiesen hat.
	prev, next map[*tree.Node]string
	unstable   bool

	// ids ordnet jeder ID die erste und letzte Seite zu (formatiert).
	ids map[string]*idPages
	// cited merkt sich die in diesem Durchlauf ausgegebenen Verweise.
	cited map[*tree.Node]citation
	// prevIDs sind die Seiten der IDs aus dem vorigen Durchlauf.
	prevIDs map[string]*idPages
	// format ist das Seitenzahlformat der aktuellen Seitenfolge.
	format string
	// links sind interne Verweise und Lesezeichen zur Prüfung am Ende.
	links []linkCheck

	// marks sind die Marker-Ereignisse je Seite, markOrder in Seitenfolge.
	marks     map[*areatree.Page]*pageMarks
	markOrder []*pageMarks
	curSeq    *tree.Node
}

// idPages sind die erste und letzte Seite einer ID.
type idPages struct {
	first, last string
}

// citation ist ein ausgegebener Seitenverweis.
type citation struct {
	ref  string
	last bool
	text string
}

// recordID vermerkt, dass ein Bereich mit der ID auf einer Seite liegt.
func (e *engine) recordID(id string, page string) {
	if p, ok := e.ids[id]; ok {
		p.last = page
		return
	}
	e.ids[id] = &idPages{first: page, last: page}
}

// recordDest vermerkt die erste Lage einer ID als Sprungziel.
func (e *engine) recordDest(id string, page *areatree.Page, at areatree.Point) {
	if e.doc.Dests == nil {
		e.doc.Dests = map[string]areatree.Dest{}
	}
	if _, ok := e.doc.Dests[id]; ok {
		return
	}
	for i := len(e.doc.Pages) - 1; i >= 0; i-- {
		if e.doc.Pages[i] == page {
			e.doc.Dests[id] = areatree.Dest{Page: i, At: at}
			return
		}
	}
}

// links sind die internen Verweise eines Durchlaufs, geprüft am Ende.
type linkCheck struct {
	node *tree.Node
	id   string
}

// checkLinks prüft, dass interne Verweise und Lesezeichen auf vorhandene IDs
// zeigen.
func (e *engine) checkLinks() error {
	for _, l := range e.links {
		if _, ok := e.doc.Dests[l.id]; !ok {
			return tree.Errorf(l.node, "internal-destination %q verweist auf keine ID im Dokument", l.id)
		}
	}
	return nil
}

// citationText liefert den Text eines Verweises aus dem vorigen Durchlauf.
// Fehlt er, wird geschätzt und ein weiterer Durchlauf ist nötig.
func (e *engine) citationText(n *tree.Node, ref string, last bool) string {
	text := "1"
	if p, ok := e.prevIDs[ref]; ok {
		text = p.first
		if last {
			text = p.last
		}
	}
	e.cited[n] = citation{ref: ref, last: last, text: text}
	return text
}

// checkCitations prüft nach einem Durchlauf alle Verweise.
func (e *engine) checkCitations() error {
	for n, c := range e.cited {
		p, ok := e.ids[c.ref]
		if !ok {
			return tree.Errorf(n, "ref-id %q verweist auf keine ID im Dokument", c.ref)
		}
		want := p.first
		if c.last {
			want = p.last
		}
		if want != c.text {
			e.unstable = true
		}
	}
	return nil
}

// maxPasses begrenzt die Durchläufe für seitenabhängige Inhalte.
const maxPasses = 4

// estimate vermerkt den tatsächlichen Text eines seitenabhängigen Inhalts.
func (e *engine) estimate(n *tree.Node, text string, widthChanged bool) {
	e.next[n] = text
	if widthChanged {
		e.unstable = true
	}
}

// guess liefert den Text aus dem vorigen Durchlauf oder eine Schätzung.
func (e *engine) guess(n *tree.Node) string {
	if t, ok := e.prev[n]; ok {
		return t
	}
	return "1"
}

// Layout setzt das Dokument.
func Layout(root *tree.Node, opts Options) (*areatree.Document, error) {
	if opts.Fonts == nil {
		opts.Fonts = font.NewRegistry()
	}
	if opts.Breaker == nil {
		opts.Breaker = linebreak.KnuthPlass{}
	}
	if opts.Resolver == nil {
		opts.Resolver = denyResolver{}
	}
	if opts.Context == nil {
		opts.Context = context.Background()
	}
	if err := checkIDs(root); err != nil {
		return nil, err
	}
	prev := map[*tree.Node]string{}
	prevIDs := map[string]*idPages{}
	for pass := 1; ; pass++ {
		if err := opts.Context.Err(); err != nil {
			return nil, err
		}
		e := &engine{
			opts:       opts,
			doc:        &areatree.Document{},
			masters:    map[string]*master{},
			seqMasters: map[string]*seqMaster{},
			ids:        map[string]*idPages{},
			cited:      map[*tree.Node]citation{},
			marks:      map[*areatree.Page]*pageMarks{},
			fontIDs:    map[*font.Face]areatree.FontID{},
			imgIDs:     map[string]areatree.ImageID{},
			prev:       prev,
			prevIDs:    prevIDs,
			next:       map[*tree.Node]string{},
		}
		if err := e.run(root); err != nil {
			return nil, err
		}
		if err := e.checkCitations(); err != nil {
			return nil, err
		}
		if err := e.checkLinks(); err != nil {
			return nil, err
		}
		if !e.unstable {
			return e.doc, nil
		}
		if pass == maxPasses {
			return nil, tree.Errorf(root, "Seitenzahlen nach %d Durchläufen nicht stabil", maxPasses)
		}
		prev, prevIDs = e.next, e.ids
	}
}

// checkIDs prüft, dass jede ID nur einmal vorkommt.
func checkIDs(root *tree.Node) error {
	seen := map[string]*tree.Node{}
	var walk func(n *tree.Node) error
	walk = func(n *tree.Node) error {
		if n.Kind != tree.Element {
			return nil
		}
		if id, ok := n.Attr("id"); ok && id != "" && n.Space == tree.NSFO {
			if prev, dup := seen[id]; dup {
				return tree.Errorf(n, "id %q ist doppelt, zuerst bei %s", id, prev.Pos())
			}
			seen[id] = n
		}
		for _, c := range n.Children {
			if err := walk(c); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(root)
}

func (e *engine) run(root *tree.Node) error {
	rootProps, err := props.Compute(props.Root(), root, 0)
	if err != nil {
		return err
	}
	e.doc.Meta.Lang = rootProps.Lang
	sequences := 0
	for _, c := range root.Children {
		switch {
		case c.IsFO("layout-master-set"):
			if err := e.readMasters(c); err != nil {
				return err
			}
		case c.IsFO("bookmark-tree"):
			bms, err := e.bookmarks(c)
			if err != nil {
				return err
			}
			e.doc.Bookmarks = bms
		case c.IsFO("declarations"):
			if err := e.declarations(c); err != nil {
				return err
			}
		case c.IsFO("page-sequence"):
			if len(e.masters) == 0 {
				return tree.Errorf(c, "fo:layout-master-set fehlt vor fo:page-sequence")
			}
			sequences++
			if err := e.pageSequence(c, rootProps); err != nil {
				return err
			}
		}
	}
	if sequences == 0 {
		return tree.Errorf(root, "fo:root enthält keine fo:page-sequence")
	}
	return nil
}

// master ist eine einfache Seitenvorlage.
type master struct {
	node          *tree.Node
	width, height areatree.Length
	body          areatree.Rect
	bodyName      string
	bodyNode      *tree.Node
	regions       []*region // region-before und region-after
}

type region struct {
	node *tree.Node
	name string
	rect areatree.Rect
}

func (e *engine) readMasters(lms *tree.Node) error {
	for _, m := range lms.Children {
		if m.IsFO("page-sequence-master") {
			if err := e.readSequenceMaster(m); err != nil {
				return err
			}
			continue
		}
		if !m.IsFO("simple-page-master") {
			continue
		}
		name, ok := m.Attr("master-name")
		if !ok || name == "" {
			return tree.Errorf(m, "master-name fehlt")
		}
		if _, dup := e.masters[name]; dup || e.seqMasters[name] != nil {
			return tree.Errorf(m, "master-name %q ist doppelt", name)
		}
		w, err := props.LengthAttr(m, "page-width", areatree.Points(595.276), 12*areatree.Pt, -1)
		if err != nil {
			return err
		}
		h, err := props.LengthAttr(m, "page-height", areatree.Points(841.89), 12*areatree.Pt, -1)
		if err != nil {
			return err
		}
		if w <= 0 || h <= 0 || w > 14400*areatree.Pt || h > 14400*areatree.Pt {
			return tree.Errorf(m, "Seitengröße %sx%s pt ist ungültig", w, h)
		}
		margins, err := props.Margins(m, w)
		if err != nil {
			return err
		}
		content := areatree.Rect{X: margins[areatree.Left], Y: margins[areatree.Top],
			W: w - margins[areatree.Left] - margins[areatree.Right], H: h - margins[areatree.Top] - margins[areatree.Bottom]}
		mm := &master{node: m, width: w, height: h}
		for _, r := range m.Children {
			switch {
			case r.IsFO("region-body"):
				bm, err := props.Margins(r, content.W)
				if err != nil {
					return err
				}
				mm.body = areatree.Rect{X: content.X + bm[areatree.Left], Y: content.Y + bm[areatree.Top],
					W: content.W - bm[areatree.Left] - bm[areatree.Right], H: content.H - bm[areatree.Top] - bm[areatree.Bottom]}
				mm.bodyName = attrOr(r, "region-name", "xsl-region-body")
				mm.bodyNode = r
			case r.IsFO("region-before"), r.IsFO("region-after"):
				ext, err := props.LengthAttr(r, "extent", 0, 12*areatree.Pt, content.H)
				if err != nil {
					return err
				}
				reg := &region{node: r, name: attrOr(r, "region-name", "xsl-"+r.Name)}
				if r.IsFO("region-before") {
					reg.rect = areatree.Rect{X: content.X, Y: content.Y, W: content.W, H: ext}
				} else {
					reg.rect = areatree.Rect{X: content.X, Y: content.Y + content.H - ext, W: content.W, H: ext}
				}
				mm.regions = append(mm.regions, reg)
			}
		}
		if mm.bodyNode == nil {
			return tree.Errorf(m, "fo:region-body fehlt")
		}
		// Eine Region der Größe 0 ist zulässig, Inhalt passt dort nicht und
		// läuft auf die nächste Seite. Negative Größen sind ein Fehler.
		if mm.body.W < 0 || mm.body.H < 0 {
			return tree.Errorf(mm.bodyNode, "fo:region-body hat eine negative Größe, Ränder zu groß")
		}
		e.masters[name] = mm
	}
	return e.checkMasters()
}

func attrOr(n *tree.Node, name, def string) string {
	if v, ok := n.Attr(name); ok && v != "" {
		return v
	}
	return def
}

// fontID registriert eine Schrift im Dokument.
func (e *engine) fontID(f *font.Face) areatree.FontID {
	if id, ok := e.fontIDs[f]; ok {
		return id
	}
	id := areatree.FontID(len(e.doc.Fonts))
	e.doc.Fonts = append(e.doc.Fonts, areatree.FontRef{Source: f})
	e.fontIDs[f] = id
	return id
}

// imageID lädt ein Bild über den Resolver und registriert es einmal.
func (e *engine) imageID(n *tree.Node, src string) (areatree.ImageID, *image.Image, error) {
	data, err := e.opts.Resolver.Read(src)
	if err != nil {
		return 0, nil, tree.Errorf(n, "Bild %q: %v", src, err)
	}
	im, err := image.Load(data)
	if err != nil {
		return 0, nil, tree.Errorf(n, "Bild %q: %v", src, err)
	}
	if id, ok := e.imgIDs[im.Key()]; ok {
		return id, e.doc.Images[id].Source.(*image.Image), nil
	}
	id := areatree.ImageID(len(e.doc.Images))
	e.doc.Images = append(e.doc.Images, areatree.ImageRef{Source: im})
	e.imgIDs[im.Key()] = id
	return id, im, nil
}

func (e *engine) face(n *tree.Node, p *props.Computed) (*font.Face, error) {
	f, err := e.opts.Fonts.Lookup(p.FontFamily, p.FontWeight, p.Italic)
	if err != nil {
		return nil, tree.Errorf(n, "%v", err)
	}
	return f, nil
}

// bookmarks liest fo:bookmark-tree (XSL 1.1 6.11).
func (e *engine) bookmarks(n *tree.Node) ([]areatree.Bookmark, error) {
	var out []areatree.Bookmark
	for _, c := range n.Children {
		if !c.IsFO("bookmark") {
			continue
		}
		bm := areatree.Bookmark{Open: attrOr(c, "starting-state", "show") == "show"}
		if id, ok := c.Attr("internal-destination"); ok && id != "" {
			bm.Dest = id
			e.links = append(e.links, linkCheck{node: c, id: id})
		} else if u, ok := c.Attr("external-destination"); ok && u != "" {
			bm.URI = srcURI(u)
		} else {
			return nil, tree.Errorf(c, "fo:bookmark ohne internal-destination oder external-destination")
		}
		for _, t := range c.Children {
			if t.IsFO("bookmark-title") {
				var b strings.Builder
				for _, x := range t.Children {
					if x.Kind == tree.Text {
						b.WriteString(x.Data)
					}
				}
				bm.Title = strings.Join(strings.Fields(b.String()), " ")
			}
		}
		kids, err := e.bookmarks(c)
		if err != nil {
			return nil, err
		}
		bm.Children = kids
		out = append(out, bm)
	}
	return out, nil
}
