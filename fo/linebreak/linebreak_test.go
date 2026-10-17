package linebreak

import (
	"math"
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

// cost berechnet die Kosten einer Umbruchfolge wie Knuth-Plass: je Zeile
// (1 + 100·|r|³)², bei der letzten Zeile mit Füllung in Zeilenbreite, plus
// 50 je Wechsel zwischen nicht benachbarten Fitness-Klassen. ok ist false,
// wenn eine Zeile außerhalb von -1 ≤ r ≤ tol liegt.
func cost(items []Item, breaks []int, avail areatree.Length, tol float64) (float64, bool) {
	var sum float64
	start := 0
	prevFit := -1
	for li, b := range breaks {
		var L, Y, Z float64
		for i := start; i < b; i++ {
			L += float64(items[i].Width)
			Y += float64(items[i].Stretch)
			Z += float64(items[i].Shrink)
		}
		if li == len(breaks)-1 {
			Y += float64(avail)
		}
		var r float64
		switch {
		case L < float64(avail):
			r = (float64(avail) - L) / Y
		case L > float64(avail):
			if Z == 0 {
				return 0, false
			}
			r = (float64(avail) - L) / Z
		}
		if r < -1 || r > tol {
			return 0, false
		}
		f := 1 + 100*math.Pow(math.Abs(r), 3)
		sum += f * f
		fit := fitness(r)
		if prevFit >= 0 && abs(fit-prevFit) > 1 {
			sum += 50
		}
		prevFit = fit
		start = b + 1
	}
	return sum, true
}

func TestKnuthPlassIsOptimal(t *testing.T) {
	// Erschöpfende Suche über alle Umbruchfolgen kleiner Absätze im
	// Blocksatz. Knuth-Plass muss die minimalen Kosten finden.
	seed := uint32(11)
	next := func() uint32 { seed = seed*1664525 + 1013904223; return seed >> 8 }
	const avail = 10000
	checked := 0
	for p := 0; p < 60; p++ {
		var items []Item
		var glues []int
		for i := 0; i < 12; i++ {
			if i > 0 {
				glues = append(glues, len(items))
				items = append(items, Item{Kind: Glue, Width: 300, Stretch: 300, Shrink: 100})
			}
			items = append(items, Item{Kind: Box, Width: areatree.Length(900 + next()%1800)})
		}
		best := math.Inf(1)
		for mask := 0; mask < 1<<len(glues); mask++ {
			var bs []int
			for k, g := range glues {
				if mask&(1<<k) != 0 {
					bs = append(bs, g)
				}
			}
			bs = append(bs, len(items))
			if c, ok := cost(items, bs, avail, 1); ok && c < best {
				best = c
			}
		}
		if math.IsInf(best, 1) {
			continue
		}
		checked++
		got, err := KnuthPlass{Tolerances: []float64{1}}.Break(items, fixed(avail))
		if err != nil {
			t.Fatal(err)
		}
		c, ok := cost(items, indices(got), avail, 1)
		if !ok || math.Abs(c-best) > 1e-6*best {
			t.Errorf("Absatz %d: Knuth-Plass %.3f (zulässig %v), Optimum %.3f, Umbrüche %v", p, c, ok, best, indices(got))
		}
	}
	if checked < 20 {
		t.Fatalf("nur %d Absätze geprüft", checked)
	}
}

func TestKnuthPlassForcedAndOverfull(t *testing.T) {
	items := words(20, 20)
	items = append(items, Item{Kind: Penalty, Penalty: ForcedBreak})
	items = append(items, words(500)...) // überlange Box
	bs, err := KnuthPlass{}.Break(items, fixed(100))
	if err != nil {
		t.Fatal(err)
	}
	got := indices(bs)
	if len(got) != 2 || got[0] != 3 || got[1] != len(items) {
		t.Fatalf("Umbrüche %v", got)
	}
	if bs[0].Ratio != 0 {
		t.Errorf("Zeile vor erzwungenem Umbruch gedehnt: %v", bs[0].Ratio)
	}
}

func TestKnuthPlassRagged(t *testing.T) {
	// Flattersatz nach FOP: Glue(0,+R) Penalty Glue(w,-R).
	var items []Item
	for i, w := range []areatree.Length{40, 40, 40, 40} {
		if i > 0 {
			items = append(items,
				Item{Kind: Glue, Stretch: RaggedStretch, Payload: -1},
				Item{Kind: Penalty, Payload: -1},
				Item{Kind: Glue, Width: 5, Stretch: -RaggedStretch, Payload: -1})
		}
		items = append(items, Item{Kind: Box, Width: w})
	}
	bs, err := KnuthPlass{}.Break(items, fixed(130))
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) != 2 {
		t.Fatalf("Umbrüche %v", indices(bs))
	}
	if items[bs[0].Index].Kind != Penalty {
		t.Errorf("Flattersatz bricht nicht an der Penalty: %v", indices(bs))
	}
}
