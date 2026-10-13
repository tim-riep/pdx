package layout

import (
	"errors"
	"strings"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/fo/linebreak"
	"github.com/tim-riep/pdx/fo/props"
	"github.com/tim-riep/pdx/fo/tree"
	"github.com/tim-riep/pdx/font"
)

type pieceKind uint8

const (
	pText pieceKind = iota
	pSpace
	pGraphic
	pBreak
	// pNoBreak verbietet den Umbruch am folgenden Leerraum (U+00A0).
	pNoBreak
)

// style ist ein Textstil mit aufgelöster Schrift.
type style struct {
	face     *font.Face
	fontID   areatree.FontID
	p        *props.Computed
	asc      areatree.Length
	desc     areatree.Length
	space    areatree.Glyph
	hasSpace bool
	nbsp     areatree.Glyph
	hasNbsp  bool
}

// above und below liefern die Ausdehnung über und unter der Grundlinie
// einschließlich halbem Durchschuss (half-leading).
func (s *style) above() areatree.Length {
	return s.asc + (s.p.LineHeight-s.asc-s.desc)/2
}

func (s *style) halfLeading() areatree.Length {
	return (s.p.LineHeight - s.asc - s.desc) / 2
}

func (s *style) below() areatree.Length {
	return s.desc + (s.p.LineHeight-s.asc-s.desc+1)/2
}

// piece ist ein Stück eines Absatzes: ein Wort, ein Leerraum, ein Bild oder
// ein erzwungener Umbruch.
type piece struct {
	kind   pieceKind
	st     *style
	glyphs []areatree.Glyph
	width  areatree.Length
	img    areatree.ImageID
	height areatree.Length
	// glyph ist bei Leerräumen die Glyphe (Leerzeichen oder U+00A0).
	glyph areatree.Glyph
	// deferred ist bei Seitenzahlen im Fluss gesetzt, deren Wert erst beim
	// Platzieren feststeht.
	deferred *tree.Node
	// ids sind IDs von Inline-Elementen, deren Inhalt mit diesem Stück beginnt.
	ids []string
}

// paragraph sammelt die Stücke eines Absatzes.
type paragraph struct {
	g      *galley
	p      *props.Computed
	node   *tree.Node
	pieces []piece
	styles map[styleKey]*style
}

type styleKey struct {
	p    *props.Computed
	face *font.Face
}

func newParagraph(g *galley, p *props.Computed, n *tree.Node) *paragraph {
	return &paragraph{g: g, p: p, node: n, styles: map[styleKey]*style{}}
}

func (pa *paragraph) empty() bool {
	for _, pc := range pa.pieces {
		if pc.kind != pSpace {
			return false
		}
	}
	return true
}

func (pa *paragraph) style(n *tree.Node, p *props.Computed, f *font.Face) (*style, error) {
	if f == nil {
		var err error
		if f, err = pa.g.e.face(n, p); err != nil {
			return nil, err
		}
	}
	k := styleKey{p, f}
	if s, ok := pa.styles[k]; ok {
		return s, nil
	}
	asc, desc, _ := f.Metrics(p.FontSize)
	s := &style{face: f, fontID: pa.g.e.fontID(f), p: p, asc: asc, desc: desc}
	if gs, err := f.Shape(" ", p.FontSize, font.ShapeOptions{}); err == nil && len(gs) == 1 {
		s.space, s.hasSpace = gs[0], true
	}
	if gs, err := f.Shape("\u00a0", p.FontSize, font.ShapeOptions{}); err == nil && len(gs) == 1 {
		s.nbsp, s.hasNbsp = gs[0], true
	}
	pa.styles[k] = s
	return s, nil
}

// add nimmt einen Text- oder Inlineknoten auf.
func (pa *paragraph) add(n *tree.Node, p *props.Computed) error {
	if n.Kind == tree.Text {
		return pa.text(n, p, n.Data, true)
	}
	if n.IsFO("marker") {
		return nil
	}
	if m, isRetrieve, err := pa.g.retrieve(n); isRetrieve {
		if err != nil || m == nil {
			return err
		}
		for _, ch := range m.Children {
			if err := pa.add(ch, p); err != nil {
				return err
			}
		}
		return nil
	}
	switch {
	case n.IsFO("inline"):
		ip, err := props.Compute(p, n, pa.g.contentW(pa.p))
		if err != nil {
			return err
		}
		before := len(pa.pieces)
		for _, ch := range n.Children {
			if err := pa.add(ch, ip); err != nil {
				return err
			}
		}
		pa.markID(n, before)
		return nil
	case n.IsFO("page-number-citation"), n.IsFO("page-number-citation-last"):
		ip, err := props.Compute(p, n, pa.g.contentW(pa.p))
		if err != nil {
			return err
		}
		ref, _ := n.Attr("ref-id")
		if ref == "" {
			return tree.Errorf(n, "ref-id fehlt")
		}
		text := pa.g.e.citationText(n, ref, n.IsFO("page-number-citation-last"))
		return pa.text(n, ip, text, false)
	case n.IsFO("page-number"):
		ip, err := props.Compute(p, n, pa.g.contentW(pa.p))
		if err != nil {
			return err
		}
		if pa.g.pageNo > 0 {
			return pa.text(n, ip, formatNumber(pa.g.pageNo, pa.g.e.format), false)
		}
		if err := pa.text(n, ip, pa.g.e.guess(n), false); err != nil {
			return err
		}
		pa.pieces[len(pa.pieces)-1].deferred = n
		return nil
	case n.IsFO("external-graphic"):
		return pa.graphic(n, p)
	}
	return tree.Errorf(n, "%s wird im Absatz nicht unterstützt", n.Label())
}

// markID hängt die ID eines Inline-Elements an das erste Stück mit Inhalt.
func (pa *paragraph) markID(n *tree.Node, from int) {
	id, ok := n.Attr("id")
	if !ok || id == "" {
		return
	}
	for i := from; i < len(pa.pieces); i++ {
		if k := pa.pieces[i].kind; k == pText || k == pGraphic {
			pa.pieces[i].ids = append(pa.pieces[i].ids, id)
			return
		}
	}
}

// text zerlegt Text in Wörter und Leerräume. Leerraum wird nach den
// Vorgaben von XSL (white-space-collapse) zusammengefasst, Zeilenumbrüche
// werden mit linefeed-treatment="preserve" zu erzwungenen Umbrüchen.
func (pa *paragraph) text(n *tree.Node, p *props.Computed, s string, collapse bool) error {
	st, err := pa.style(n, p, nil)
	if err != nil {
		return err
	}
	var word strings.Builder
	flushWord := func() error {
		if word.Len() == 0 {
			return nil
		}
		w := word.String()
		word.Reset()
		return pa.word(n, p, st, w)
	}
	for _, r := range s {
		switch {
		case r == '\n' && p.LinefeedPreserve && collapse:
			if err := flushWord(); err != nil {
				return err
			}
			pa.trimSpace()
			pa.pieces = append(pa.pieces, piece{kind: pBreak, st: st})
		case r == 0x00a0 && st.hasNbsp:
			// U+00A0 ist nach XSL 1.1 7.17.7 ein Wortzwischenraum und wird im
			// Blocksatz gedehnt, an ihm darf aber nicht umbrochen werden.
			if err := flushWord(); err != nil {
				return err
			}
			pa.pieces = append(pa.pieces, piece{kind: pNoBreak, st: st},
				piece{kind: pSpace, st: st, width: st.nbsp.Advance, glyph: st.nbsp})
		case collapse && (r == ' ' || r == '\t' || r == '\r' || r == '\n'):
			if err := flushWord(); err != nil {
				return err
			}
			pa.addSpace(n, st)
		default:
			word.WriteRune(r)
		}
	}
	return flushWord()
}

func (pa *paragraph) trimSpace() {
	for len(pa.pieces) > 0 && (pa.pieces[len(pa.pieces)-1].kind == pSpace || pa.pieces[len(pa.pieces)-1].kind == pNoBreak) {
		pa.pieces = pa.pieces[:len(pa.pieces)-1]
	}
}

func (pa *paragraph) addSpace(n *tree.Node, st *style) {
	if len(pa.pieces) == 0 {
		return
	}
	switch pa.pieces[len(pa.pieces)-1].kind {
	case pSpace, pBreak, pNoBreak:
		return
	}
	if !st.hasSpace {
		return
	}
	pa.pieces = append(pa.pieces, piece{kind: pSpace, st: st, width: st.space.Advance, glyph: st.space})
}

// word formt ein Wort. Fehlen der Schrift Zeichen, werden die eingebetteten
// Liberation-Schriften als Ersatz versucht.
func (pa *paragraph) word(n *tree.Node, p *props.Computed, st *style, w string) error {
	gs, err := st.face.Shape(w, p.FontSize, font.ShapeOptions{})
	var miss *font.MissingGlyphError
	if errors.As(err, &miss) {
		for _, fam := range []string{"sans-serif", "serif", "monospace"} {
			f, ferr := pa.g.e.opts.Fonts.Lookup(fam, p.FontWeight, p.Italic)
			if ferr != nil || f == st.face {
				continue
			}
			if g2, err2 := f.Shape(w, p.FontSize, font.ShapeOptions{}); err2 == nil {
				fst, serr := pa.style(n, p, f)
				if serr != nil {
					return serr
				}
				gs, st, err = g2, fst, nil
				break
			}
		}
	}
	if err != nil {
		return tree.Errorf(n, "%v", err)
	}
	var width areatree.Length
	for _, g := range gs {
		width += g.Advance
	}
	pa.pieces = append(pa.pieces, piece{kind: pText, st: st, glyphs: gs, width: width})
	return nil
}

// items übersetzt die Stücke in das Box-Glue-Penalty-Modell.
func (pa *paragraph) items(from int) []linebreak.Item {
	items := make([]linebreak.Item, 0, len(pa.pieces))
	for i := from; i < len(pa.pieces); i++ {
		pc := pa.pieces[i]
		switch pc.kind {
		case pText, pGraphic:
			items = append(items, linebreak.Item{Kind: linebreak.Box, Width: pc.width, Payload: i})
		case pSpace:
			items = append(items, linebreak.Item{Kind: linebreak.Glue, Width: pc.width,
				Stretch: pc.width / 2, Shrink: pc.width / 3, Payload: i})
		case pBreak:
			items = append(items, linebreak.Item{Kind: linebreak.Penalty, Penalty: linebreak.ForcedBreak, Payload: i})
		case pNoBreak:
			items = append(items, linebreak.Item{Kind: linebreak.Penalty, Penalty: linebreak.Forbidden, Payload: i})
		}
	}
	return items
}

// setParagraph bricht den Absatz um und setzt die Zeilen.
func (g *galley) setParagraph(pa *paragraph, first bool) error {
	pa.trimSpace()
	p := pa.p
	avail := g.contentW(p)
	x0 := g.contentX(p)
	indent := areatree.Length(0)
	if first {
		indent = p.TextIndent
	}
	ord := g.next()
	if ord.major < g.skip.major {
		return nil
	}
	from := 0
	if ord.major == g.skip.major {
		// Neuaufbau nach Breitenwechsel: der Absatz wird ab dem ersten Stück
		// der neuen Seite umbrochen, ohne Einzug der ersten Zeile.
		from, indent = g.skip.minor, 0
	}
	items := pa.items(from)
	breaks, err := g.e.opts.Breaker.Break(items, func(line int) areatree.Length {
		if line == 0 {
			return avail - indent
		}
		return avail
	})
	if err != nil {
		return tree.Errorf(pa.node, "%v", err)
	}
	strut, err := pa.style(pa.node, p, nil)
	if err != nil {
		return err
	}
	start := 0
	for li, br := range breaks {
		end := br.Index
		// Glue und Penalties am Zeilenanfang und Glue am Zeilenende entfallen.
		s := start
		for s < end && items[s].Kind != linebreak.Box {
			s++
		}
		e := end
		for e > s && items[e-1].Kind != linebreak.Box {
			e--
		}
		last := end >= len(items) || (items[end].Kind == linebreak.Penalty && items[end].Penalty <= linebreak.ForcedBreak)
		lineW := avail
		lx := x0
		if li == 0 {
			lineW -= indent
			lx += indent
		}
		lo := ord
		if s < len(items) {
			lo.minor = items[s].Payload
		}
		// Witwen und Waisen (XSL 1.1 7.20.6 und 7.20.4): Vor Zeile li darf
		// nur umbrochen werden, wenn davor mindestens orphans und danach
		// mindestens widows Zeilen stehen.
		pen := 0
		if li > 0 && (li < p.Orphans || len(breaks)-li < p.Widows) {
			pen = penaltyInf
		}
		if err := g.setLine(pa, items[s:e], strut, lx, lineW, br.Ratio, last, lo, pen); err != nil {
			return err
		}
		start = end + 1
	}
	return nil
}

func (g *galley) setLine(pa *paragraph, seg []linebreak.Item, strut *style, x0, avail areatree.Length, ratio float64, last bool, ord ordinal, pen int) error {
	p := pa.p
	align := p.TextAlign
	if last {
		switch {
		case p.TextAlignLast != "relative":
			align = p.TextAlignLast
		case align == "justify":
			align = "start"
		}
	}
	var natural, stretch, shrink areatree.Length
	for _, it := range seg {
		natural += it.Width
		stretch += it.Stretch
		shrink += it.Shrink
	}
	if align == "justify" && last && stretch > 0 && natural < avail {
		ratio = float64(avail-natural) / float64(stretch)
	}
	if align != "justify" {
		ratio = 0
	}
	adjust := func(it linebreak.Item) areatree.Length {
		switch {
		case ratio > 0:
			return it.Width + areatree.Length(ratio*float64(it.Stretch))
		case ratio < 0:
			return it.Width + areatree.Length(ratio*float64(it.Shrink))
		}
		return it.Width
	}
	var width areatree.Length
	for _, it := range seg {
		width += adjust(it)
	}
	x := x0
	switch align {
	case "center":
		x += (avail - width) / 2
	case "end":
		x += avail - width
	}

	// Vertikale Maße.
	above, below := strut.above(), strut.below()
	for _, it := range seg {
		pc := pa.pieces[it.Payload]
		switch pc.kind {
		case pText, pSpace:
			above, below = max(above, pc.st.above()), max(below, pc.st.below())
		case pGraphic:
			// Wie bei Apache FOP erhält auch ein Bild den halben Durchschuss
			// der Zeile über sich (Strut des Blocks).
			above = max(above, pc.height+strut.halfLeading())
		}
	}
	h := above + below
	baseline := above
	line := &lineBox{h: h, w: avail, x: x0}

	var run *areatree.GlyphRun
	var runStyle *style
	for _, it := range seg {
		pc := pa.pieces[it.Payload]
		switch pc.kind {
		case pText, pSpace:
			if run == nil || runStyle != pc.st || pc.deferred != nil {
				run = &areatree.GlyphRun{Origin: areatree.Point{X: x, Y: baseline}, Font: pc.st.fontID,
					Size: pc.st.p.FontSize, Color: pc.st.p.Color, Ascent: pc.st.asc, Descent: pc.st.desc}
				runStyle = pc.st
				line.areas = append(line.areas, run)
				if pc.deferred != nil {
					line.deferred = append(line.deferred, deferredRun{index: len(line.areas) - 1, node: pc.deferred, st: pc.st, reserved: pc.width})
				}
			}
			if pc.kind == pText {
				run.Glyphs = append(run.Glyphs, pc.glyphs...)
			} else {
				g := pc.glyph
				g.Advance = adjust(it)
				run.Glyphs = append(run.Glyphs, g)
			}
			x += adjust(it)
			if pc.deferred != nil {
				run = nil // nach der Seitenzahl beginnt ein neuer Run
			}
		case pGraphic:
			run = nil
			line.areas = append(line.areas, &areatree.Image{
				Rect:  areatree.Rect{X: x, Y: baseline - pc.height, W: pc.width, H: pc.height},
				Image: pc.img,
			})
			x += pc.width
		}
	}
	for _, it := range seg {
		line.ids = append(line.ids, pa.pieces[it.Payload].ids...)
	}
	g.addLine(line, ord, pen)
	return nil
}
