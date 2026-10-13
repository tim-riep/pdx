package layout

import (
	"testing"

	"github.com/tim-riep/pdx/fo/tree"
)

func TestFormatNumber(t *testing.T) {
	cases := map[string]string{"1": "7", "01": "07", "001": "007", "i": "vii", "I": "VII", "a": "g", "A": "G"}
	for f, want := range cases {
		if got := formatNumber(7, f); got != want {
			t.Errorf("formatNumber(7, %q) = %q, erwartet %q", f, got, want)
		}
	}
	if formatNumber(1994, "I") != "MCMXCIV" || formatNumber(28, "a") != "ab" {
		t.Error("römisch oder alphabetisch falsch")
	}
}

func TestSelector(t *testing.T) {
	e := &engine{masters: map[string]*master{"erste": {}, "gerade": {}, "ungerade": {}, "letzte": {}, "leer": {}, "einmal": {}}}
	n := &tree.Node{Line: 1, Col: 1}
	sm := &seqMaster{node: n, specs: []pageSpec{
		{kind: specSingle, master: "einmal", max: 1},
		{kind: specAlternatives, max: -1, conds: []condRef{
			{master: "leer", position: "any", oddEven: "any", blank: "blank"},
			{master: "letzte", position: "last", oddEven: "any", blank: "any"},
			{master: "gerade", position: "any", oddEven: "even", blank: "any"},
			{master: "ungerade", position: "any", oddEven: "odd", blank: "any"},
		}},
	}}
	s := &selector{e: e, sm: sm}
	want := []struct {
		c    pageCond
		name string
	}{
		{pageCond{number: 1, first: true}, "einmal"},
		{pageCond{number: 2}, "gerade"},
		{pageCond{number: 3}, "ungerade"},
		{pageCond{number: 4, blank: true}, "leer"},
		{pageCond{number: 5, last: true}, "letzte"},
	}
	for _, w := range want {
		// Vorausschau darf nichts verbrauchen.
		if _, err := s.pick(w.c, false); err != nil {
			t.Fatal(err)
		}
		m, err := s.pick(w.c, true)
		if err != nil {
			t.Fatal(err)
		}
		if m != e.masters[w.name] {
			t.Errorf("Seite %d: falsche Vorlage, erwartet %s", w.c.number, w.name)
		}
	}
	// Erschöpfte Folge ist ein Fehler.
	s2 := &selector{e: e, sm: &seqMaster{node: n, specs: []pageSpec{{kind: specSingle, master: "einmal", max: 1, node: n}}}}
	s2.pick(pageCond{number: 1}, true)
	if _, err := s2.pick(pageCond{number: 2}, true); err == nil {
		t.Error("erschöpfte Seitenfolge nicht erkannt")
	}
}
