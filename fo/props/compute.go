package props

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/fo/tree"
)

// Computed sind die aufgelösten Eigenschaften eines FO-Elements. Längen sind
// absolut. Die ersten Felder werden vererbt, die übrigen nicht.
type Computed struct {
	// Vererbte Eigenschaften.
	FontFamily    string
	FontSize      Length
	FontWeight    int
	Italic        bool
	Color         areatree.Color
	TextAlign     string // start, center, end, justify (left und right sind abgebildet)
	TextAlignLast string
	// LineHeightFactor ist gesetzt, wenn line-height als Zahl oder normal
	// angegeben ist. Dann vererbt sich der Faktor, nicht die Länge.
	LineHeightFactor float64
	LineHeight       Length
	StartIndent      Length
	EndIndent        Length
	TextIndent       Length
	LinefeedPreserve bool
	Widows           int
	Hyphenate        bool
	HyphenChar       string
	HyphenPush       int
	HyphenRemain     int
	Orphans          int
	Lang             string

	// Nicht vererbte Eigenschaften.
	SpaceBefore, SpaceAfter Length
	Padding                 [4]Length
	Border                  [4]areatree.Border
	Background              *areatree.Color
	// Absolute ist "absolute" oder "fixed" bei absoluter Positionierung.
	Absolute                 string
	Top, Left, Right, Bottom *Length
	Width, Height            *Length
}

// Root liefert die Anfangswerte nach XSL 1.1 (Schrift wie bei Apache FOP
// 12pt sans-serif).
func Root() *Computed {
	return &Computed{
		FontFamily:       "sans-serif",
		FontSize:         12 * areatree.Pt,
		FontWeight:       400,
		Color:            areatree.Black,
		TextAlign:        "start",
		TextAlignLast:    "relative",
		LineHeightFactor: 1.2,
		LineHeight:       areatree.Points(14.4),
		Widows:           2,
		Orphans:          2,
		HyphenChar:       "\u2010",
		HyphenPush:       2,
		HyphenRemain:     2,
	}
}

// inherit kopiert die vererbten Eigenschaften.
func (c *Computed) inherit() *Computed {
	return &Computed{
		FontFamily: c.FontFamily, FontSize: c.FontSize, FontWeight: c.FontWeight, Italic: c.Italic,
		Color: c.Color, TextAlign: c.TextAlign, TextAlignLast: c.TextAlignLast,
		LineHeightFactor: c.LineHeightFactor, LineHeight: c.LineHeight,
		StartIndent: c.StartIndent, EndIndent: c.EndIndent, TextIndent: c.TextIndent,
		LinefeedPreserve: c.LinefeedPreserve, Lang: c.Lang, Widows: c.Widows, Orphans: c.Orphans,
		Hyphenate: c.Hyphenate, HyphenChar: c.HyphenChar, HyphenPush: c.HyphenPush, HyphenRemain: c.HyphenRemain,
	}
}

var fontSizes = map[string]float64{
	"xx-small": 7, "x-small": 8.3, "small": 10, "medium": 12, "large": 14.4, "x-large": 17.3, "xx-large": 20.7,
}

// Compute löst die Eigenschaften eines Elements auf. width ist die Breite
// des umgebenden Bereichs für Prozentangaben.
func Compute(parent *Computed, n *tree.Node, width Length) (*Computed, error) {
	return ComputeEnv(parent, n, width, nil)
}

// ComputeEnv ist Compute mit Kontextfunktionen wie body-start().
func ComputeEnv(parent *Computed, n *tree.Node, width Length, env *Env) (*Computed, error) {
	c := parent.inherit()
	a := n.Attrs
	fail := func(prop string, err error) error {
		return tree.Errorf(n, "%s: %v", prop, err)
	}

	// Schriftgröße zuerst, weil em-Angaben davon abhängen.
	if v, ok := a["font-size"]; ok {
		switch {
		case fontSizes[v] > 0:
			c.FontSize = areatree.Points(fontSizes[v])
		case v == "smaller":
			c.FontSize = Length(float64(parent.FontSize) / 1.2)
		case v == "larger":
			c.FontSize = Length(float64(parent.FontSize) * 1.2)
		default:
			l, err := EvalLength(v, parent.FontSize, parent.FontSize, env)
			if err != nil {
				return nil, fail("font-size", err)
			}
			if l <= 0 {
				return nil, fail("font-size", fmt.Errorf("muss positiv sein"))
			}
			c.FontSize = l
		}
	}
	fs := c.FontSize
	if v, ok := a["font-family"]; ok {
		c.FontFamily = v
	}
	if v, ok := a["font-weight"]; ok {
		switch v {
		case "normal":
			c.FontWeight = 400
		case "bold":
			c.FontWeight = 700
		case "bolder":
			c.FontWeight = min(parent.FontWeight+300, 900)
		case "lighter":
			c.FontWeight = max(parent.FontWeight-300, 100)
		default:
			w, err := strconv.Atoi(v)
			if err != nil || w < 100 || w > 900 {
				return nil, fail("font-weight", fmt.Errorf("ungültiger Wert %q", v))
			}
			c.FontWeight = w
		}
	}
	if v, ok := a["font-style"]; ok {
		switch v {
		case "normal":
			c.Italic = false
		case "italic", "oblique", "backslant":
			c.Italic = true
		default:
			return nil, fail("font-style", fmt.Errorf("ungültiger Wert %q", v))
		}
	}
	if v, ok := a["color"]; ok {
		col, set, err := ParseColor(v)
		if err != nil {
			return nil, fail("color", err)
		}
		if set {
			c.Color = col
		}
	}
	if v, ok := a["text-align"]; ok {
		al, err := align(v)
		if err != nil {
			return nil, fail("text-align", err)
		}
		c.TextAlign = al
	}
	if v, ok := a["text-align-last"]; ok {
		if v == "relative" {
			c.TextAlignLast = v
		} else {
			al, err := align(v)
			if err != nil {
				return nil, fail("text-align-last", err)
			}
			c.TextAlignLast = al
		}
	}
	// line-height: Faktor oder Länge. Ein vererbter Faktor gilt mit der
	// eigenen Schriftgröße.
	if v, ok := a["line-height"]; ok {
		if v == "normal" {
			c.LineHeightFactor = 1.2
		} else if f, err := strconv.ParseFloat(v, 64); err == nil {
			c.LineHeightFactor = f
		} else {
			l, err := EvalLength(v, fs, fs, env)
			if err != nil {
				return nil, fail("line-height", err)
			}
			c.LineHeightFactor, c.LineHeight = 0, l
		}
	}
	if c.LineHeightFactor > 0 {
		c.LineHeight = Length(float64(fs) * c.LineHeightFactor)
	}
	if v, ok := a["hyphenate"]; ok {
		switch v {
		case "true":
			c.Hyphenate = true
		case "false":
			c.Hyphenate = false
		default:
			return nil, fail("hyphenate", fmt.Errorf("ungültiger Wert %q", v))
		}
	}
	if v, ok := a["hyphenation-character"]; ok && v != "" {
		c.HyphenChar = v
	}
	for _, wo := range []struct {
		name string
		dst  *int
	}{{"widows", &c.Widows}, {"orphans", &c.Orphans},
		{"hyphenation-push-character-count", &c.HyphenPush}, {"hyphenation-remain-character-count", &c.HyphenRemain}} {
		if v, ok := a[wo.name]; ok {
			k, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil || k < 0 {
				return nil, fail(wo.name, fmt.Errorf("ungültiger Wert %q", v))
			}
			*wo.dst = k
		}
	}
	if v, ok := a["linefeed-treatment"]; ok {
		c.LinefeedPreserve = v == "preserve"
	}
	if v, ok := a["white-space"]; ok && v == "pre" {
		c.LinefeedPreserve = true
	}
	if v, ok := a["xml:lang"]; ok {
		c.Lang = v
	} else if v, ok := a["language"]; ok {
		c.Lang = v
		if ct, ok := a["country"]; ok {
			c.Lang += "-" + strings.ToUpper(ct)
		}
	}

	if err := c.boxProps(n, parent, width, env); err != nil {
		return nil, err
	}
	return c, nil
}

func align(v string) (string, error) {
	switch v {
	case "start", "left", "inside":
		return "start", nil
	case "end", "right", "outside":
		return "end", nil
	case "center", "justify":
		return v, nil
	}
	return "", fmt.Errorf("ungültiger Wert %q", v)
}

// sides ordnet die Bezeichner der vier Seiten zu (Schreibrichtung lr-tb).
var sides = [4][]string{
	areatree.Top:    {"top", "before"},
	areatree.Right:  {"right", "end"},
	areatree.Bottom: {"bottom", "after"},
	areatree.Left:   {"left", "start"},
}

// lookupSide sucht prop-Seite, etwa padding-top oder padding-before.
func lookupSide(a map[string]string, prefix string, side int, suffix string) (string, bool) {
	for _, s := range sides[side] {
		k := prefix + "-" + s
		if suffix != "" {
			k += "-" + suffix
		}
		if v, ok := a[k]; ok {
			return v, true
		}
		if suffix == "" {
			// Komponentenform wie padding-before.length.
			if v, ok := a[k+".length"]; ok {
				return v, true
			}
		}
	}
	return "", false
}

func (c *Computed) boxProps(n *tree.Node, parent *Computed, width Length, env *Env) error {
	a := n.Attrs
	fs := c.FontSize
	fail := func(prop string, err error) error { return tree.Errorf(n, "%s: %v", prop, err) }
	length := func(prop, v string) (Length, error) {
		l, err := EvalLength(v, fs, width, env)
		if err != nil {
			return 0, fail(prop, err)
		}
		return l, nil
	}

	// Padding.
	if v, ok := a["padding"]; ok {
		vs, err := expand4(splitValues(v))
		if err != nil {
			return fail("padding", err)
		}
		for i, s := range vs {
			if c.Padding[i], err = length("padding", s); err != nil {
				return err
			}
		}
	}
	for side := 0; side < 4; side++ {
		if v, ok := lookupSide(a, "padding", side, ""); ok {
			l, err := length("padding", v)
			if err != nil {
				return err
			}
			c.Padding[side] = l
		}
	}

	// Rahmen: Shorthand border, border-Seite, border-width/style/color, Einzelwerte.
	var bw [4]Length
	var bs [4]areatree.BorderStyle
	var bc [4]areatree.Color
	for i := range bc {
		bc[i] = c.Color
		bw[i] = 1000 // medium
	}
	applyBorder := func(side int, v string) error {
		for _, part := range splitValues(v) {
			if st, ok := borderStyles[part]; ok {
				bs[side] = st
				continue
			}
			if col, set, err := ParseColor(part); err == nil {
				if set {
					bc[side] = col
				}
				continue
			}
			w, err := parseBorderWidth(part, fs)
			if err != nil {
				return fail("border", fmt.Errorf("Wert %q nicht verstanden", part))
			}
			bw[side] = w
		}
		return nil
	}
	if v, ok := a["border"]; ok {
		for side := 0; side < 4; side++ {
			if err := applyBorder(side, v); err != nil {
				return err
			}
		}
	}
	for _, kind := range []string{"width", "style", "color"} {
		v, ok := a["border-"+kind]
		if !ok {
			continue
		}
		vs, err := expand4(splitValues(v))
		if err != nil {
			return fail("border-"+kind, err)
		}
		for side, s := range vs {
			if err := c.borderPart(kind, s, side, &bw, &bs, &bc); err != nil {
				return fail("border-"+kind, err)
			}
		}
	}
	for side := 0; side < 4; side++ {
		if v, ok := lookupSide(a, "border", side, ""); ok {
			if err := applyBorder(side, v); err != nil {
				return err
			}
		}
		for _, kind := range []string{"width", "style", "color"} {
			if v, ok := lookupSide(a, "border", side, kind); ok {
				if err := c.borderPart(kind, v, side, &bw, &bs, &bc); err != nil {
					return fail("border-"+kind, err)
				}
			}
		}
	}
	for i := 0; i < 4; i++ {
		if bs[i] != areatree.None {
			c.Border[i] = areatree.Border{Width: bw[i], Style: bs[i], Color: bc[i]}
		}
	}

	if v, ok := a["background-color"]; ok {
		col, set, err := ParseColor(v)
		if err != nil {
			return fail("background-color", err)
		}
		if set {
			c.Background = &col
		}
	}

	// Ränder. margin-top und margin-bottom wirken wie space-before und
	// space-after, margin-left und margin-right setzen die Einzüge (XSL 1.1
	// 5.3.2).
	var margin [4]*Length
	if v, ok := a["margin"]; ok {
		vs, err := expand4(splitValues(v))
		if err != nil {
			return fail("margin", err)
		}
		for i, s := range vs {
			l, err := length("margin", s)
			if err != nil {
				return err
			}
			margin[i] = &l
		}
	}
	for side, name := range []string{"margin-top", "margin-right", "margin-bottom", "margin-left"} {
		if v, ok := a[name]; ok {
			l, err := length(name, v)
			if err != nil {
				return err
			}
			margin[side] = &l
		}
	}
	bwid := func(side int) Length {
		if c.Border[side].Visible() {
			return c.Border[side].Width
		}
		return 0
	}
	if margin[areatree.Left] != nil {
		c.StartIndent = parent.StartIndent + *margin[areatree.Left] + bwid(areatree.Left) + c.Padding[areatree.Left]
	}
	if margin[areatree.Right] != nil {
		c.EndIndent = parent.EndIndent + *margin[areatree.Right] + bwid(areatree.Right) + c.Padding[areatree.Right]
	}
	if v, ok := a["start-indent"]; ok {
		l, err := length("start-indent", v)
		if err != nil {
			return err
		}
		c.StartIndent = l
	}
	if v, ok := a["end-indent"]; ok {
		l, err := length("end-indent", v)
		if err != nil {
			return err
		}
		c.EndIndent = l
	}
	if v, ok := a["text-indent"]; ok {
		l, err := length("text-indent", v)
		if err != nil {
			return err
		}
		c.TextIndent = l
	}
	if margin[areatree.Top] != nil {
		c.SpaceBefore = *margin[areatree.Top]
	}
	if margin[areatree.Bottom] != nil {
		c.SpaceAfter = *margin[areatree.Bottom]
	}
	for _, sp := range []struct {
		name string
		dst  *Length
	}{{"space-before", &c.SpaceBefore}, {"space-after", &c.SpaceAfter}} {
		v, ok := a[sp.name]
		if !ok {
			v, ok = a[sp.name+".optimum"]
		}
		if !ok {
			continue
		}
		l, err := length(sp.name, v)
		if err != nil {
			return err
		}
		*sp.dst = l
	}

	// Positionierung und Größe.
	if v, ok := a["absolute-position"]; ok {
		switch v {
		case "absolute", "fixed":
			c.Absolute = v
		case "auto":
		default:
			return fail("absolute-position", fmt.Errorf("Wert %q wird nicht unterstützt", v))
		}
	}
	for _, p := range []struct {
		name string
		dst  **Length
	}{{"top", &c.Top}, {"left", &c.Left}, {"right", &c.Right}, {"bottom", &c.Bottom}, {"width", &c.Width}, {"height", &c.Height}} {
		v, ok := a[p.name]
		if !ok || v == "auto" {
			continue
		}
		l, err := length(p.name, v)
		if err != nil {
			return err
		}
		*p.dst = &l
	}
	return nil
}

func (c *Computed) borderPart(kind, s string, side int, bw *[4]Length, bs *[4]areatree.BorderStyle, bc *[4]areatree.Color) error {
	switch kind {
	case "width":
		w, err := parseBorderWidth(s, c.FontSize)
		if err != nil {
			return err
		}
		bw[side] = w
	case "style":
		st, ok := borderStyles[s]
		if !ok {
			return fmt.Errorf("unbekannter Linienstil %q", s)
		}
		bs[side] = st
	case "color":
		col, set, err := ParseColor(s)
		if err != nil {
			return err
		}
		if set {
			bc[side] = col
		}
	}
	return nil
}

// BorderWidth liefert die sichtbare Rahmenbreite einer Seite.
func (c *Computed) BorderWidth(side int) Length {
	if c.Border[side].Visible() {
		return c.Border[side].Width
	}
	return 0
}

// Margins liest die Ränder eines Seiten- oder Regionselements (margin und
// margin-Seite). Prozentangaben beziehen sich auf base.
func Margins(n *tree.Node, base Length) ([4]Length, error) {
	var m [4]Length
	fs := 12 * areatree.Pt
	if v, ok := n.Attrs["margin"]; ok {
		vs, err := expand4(splitValues(v))
		if err != nil {
			return m, tree.Errorf(n, "margin: %v", err)
		}
		for i, s := range vs {
			l, err := ParseLength(s, fs, base)
			if err != nil {
				return m, tree.Errorf(n, "margin: %v", err)
			}
			m[i] = l
		}
	}
	for side, name := range []string{"margin-top", "margin-right", "margin-bottom", "margin-left"} {
		if v, ok := n.Attrs[name]; ok {
			l, err := ParseLength(v, fs, base)
			if err != nil {
				return m, tree.Errorf(n, "%s: %v", name, err)
			}
			m[side] = l
		}
	}
	return m, nil
}

// LengthAttr liest ein Längenattribut oder liefert def.
func LengthAttr(n *tree.Node, name string, def, fontSize, percentBase Length) (Length, error) {
	v, ok := n.Attrs[name]
	if !ok || v == "auto" {
		return def, nil
	}
	l, err := ParseLength(v, fontSize, percentBase)
	if err != nil {
		return 0, tree.Errorf(n, "%s: %v", name, err)
	}
	return l, nil
}
