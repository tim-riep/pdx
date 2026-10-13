package linebreak

import (
	"testing"

	"github.com/tim-riep/pdx/areatree"
)

// words baut Boxen der angegebenen Breiten mit Glue der Breite 1 dazwischen.
func words(ws ...areatree.Length) []Item {
	var it []Item
	for i, w := range ws {
		if i > 0 {
			it = append(it, Item{Kind: Glue, Width: 1, Stretch: 1, Shrink: 0})
		}
		it = append(it, Item{Kind: Box, Width: w, Payload: i})
	}
	return it
}

func fixed(w areatree.Length) LineWidth { return func(int) areatree.Length { return w } }

func indices(bs []Break) []int {
	var out []int
	for _, b := range bs {
		out = append(out, b.Index)
	}
	return out
}

func TestGreedy(t *testing.T) {
	cases := []struct {
		name  string
		items []Item
		width areatree.Length
		want  []int
	}{
		{"eine Zeile", words(3, 3), 10, []int{3}},
		{"zwei Zeilen", words(4, 4, 4), 10, []int{3, 5}},
		{"exakt", words(4, 5), 10, []int{3}},
		{"Überlauf", words(20, 2), 10, []int{1, 3}},
		{"leer", nil, 10, nil},
	}
	for _, c := range cases {
		bs, err := Greedy{}.Break(c.items, fixed(c.width))
		if err != nil {
			t.Fatal(err)
		}
		got := indices(bs)
		if len(got) != len(c.want) {
			t.Errorf("%s: %v, erwartet %v", c.name, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: %v, erwartet %v", c.name, got, c.want)
			}
		}
	}
}

func TestForcedBreakAndRatio(t *testing.T) {
	items := words(2, 2)
	items = append(items, Item{Kind: Penalty, Penalty: ForcedBreak})
	items = append(items, words(2)...)
	bs, _ := Greedy{}.Break(items, fixed(10))
	if got := indices(bs); len(got) != 2 || got[0] != 3 || got[1] != 5 {
		t.Fatalf("Umbrüche %v", got)
	}
	if bs[0].Ratio != 0 {
		t.Errorf("Zeile vor erzwungenem Umbruch darf nicht gedehnt werden: %v", bs[0].Ratio)
	}
	bs, _ = Greedy{}.Break(words(4, 4, 4), fixed(10))
	if bs[0].Ratio != 1 {
		t.Errorf("Ratio der ersten Zeile %v, erwartet 1", bs[0].Ratio)
	}
}
