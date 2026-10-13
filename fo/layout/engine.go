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
	"strconv"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/fo/linebreak"
	"github.com/tim-riep/pdx/fo/props"
	"github.com/tim-riep/pdx/fo/tree"
	"github.com/tim-riep/pdx/font"
	"github.com/tim-riep/pdx/image"
)

// Options steuert das Layout.
type Options struct {
	// Fonts löst Schriftfamilien auf. nil bedeutet font.NewRegistry().
	Fonts *font.Registry
	// Resolver lädt Bilder und Anhänge. nil verweigert jeden Zugriff.
	Resolver Resolver
	// Breaker bricht Absätze um. nil bedeutet linebreak.Greedy.
	Breaker linebreak.Breaker
}

type engine struct {
	opts    Options
	doc     *areatree.Document
	masters map[string]*master
	fontIDs map[*font.Face]areatree.FontID
	imgIDs  map[string]areatree.ImageID
	pageNo  int

	// prev enthält die Texte seitenabhängiger Inhalte aus dem vorigen
	// Durchlauf, next die dieses Durchlaufs. unstable ist gesetzt, wenn sich
	// eine reservierte Breite als falsch erwiesen hat.
	prev, next map[*tree.Node]string
	unstable   bool
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
		opts.Breaker = linebreak.Greedy{}
	}
	if opts.Resolver == nil {
		opts.Resolver = denyResolver{}
	}
	prev := map[*tree.Node]string{}
	for pass := 1; ; pass++ {
		e := &engine{
			opts:    opts,
			doc:     &areatree.Document{},
			masters: map[string]*master{},
			fontIDs: map[*font.Face]areatree.FontID{},
			imgIDs:  map[string]areatree.ImageID{},
			prev:    prev,
			next:    map[*tree.Node]string{},
		}
		if err := e.run(root); err != nil {
			return nil, err
		}
		if !e.unstable {
			return e.doc, nil
		}
		if pass == maxPasses {
			return nil, tree.Errorf(root, "Seitenzahlen nach %d Durchläufen nicht stabil", maxPasses)
		}
		prev = e.next
	}
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
		if !m.IsFO("simple-page-master") {
			continue
		}
		name, ok := m.Attr("master-name")
		if !ok || name == "" {
			return tree.Errorf(m, "master-name fehlt")
		}
		if _, dup := e.masters[name]; dup {
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
		if mm.body.W <= 0 || mm.body.H <= 0 {
			return tree.Errorf(mm.bodyNode, "fo:region-body hat keine Fläche, Ränder zu groß")
		}
		e.masters[name] = mm
	}
	return nil
}

func attrOr(n *tree.Node, name, def string) string {
	if v, ok := n.Attr(name); ok && v != "" {
		return v
	}
	return def
}

// sequence ist der Zustand einer fo:page-sequence.
type sequence struct {
	e       *engine
	m       *master
	props   *props.Computed
	statics map[string]*tree.Node
	page    *areatree.Page
}

func (e *engine) pageSequence(ps *tree.Node, rootProps *props.Computed) error {
	ref, _ := ps.Attr("master-reference")
	m, ok := e.masters[ref]
	if !ok {
		return tree.Errorf(ps, "Seitenvorlage %q nicht gefunden (fo:page-sequence-master folgt in Phase 1)", ref)
	}
	if v, ok := ps.Attr("initial-page-number"); ok && v != "auto" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return tree.Errorf(ps, "initial-page-number %q wird nicht unterstützt", v)
		}
		e.pageNo = n - 1
	}
	p, err := props.Compute(rootProps, ps, m.body.W)
	if err != nil {
		return err
	}
	s := &sequence{e: e, m: m, props: p, statics: map[string]*tree.Node{}}
	var flow *tree.Node
	for _, c := range ps.Children {
		switch {
		case c.IsFO("static-content"):
			name, _ := c.Attr("flow-name")
			if _, dup := s.statics[name]; dup {
				return tree.Errorf(c, "flow-name %q ist doppelt", name)
			}
			s.statics[name] = c
		case c.IsFO("flow"):
			if flow != nil {
				return tree.Errorf(c, "mehr als ein fo:flow")
			}
			flow = c
		}
	}
	if flow == nil {
		return tree.Errorf(ps, "fo:flow fehlt")
	}
	if name, _ := flow.Attr("flow-name"); name != m.bodyName {
		return tree.Errorf(flow, "flow-name %q passt nicht zur Region %q", name, m.bodyName)
	}
	flowProps, err := props.Compute(p, flow, m.body.W)
	if err != nil {
		return err
	}
	g := newGalley(e, m.body.W, 0)
	if err := g.blocks(flow, flowProps); err != nil {
		return err
	}
	return s.paginate(g.items)
}

// paginate verteilt die Galley auf Seiten.
func (s *sequence) paginate(items []item) error {
	pos := 0
	for {
		region := s.newPage()
		end := breakPage(items, pos, s.m.body.H)
		pl := newPlacer(s.e, s.page, s.m.body, region)
		if err := pl.place(items[pos:end]); err != nil {
			return err
		}
		if err := s.finishPage(); err != nil {
			return err
		}
		if end >= len(items) {
			return nil
		}
		pos = end + 1
	}
}

// newPage beginnt eine neue Seite und liefert den Container der Region body.
func (s *sequence) newPage() *areatree.Container {
	s.e.pageNo++
	s.page = &areatree.Page{Number: s.e.pageNo, Width: s.m.width, Height: s.m.height}
	s.e.doc.Pages = append(s.e.doc.Pages, s.page)
	region := &areatree.Container{Kind: areatree.KindRegion, Rect: s.m.body, Source: s.m.bodyNode.Label()}
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
		if err := g.blocks(sc, p); err != nil {
			return err
		}
		if err := newPlacer(s.e, s.page, r.rect, cont).place(g.items); err != nil {
			return err
		}
	}
	return nil
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
