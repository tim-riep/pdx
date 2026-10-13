package props

import (
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
