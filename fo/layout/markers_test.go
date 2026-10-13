package layout

import (
	"testing"

	"github.com/tim-riep/pdx/fo/tree"
)

func TestPickMarker(t *testing.T) {
	m := func(s string) markerSet { return markerSet{"k": &tree.Node{Data: s}} }
	evs := []*mkEvent{
		{markers: m("fortgesetzt"), start: false, end: true},
		{markers: m("a"), start: true, end: true},
		{markers: m("b"), start: true, end: false},
	}
	cases := map[string]string{
		"first-starting-within-page": "a",
		"first-including-carryover":  "fortgesetzt",
		"last-starting-within-page":  "b",
		"last-ending-within-page":    "a",
		"first-starting":             "a",
		"last-ending":                "a",
	}
	for pos, want := range cases {
		got := pick(evs, "k", pos)
		if got == nil || got.Data != want {
			t.Errorf("%s: erwartet %s", pos, want)
		}
	}
	if pick(evs, "andere", "first-including-carryover") != nil {
		t.Error("fremde Klasse gefunden")
	}
	if lastAny(evs, "k").Data != "b" {
		t.Error("lastAny falsch")
	}
}
