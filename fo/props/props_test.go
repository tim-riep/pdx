package props

import (
	"math"
	"testing"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/fo/tree"
)

func TestParseLength(t *testing.T) {
	cases := map[string]Length{"12pt": 12000, "10mm": 28346, "1in": 72000, "2.5cm": 70866, "1pc": 12000,
		"1.5em": 15000, "50%": 50000, "0": 0, "-3pt": -3000, "4px": 3000}
	for in, want := range cases {
		got, err := ParseLength(in, 10000, 100000)
		if err != nil || got != want {
			t.Errorf("ParseLength(%q) = %d, %v, erwartet %d", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "12", "pt", "12xx", "1 2pt"} {
		if _, err := ParseLength(bad, 10000, 100000); err == nil {
			t.Errorf("ParseLength(%q) ohne Fehler", bad)
		}
	}
}

func TestParseColor(t *testing.T) {
	c, ok, err := ParseColor("#ff8000")
	if err != nil || !ok || c.Space != areatree.RGB || c.C[1] != 0.502 {
		t.Errorf("#ff8000 = %+v", c)
	}
	c, _, _ = ParseColor("#333")
	if c.Space != areatree.Gray || c.C[0] != 0.2 {
		t.Errorf("#333 = %+v", c)
	}
	if _, ok, _ := ParseColor("transparent"); ok {
		t.Error("transparent sollte keine Farbe liefern")
	}
	if c, _, err := ParseColor("rgb(255, 0, 0)"); err != nil || c.C[0] != 1 {
		t.Errorf("rgb() = %+v %v", c, err)
	}
	if _, _, err := ParseColor("lila"); err == nil {
		t.Error("unbekannte Farbe ohne Fehler")
	}
}

func node(attrs map[string]string) *tree.Node {
	return &tree.Node{Kind: tree.Element, Space: tree.NSFO, Name: "block", Attrs: attrs, Line: 1, Col: 1}
}

func TestComputeInheritanceAndBox(t *testing.T) {
	root := Root()
	p, err := Compute(root, node(map[string]string{"font-size": "10pt", "line-height": "1.5", "margin-left": "1cm",
		"border": "0.5pt solid #ff0000", "padding": "2pt 4pt", "space-before": "6pt", "text-align": "right"}), 400000)
	if err != nil {
		t.Fatal(err)
	}
	if p.LineHeight != 15000 || p.TextAlign != "end" || p.SpaceBefore != 6000 {
		t.Errorf("Eigenschaften: %+v", p)
	}
	if p.StartIndent != 28346+500+4000 {
		t.Errorf("start-indent %d", p.StartIndent)
	}
	if !p.Border[areatree.Top].Visible() || p.Border[areatree.Top].Width != 500 || p.Border[areatree.Top].Color.C[0] != 1 {
		t.Errorf("Rahmen %+v", p.Border)
	}
	c, err := Compute(p, node(map[string]string{"font-size": "200%"}), 400000)
	if err != nil {
		t.Fatal(err)
	}
	if c.FontSize != 20000 || c.LineHeight != 30000 || c.StartIndent != p.StartIndent || c.SpaceBefore != 0 || c.Border[0].Visible() {
		t.Errorf("Vererbung: %+v", c)
	}
	if _, err := Compute(root, node(map[string]string{"font-weight": "fett"}), 0); err == nil {
		t.Error("ungültiges font-weight ohne Fehler")
	}
}

func TestEvalLength(t *testing.T) {
	env := &Env{BodyStart: func() (Length, error) { return 30000, nil }, LabelEnd: func() (Length, error) { return 400000, nil }}
	cases := map[string]Length{
		"2pt + 3mm":         2000 + 8504,
		"(10pt - 4pt) * 2":  12000,
		"12pt div 4":        3000,
		"-1em":              -10000,
		"body-start()":      30000,
		"label-end() - 6pt": 394000,
		"max(5pt, 1em)":     10000,
		"abs(-3pt)":         3000,
		"50% + 10pt":        60000,
	}
	for in, want := range cases {
		got, err := EvalLength(in, 10000, 100000, env)
		if err != nil || got != want {
			t.Errorf("EvalLength(%q) = %d, %v, erwartet %d", in, got, err, want)
		}
	}
	for _, bad := range []string{"2pt + 3", "body-start()", "(1pt", "2pt * 3pt", "foo(1)", "proportional-column-width(1)"} {
		if _, err := EvalLength(bad, 10000, 100000, nil); err == nil {
			t.Errorf("EvalLength(%q) ohne Fehler", bad)
		}
	}
	if _, p, err := EvalColumnWidth("proportional-column-width(2)", 10000, 100000); err != nil || p != 2 {
		t.Errorf("proportional-column-width: %v %v", p, err)
	}
	if f, _, err := EvalColumnWidth("25%", 10000, 200000); err != nil || f != 50000 {
		t.Errorf("column-width 25%%: %v %v", f, err)
	}
}

func TestNamedColorsAndExpressions(t *testing.T) {
	for _, name := range []string{"lightgray", "PaleGoldenrod", "cyan", "lightgrey"} {
		if _, ok, err := ParseColor(name); err != nil || !ok {
			t.Errorf("%s: %v", name, err)
		}
	}
	for s, want := range map[string]float64{"0.5in + 1mm": 36 + 72/25.4, "1in - -4pc": 120, "12pt * 1.2 * 2": 28.8} {
		l, err := ParseLength(s, 12*areatree.Pt, -1)
		if err != nil || math.Abs(l.Pt()-want) > 0.01 {
			t.Errorf("%s: %v %v, erwartet %v", s, l.Pt(), err, want)
		}
	}
}
