package layout

import (
	"testing"

	"github.com/tim-riep/pdx/areatree"
)

func boxes(hs ...int) []item {
	var it []item
	for i, h := range hs {
		if i > 0 {
			it = append(it, item{kind: itPenalty})
		}
		it = append(it, item{kind: itBox, h: Lpt(h)})
	}
	return it
}

func Lpt(v int) areatree.Length { return areatree.Length(v * 1000) }

func TestBreakPage(t *testing.T) {
	items := boxes(10, 10, 10, 10) // Penalties bei 1, 3, 5
	if got := breakPage(items, 0, Lpt(25)); got != 3 {
		t.Errorf("Umbruch bei %d, erwartet 3", got)
	}
	if got := breakPage(items, 0, Lpt(100)); got != len(items) {
		t.Errorf("alles passt, erhalten %d", got)
	}
	// Keep: die Stelle bei 3 ist teuer, gewählt wird die billigere bei 1.
	items[3].penalty = 5
	if got := breakPage(items, 0, Lpt(25)); got != 1 {
		t.Errorf("Keep nicht beachtet: %d", got)
	}
	// Nur verbotene Stellen: späteste wird genommen statt Inhalt zu verlieren.
	items[1].penalty, items[3].penalty = penaltyInf, penaltyInf
	if got := breakPage(items, 0, Lpt(25)); got != 3 {
		t.Errorf("Rückfall bei verbotenen Stellen: %d", got)
	}
	// Erzwungener Umbruch.
	items[1].penalty = penaltyForced
	if got := breakPage(items, 0, Lpt(100)); got != 1 {
		t.Errorf("erzwungener Umbruch: %d", got)
	}
	// Glue nach einer Penalty zählt nur, wenn danach Inhalt folgt.
	g := []item{{kind: itBox, h: Lpt(10)}, {kind: itPenalty}, {kind: itGlue, h: Lpt(20)}, {kind: itBox, h: Lpt(10)}}
	if got := breakPage(g, 0, Lpt(35)); got != 1 {
		t.Errorf("Glue falsch gezählt: %d", got)
	}
	// Übergroße Box: Umbruch an der ersten Stelle danach.
	if got := breakPage(boxes(50, 10), 0, Lpt(20)); got != 1 {
		t.Errorf("übergroße Box: %d", got)
	}
}
