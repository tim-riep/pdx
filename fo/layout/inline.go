package layout

import (
	"unicode"

	"sort"

	"errors"
	"strings"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/fo/linebreak"
	"github.com/tim-riep/pdx/fo/props"
	"github.com/tim-riep/pdx/fo/tree"
	"github.com/tim-riep/pdx/font"
	"github.com/tim-riep/pdx/hyph"
)

type pieceKind uint8

const (
	pText pieceKind = iota
	pSpace
	pGraphic
	pBreak
	// pNoBreak verbietet den Umbruch am folgenden Leerraum (U+00A0).
	pNoBreak
	// pHyphen ist eine Trennstelle in einem Wort.
	pHyphen
	// pBreakAfter ist eine Umbruchstelle nach einem vorhandenen Bindestrich.
	// Beim Umbruch wird kein Zeichen eingefügt.
	pBreakAfter
	// pEdge ist Rahmen plus Padding am Anfang oder Ende eines Inline-Elements.
	pEdge
	// pLeader ist ein fo:leader, ein dehnbarer Glue ohne Umbruchstelle.
	pLeader
)

// leaderSpec beschreibt das Muster eines fo:leader.
type leaderSpec struct {
	pattern   string // space, rule, dots
	thickness areatree.Length
	style     areatree.BorderStyle
	patternW  areatree.Length // 0 bei use-font-metrics
	min, max  areatree.Length
}

// linkRef ist das Ziel eines fo:basic-link.
type linkRef struct {
	dest, uri string
}

// hyphenPenalty ist der Strafwert einer Trennstelle wie bei Apache FOP
// (KnuthPenalty.FLAGGED_PENALTY).
const hyphenPenalty = 50

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
	hyphen   *areatree.Glyph // Trennstrich, nil solange nicht gebraucht
}

// hyphenGlyph liefert den Trennstrich des Stils. hyphenation-character ist
// nach XSL 1.1 7.10.3 anfangs U+2010, fehlt die Glyphe, gilt U+002D.
func (s *style) hyphenGlyph() (areatree.Glyph, bool) {
	if s.hyphen != nil {
		return *s.hyphen, true
	}
	for _, h := range []string{s.p.HyphenChar, "-"} {
		if gs, err := s.face.Shape(h, s.p.FontSize, font.ShapeOptions{}); err == nil && len(gs) == 1 {
			g := gs[0]
			s.hyphen = &g
			return g, true
		}
	}
	return areatree.Glyph{}, false
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
	// link ist bei Inhalt eines fo:basic-link gesetzt.
	link *linkRef
	// leader beschreibt einen fo:leader.
	leader *leaderSpec
	// code ist bei Barcodes gesetzt (pGraphic ohne Bild).
	code *codeArt
	// decos sind die Inline-Elemente mit Hintergrund, Rahmen oder Padding,
	// in denen das Stück liegt, von außen nach innen. Bei pEdge ist decos[0]
	// das Element der Kante und start gibt an, ob es die Anfangskante ist.
	decos []*inlineDeco
	start bool
	// box ist bei Bildern mit Rahmen, Padding oder Hintergrund gesetzt.
	box *graphicBox
	// nbsp ist bei U+00A0 gesetzt. Es ist kein XML-Leerraum und wird weder
	// am Absatzrand entfernt noch macht es einen Absatz leer (7.16.12).
	nbsp bool
}

// inlineDeco ist ein Inline-Element mit Hintergrund, Rahmen oder Padding.
type inlineDeco struct {
	p  *props.Computed
	st *style
	n  *tree.Node
}

// hasDeco meldet Hintergrund, sichtbaren Rahmen oder Padding.
func hasDeco(p *props.Computed) bool {
	if p.Background != nil {
		return true
	}
	for i := 0; i < 4; i++ {
		if p.Padding[i] > 0 || p.Border[i].Visible() {
			return true
		}
	}
	return false
}

// edgeWidth liefert Rahmen plus Padding einer Seite.
func edgeWidth(p *props.Computed, side int) areatree.Length {
	w := p.Padding[side]
	if p.Border[side].Visible() {
		w += p.Border[side].Width
	}
	return w
}

// decorate umgibt die Stücke ab before mit den Kanten eines Inline-Elements.
// Bei einem Umbruch innerhalb des Elements entfallen Rahmen und Padding an
// der Umbruchstelle (Konditionalität discard, XSL 1.1 7.8.31 bis 7.8.34).
func (pa *paragraph) decorate(n *tree.Node, ip *props.Computed, before int) error {
	if !hasDeco(ip) {
		return nil
	}
	st, err := pa.style(n, ip, nil)
	if err != nil {
		return err
	}
	d := &inlineDeco{p: ip, st: st, n: n}
	for i := before; i < len(pa.pieces); i++ {
		pa.pieces[i].decos = append([]*inlineDeco{d}, pa.pieces[i].decos...)
	}
	startEdge := piece{kind: pEdge, st: st, width: edgeWidth(ip, areatree.Left), decos: []*inlineDeco{d}, start: true}
	endEdge := piece{kind: pEdge, st: st, width: edgeWidth(ip, areatree.Right), decos: []*inlineDeco{d}}
	if len(pa.pieces) > before {
		startEdge.decos = append(startEdge.decos, pa.pieces[before].decos[1:]...)
		endEdge.decos = append(endEdge.decos, pa.pieces[len(pa.pieces)-1].decos[1:]...)
	}
	rest := append([]piece{startEdge}, pa.pieces[before:]...)
	pa.pieces = append(append(pa.pieces[:before], rest...), endEdge)
	return nil
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
		if pc.kind != pSpace || pc.nbsp {
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
		return pa.decorate(n, ip, before)
	case n.IsFO("basic-link"):
		ip, err := props.Compute(p, n, pa.g.contentW(pa.p))
		if err != nil {
			return err
		}
		ref := &linkRef{}
		if id, ok := n.Attr("internal-destination"); ok && id != "" {
			ref.dest = id
			pa.g.e.links = append(pa.g.e.links, linkCheck{node: n, id: id})
		} else if u, ok := n.Attr("external-destination"); ok && srcURI(u) != "" {
			ref.uri = srcURI(u)
		} else {
			return tree.Errorf(n, "fo:basic-link ohne internal-destination oder external-destination")
		}
		before := len(pa.pieces)
		for _, ch := range n.Children {
			if err := pa.add(ch, ip); err != nil {
				return err
			}
		}
		for i := before; i < len(pa.pieces); i++ {
			if pa.pieces[i].link == nil {
				pa.pieces[i].link = ref
			}
		}
		pa.markID(n, before)
		return pa.decorate(n, ip, before)
	case n.IsFO("leader"):
		return pa.leader(n, p)
	case n.IsFO("instream-foreign-object"):
		return pa.foreign(n, p)
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

// leader nimmt ein fo:leader auf (XSL 1.1 6.6.9). leader-length gibt
// Mindest-, Wunsch- und Höchstlänge an (anfangs 0, 12pt und 100 % der
// Zeile). Gedehnt wird über den Blocksatz der Zeile.
func (pa *paragraph) leader(n *tree.Node, parent *props.Computed) error {
	avail := pa.g.contentW(pa.p)
	p, err := props.Compute(parent, n, avail)
	if err != nil {
		return err
	}
	st, err := pa.style(n, p, nil)
	if err != nil {
		return err
	}
	spec := &leaderSpec{pattern: attrOr(n, "leader-pattern", "space"), thickness: areatree.Pt, style: areatree.Solid,
		min: 0, max: avail}
	opt := 12 * areatree.Pt
	length := func(name string, def areatree.Length) (areatree.Length, error) {
		return props.LengthAttr(n, name, def, p.FontSize, avail)
	}
	if v, ok := n.Attr("leader-length"); ok {
		l, err := props.EvalLength(v, p.FontSize, avail, nil)
		if err != nil {
			return tree.Errorf(n, "leader-length: %v", err)
		}
		spec.min, opt, spec.max = l, l, l
	}
	if spec.min, err = length("leader-length.minimum", spec.min); err != nil {
		return err
	}
	if opt, err = length("leader-length.optimum", opt); err != nil {
		return err
	}
	if spec.max, err = length("leader-length.maximum", spec.max); err != nil {
		return err
	}
	if spec.thickness, err = length("rule-thickness", spec.thickness); err != nil {
		return err
	}
	if v, _ := n.Attr("leader-pattern-width"); v != "" && v != "use-font-metrics" {
		if spec.patternW, err = length("leader-pattern-width", 0); err != nil {
			return err
		}
	}
	switch v := attrOr(n, "rule-style", "solid"); v {
	case "dashed":
		spec.style = areatree.Dashed
	case "dotted":
		spec.style = areatree.Dotted
	case "double":
		spec.style = areatree.Double
	}
	switch spec.pattern {
	case "space", "rule", "dots":
	case "use-content":
		spec.pattern = "space" // Inhalt als Muster folgt später, Leerraum als Rückfall
	default:
		return tree.Errorf(n, "leader-pattern %q ist ungültig", spec.pattern)
	}
	opt = min(max(opt, spec.min), spec.max)
	// Die Box der Breite 0 hält den Leader am Zeilenanfang fest: Er ist ein
	// Inline-Bereich (XSL 1.1 6.6.9), kein Leerraum, der dort entfällt.
	pa.pieces = append(pa.pieces, piece{kind: pEdge, st: st}, piece{kind: pNoBreak, st: st},
		piece{kind: pLeader, st: st, width: opt, leader: spec})
	return nil
}

// markID hängt die ID eines Inline-Elements an das erste und das letzte
// Stück mit Inhalt. So kennt page-number-citation-last auch die letzte Seite
// eines umbrochenen Inline-Elements.
func (pa *paragraph) markID(n *tree.Node, from int) {
	id, ok := n.Attr("id")
	if !ok || id == "" {
		return
	}
	content := func(i int) bool {
		k := pa.pieces[i].kind
		return k == pText || k == pGraphic
	}
	for i := from; i < len(pa.pieces); i++ {
		if content(i) {
			pa.pieces[i].ids = append(pa.pieces[i].ids, id)
			for j := len(pa.pieces) - 1; j > i; j-- {
				if content(j) {
					pa.pieces[j].ids = append(pa.pieces[j].ids, id)
					break
				}
			}
			return
		}
	}
	// Ein leeres Inline-Element hat kein Stück. Seine ID gilt dann für den
	// umschließenden Block.
	if st := pa.g.stack; len(st) > 0 {
		st[len(st)-1].ids = append(st[len(st)-1].ids, id)
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
			// Am Absatzanfang hält eine Box der Breite 0 das Zeichen fest.
			if len(pa.pieces) == 0 {
				pa.pieces = append(pa.pieces, piece{kind: pEdge, st: st})
			}
			pa.pieces = append(pa.pieces, piece{kind: pNoBreak, st: st},
				piece{kind: pSpace, st: st, width: st.nbsp.Advance, glyph: st.nbsp, nbsp: true})
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
	for len(pa.pieces) > 0 {
		last := pa.pieces[len(pa.pieces)-1]
		if last.kind == pSpace && !last.nbsp || last.kind == pNoBreak {
			pa.pieces = pa.pieces[:len(pa.pieces)-1]
			continue
		}
		break
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
	// Umbruchstellen: nach vorhandenen Bindestrichen immer, Trennstellen
	// aus den Mustern nur mit hyphenate="true".
	explicit := explicitBreaks(w)
	cuts := append([]int(nil), explicit...)
	if p.Hyphenate && p.Lang != "" {
		if pats, ok, err := hyph.ForLanguage(p.Lang); err != nil {
			return tree.Errorf(n, "%v", err)
		} else if ok {
			if _, hasHyphen := st.hyphenGlyph(); hasHyphen {
				for _, c := range pats.Hyphenate(w, p.HyphenRemain, p.HyphenPush) {
					if !nearBreak(c, explicit) {
						cuts = append(cuts, c)
					}
				}
			}
		}
	}
	sort.Ints(cuts)
	frags, at := splitGlyphsAt(gs, cuts)
	for i, f := range frags {
		if i > 0 {
			if containsInt(explicit, at[i-1]) {
				pa.pieces = append(pa.pieces, piece{kind: pBreakAfter, st: st})
			} else {
				hg, _ := st.hyphenGlyph()
				pa.pieces = append(pa.pieces, piece{kind: pHyphen, st: st, width: hg.Advance, glyph: hg})
			}
		}
		var width areatree.Length
		for _, g := range f {
			width += g.Advance
		}
		pa.pieces = append(pa.pieces, piece{kind: pText, st: st, glyphs: f, width: width})
	}
	return nil
}

// splitGlyphs teilt die Glyphen eines Wortes an Trennstellen (Zeichen vor
// der Stelle). Stellen innerhalb eines Clusters, etwa einer Ligatur, entfallen.
// explicitBreaks liefert die Zeichenpositionen nach Bindestrichen, an denen
// umbrochen werden darf: nach U+002D, U+2010 und U+2013 zwischen einem
// Buchstaben und einem Zeichen, das keine Ziffer ist, mit mindestens zwei
// Zeichen davor und danach (wie die Klassen HY und
// BA in UAX #14). XSL 1.1 legt den Umbruchalgorithmus nicht fest (1.1.2).
func explicitBreaks(w string) []int {
	rs := []rune(w)
	var out []int
	// Wie bei hyphenation-remain- und -push-character-count bleiben
	// mindestens zwei Zeichen auf jeder Seite.
	for i := 2; i+2 < len(rs); i++ {
		switch rs[i] {
		case '-', '\u2010', '\u2013':
			if unicode.IsLetter(rs[i-1]) && !unicode.IsDigit(rs[i+1]) && !strings.ContainsRune("-\u2010\u2013", rs[i+1]) {
				out = append(out, i+1)
			}
		}
	}
	return out
}

// nearBreak meldet eine Trennstelle direkt an einem Bindestrich.
func nearBreak(c int, explicit []int) bool {
	for _, e := range explicit {
		if c >= e-1 && c <= e+1 {
			return true
		}
	}
	return false
}

func containsInt(v []int, x int) bool {
	for _, y := range v {
		if y == x {
			return true
		}
	}
	return false
}

// splitGlyphsAt teilt wie splitGlyphs und liefert zu jeder Grenze die
// Zeichenposition, an der geteilt wurde.
func splitGlyphsAt(gs []areatree.Glyph, cuts []int) ([][]areatree.Glyph, []int) {
	starts := map[int]int{}
	pos := 0
	for i, g := range gs {
		if g.Text != "" {
			starts[pos] = i
			pos += len([]rune(g.Text))
		}
	}
	var out [][]areatree.Glyph
	var at []int
	last := 0
	for _, c := range cuts {
		if i, ok := starts[c]; ok && i > last {
			out = append(out, gs[last:i])
			at = append(at, c)
			last = i
		}
	}
	return append(out, gs[last:]), at
}

func splitGlyphs(gs []areatree.Glyph, cuts []int) [][]areatree.Glyph {
	if len(cuts) == 0 {
		return [][]areatree.Glyph{gs}
	}
	starts := map[int]int{} // Zeichenposition zu Glyphenindex am Clusteranfang
	pos := 0
	for i, g := range gs {
		if g.Text != "" {
			starts[pos] = i
			pos += len([]rune(g.Text))
		}
	}
	var out [][]areatree.Glyph
	last := 0
	for _, c := range cuts {
		if i, ok := starts[c]; ok && i > last {
			out = append(out, gs[last:i])
			last = i
		}
	}
	return append(out, gs[last:])
}

// items übersetzt die Stücke in das Box-Glue-Penalty-Modell. Im Blocksatz
// darf ein Wortzwischenraum um die Hälfte wachsen und um ein Drittel
// schrumpfen. Im Flattersatz erhält wie bei Apache FOP nur die Zeile, die an
// einem Zwischenraum endet, die Dehnbarkeit RaggedStretch:
// Glue(0, +R), Penalty(0), Glue(w, −R). Hilfselemente tragen Payload −1.
func (pa *paragraph) items(from int, justify bool) []linebreak.Item {
	items := make([]linebreak.Item, 0, len(pa.pieces))
	R := linebreak.RaggedStretch
	for i := from; i < len(pa.pieces); i++ {
		pc := pa.pieces[i]
		switch pc.kind {
		case pText, pGraphic, pEdge:
			items = append(items, linebreak.Item{Kind: linebreak.Box, Width: pc.width, Payload: i})
		case pSpace:
			nobreak := i > 0 && pa.pieces[i-1].kind == pNoBreak
			switch {
			case justify:
				items = append(items, linebreak.Item{Kind: linebreak.Glue, Width: pc.width,
					Stretch: pc.width / 2, Shrink: pc.width / 3, Payload: i})
			case nobreak:
				items = append(items, linebreak.Item{Kind: linebreak.Glue, Width: pc.width, Payload: i})
			default:
				items = append(items,
					linebreak.Item{Kind: linebreak.Glue, Stretch: R, Payload: -1},
					linebreak.Item{Kind: linebreak.Penalty, Payload: -1},
					linebreak.Item{Kind: linebreak.Glue, Width: pc.width, Stretch: -R, Payload: i})
			}
		case pBreak:
			items = append(items, linebreak.Item{Kind: linebreak.Penalty, Penalty: linebreak.ForcedBreak, Payload: i})
		case pNoBreak:
			items = append(items, linebreak.Item{Kind: linebreak.Penalty, Penalty: linebreak.Forbidden, Payload: i})
		case pHyphen, pBreakAfter:
			// Trennstelle mit Trennstrich (markiert, Strafwert 50) oder
			// Umbruchstelle nach einem vorhandenen Bindestrich (ohne Strafwert,
			// schon im ersten Durchgang). Im Flattersatz braucht die Zeile, die
			// hier endet, dieselbe Dehnung wie an einem Wortzwischenraum.
			pen := linebreak.Item{Kind: linebreak.Penalty, Payload: i}
			if pc.kind == pHyphen {
				pen.Penalty, pen.Flagged, pen.Width = hyphenPenalty, true, pc.width
			}
			if justify {
				items = append(items, pen)
			} else {
				items = append(items,
					linebreak.Item{Kind: linebreak.Glue, Stretch: R, Payload: -1},
					pen,
					linebreak.Item{Kind: linebreak.Glue, Stretch: -R, Payload: -1})
			}
		case pLeader:
			l := pc.leader
			items = append(items, linebreak.Item{Kind: linebreak.Glue, Width: pc.width,
				Stretch: max(l.max-pc.width, 0), Shrink: max(pc.width-l.min, 0), Payload: i})
		}
	}
	return items
}

// setParagraph bricht den Absatz um und setzt die Zeilen.
func (g *galley) setParagraph(pa *paragraph, first bool) error {
	pa.trimSpace()
	// Ein erhaltener Zeilenwechsel am Absatzende beendet die letzte Zeile,
	// er eröffnet keine leere Zeile mehr.
	if n := len(pa.pieces); n > 1 && pa.pieces[n-1].kind == pBreak {
		pa.pieces = pa.pieces[:n-1]
		pa.trimSpace()
	}
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
	items := pa.items(from, p.TextAlign == "justify")
	// last-line-end-indent verkürzt die letzte Zeile, negative Werte
	// verlängern sie (XSL 1.1 7.16.3). Die unsichtbare Box am Absatzende
	// wirkt im Umbruch und in der Ausrichtung.
	if l := p.LastLineEndIndent; l != 0 && len(items) > 0 {
		items = append(items, linebreak.Item{Kind: linebreak.Box, Width: l, Payload: -1})
	}
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
		var hy *piece
		if end < len(items) && items[end].Kind == linebreak.Penalty && items[end].Flagged && items[end].Payload >= 0 {
			hy = &pa.pieces[items[end].Payload]
		}
		if err := g.setLine(pa, items[s:e], strut, lx, lineW, br.Ratio, last, lo, pen, hy); err != nil {
			return err
		}
		start = end + 1
	}
	return nil
}

func (g *galley) setLine(pa *paragraph, seg []linebreak.Item, strut *style, x0, avail areatree.Length, ratio float64, last bool, ord ordinal, pen int, hy *piece) error {
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
	// Für die Ausrichtung zählen nur echte Dehnungen. Die Hilfsdehnung des
	// Flattersatzes (+R am Hilfselement, −R am Leerraum) dient nur der
	// Umbruchwahl und gleicht sich in der Zeile aus.
	real := func(it linebreak.Item) bool { return it.Payload >= 0 && it.Stretch >= 0 }
	var natural, stretch, shrink areatree.Length
	for _, it := range seg {
		natural += it.Width
		if real(it) {
			stretch += it.Stretch
			shrink += it.Shrink
		}
	}
	if align == "justify" && last && stretch > 0 && natural < avail {
		ratio = float64(avail-natural) / float64(stretch)
	}
	if align != "justify" {
		ratio = 0
	}
	adjust := func(it linebreak.Item) areatree.Length {
		if !real(it) {
			return it.Width
		}
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
	if hy != nil {
		width += hy.width
	}
	x := x0
	switch align {
	case "center":
		x += (avail - width) / 2
	case "end":
		x += avail - width
	}

	// Vertikale Maße nach line-stacking-strategy="max-height" (XSL 1.1 4.5):
	// Die Zeile umschließt das Rechteck des Blocks und die Zuordnungsrechtecke
	// aller Inline-Bereiche, davor und danach steht der halbe Durchschuss des
	// Blocks.
	above, below := strut.asc, strut.desc
	for _, it := range seg {
		if it.Payload < 0 {
			continue
		}
		pc := pa.pieces[it.Payload]
		switch pc.kind {
		case pText, pSpace:
			above, below = max(above, pc.st.asc), max(below, pc.st.desc)
		case pGraphic:
			above = max(above, pc.height)
		}
	}
	above += strut.above() - strut.asc
	below += strut.below() - strut.desc
	h := above + below
	baseline := above
	line := &lineBox{h: h, w: avail, x: x0}

	var run *areatree.GlyphRun
	var runStyle *style
	frags := &decoFrags{}
	for _, it := range seg {
		if it.Payload < 0 {
			continue
		}
		pc := pa.pieces[it.Payload]
		frags.add(pc, x, x+adjust(it))
		switch pc.kind {
		case pEdge:
			run = nil
			x += adjust(it)
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
		case pLeader:
			run = nil
			w := adjust(it)
			line.areas = append(line.areas, drawLeader(pc, x, w, baseline)...)
			x += w
		case pGraphic:
			run = nil
			if pc.code != nil {
				line.areas = append(line.areas, pc.code.path(x, baseline-pc.height, pc.width, pc.height))
				x += pc.width
				continue
			}
			if b := pc.box; b != nil {
				if p := b.p; hasDeco(p) {
					line.areas = append(line.areas, &areatree.Container{Kind: areatree.KindInline,
						Rect:       areatree.Rect{X: x, Y: baseline - pc.height, W: pc.width, H: pc.height},
						Background: p.Background, Border: p.Border, Source: b.n.Label()})
				}
				line.areas = append(line.areas, &areatree.Image{
					Rect:  areatree.Rect{X: x + b.start, Y: baseline - pc.height + b.before, W: b.imgW, H: b.imgH},
					Image: pc.img,
				})
				x += pc.width
				continue
			}
			line.areas = append(line.areas, &areatree.Image{
				Rect:  areatree.Rect{X: x, Y: baseline - pc.height, W: pc.width, H: pc.height},
				Image: pc.img,
			})
			x += pc.width
		}
	}
	line.areas = append(frags.areas(baseline), line.areas...)
	line.areas = append(line.areas, linkAreas(pa, seg, x0, align, avail, width, adjust, h)...)
	if hy != nil {
		// Trennstrich am Zeilenende, im Stil des getrennten Wortes.
		if run == nil || runStyle != hy.st {
			run = &areatree.GlyphRun{Origin: areatree.Point{X: x, Y: baseline}, Font: hy.st.fontID,
				Size: hy.st.p.FontSize, Color: hy.st.p.Color, Ascent: hy.st.asc, Descent: hy.st.desc}
			line.areas = append(line.areas, run)
		}
		run.Glyphs = append(run.Glyphs, hy.glyph)
	}
	for _, it := range seg {
		if it.Payload >= 0 {
			line.ids = append(line.ids, pa.pieces[it.Payload].ids...)
		}
	}
	g.addLine(line, ord, pen)
	return nil
}

// drawLeader zeichnet einen fo:leader der Breite w ab x.
func drawLeader(pc piece, x, w, baseline areatree.Length) []areatree.Area {
	l := pc.leader
	switch l.pattern {
	case "rule":
		y := baseline - l.thickness/2
		return []areatree.Area{&areatree.Line{From: areatree.Point{X: x, Y: y}, To: areatree.Point{X: x + w, Y: y},
			Width: l.thickness, Style: l.style, Color: pc.st.p.Color}}
	case "dots":
		gs, err := pc.st.face.Shape(".", pc.st.p.FontSize, font.ShapeOptions{})
		if err != nil || len(gs) != 1 {
			return nil
		}
		dot := gs[0]
		// Die Musterbreite ist Glyphe plus Leerraum (XSL 1.1 7.22.3), sie ist
		// daher mindestens so breit wie der Punkt.
		pw := max(l.patternW, dot.Advance)
		if pw <= 0 {
			return nil
		}
		// Punkte auf ein Raster relativ zum Zeilenanfang, damit sie in
		// aufeinanderfolgenden Zeilen untereinander stehen.
		start := ((x + pw - 1) / pw) * pw
		run := &areatree.GlyphRun{Origin: areatree.Point{X: start, Y: baseline}, Font: pc.st.fontID,
			Size: pc.st.p.FontSize, Color: pc.st.p.Color, Ascent: pc.st.asc, Descent: pc.st.desc}
		for pos := start; pos+dot.Advance <= x+w; pos += pw {
			g := dot
			g.Advance = pw
			run.Glyphs = append(run.Glyphs, g)
		}
		if len(run.Glyphs) == 0 {
			return nil
		}
		return []areatree.Area{run}
	}
	return nil
}

// linkAreas legt über zusammenhängende Stücke desselben fo:basic-link je
// einen Link-Bereich in Zeilenhöhe.
func linkAreas(pa *paragraph, seg []linebreak.Item, x0 areatree.Length, align string, avail, width areatree.Length,
	adjust func(linebreak.Item) areatree.Length, h areatree.Length) []areatree.Area {
	x := x0
	switch align {
	case "center":
		x += (avail - width) / 2
	case "end":
		x += avail - width
	}
	var out []areatree.Area
	var cur *linkRef
	var startX areatree.Length
	flush := func(end areatree.Length) {
		if cur != nil && end > startX {
			out = append(out, &areatree.Link{Rect: areatree.Rect{X: startX, Y: 0, W: end - startX, H: h}, Dest: cur.dest, URI: cur.uri})
		}
		cur = nil
	}
	for _, it := range seg {
		if it.Payload < 0 {
			// Hilfselemente des Flattersatzes unterbrechen keinen Link.
			x += adjust(it)
			continue
		}
		ref := pa.pieces[it.Payload].link
		if ref != cur {
			flush(x)
			if ref != nil {
				cur, startX = ref, x
			}
		}
		x += adjust(it)
	}
	flush(x)
	return out
}

// decoFrags sammelt je Inline-Element mit Hintergrund oder Rahmen den Teil
// in einer Zeile.
type decoFrags struct {
	order []*inlineDeco
	frag  map[*inlineDeco]*decoFrag
}

type decoFrag struct {
	x0, x1     areatree.Length
	start, end bool
}

func (f *decoFrags) add(pc piece, x0, x1 areatree.Length) {
	if f.frag == nil {
		f.frag = map[*inlineDeco]*decoFrag{}
	}
	for i, d := range pc.decos {
		fr, ok := f.frag[d]
		if !ok {
			fr = &decoFrag{x0: x0, x1: x1}
			f.frag[d] = fr
			f.order = append(f.order, d)
		}
		fr.x0, fr.x1 = min(fr.x0, x0), max(fr.x1, x1)
		if pc.kind == pEdge && i == 0 {
			if pc.start {
				fr.start = true
			} else {
				fr.end = true
			}
		}
	}
}

// areas liefert die Hintergründe und Rahmen, äußere Elemente zuerst. Höhe
// ist der Inhaltsbereich der Schrift (Ober- und Unterlänge) plus Padding und
// Rahmen davor und danach. Die Zeilenhöhe ändern sie nicht.
func (f *decoFrags) areas(baseline areatree.Length) []areatree.Area {
	var out []areatree.Area
	for _, d := range f.order {
		fr := f.frag[d]
		p := d.p
		top := baseline - d.st.asc - p.Padding[areatree.Top]
		bottom := baseline + d.st.desc + p.Padding[areatree.Bottom]
		border := p.Border
		if p.Border[areatree.Top].Visible() {
			top -= p.Border[areatree.Top].Width
		}
		if p.Border[areatree.Bottom].Visible() {
			bottom += p.Border[areatree.Bottom].Width
		}
		if !fr.start {
			border[areatree.Left] = areatree.Border{}
		}
		if !fr.end {
			border[areatree.Right] = areatree.Border{}
		}
		out = append(out, &areatree.Container{Kind: areatree.KindInline,
			Rect:       areatree.Rect{X: fr.x0, Y: top, W: fr.x1 - fr.x0, H: bottom - top},
			Background: p.Background, Border: border, Source: d.n.Label()})
	}
	return out
}
