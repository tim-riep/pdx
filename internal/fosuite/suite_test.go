package fosuite

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tim-riep/pdx/internal/fopcompare"
)

func TestListRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "l.txt")
	m := map[string]string{"fop/b": "Grund b: mit Doppelpunkt", "fop/a": "Grund a"}
	if err := WriteList(p, "Kopf\nzweite Zeile", m); err != nil {
		t.Fatal(err)
	}
	got, err := ReadList(p)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, m) {
		t.Fatalf("%v", got)
	}
	os.WriteFile(p, []byte("fop/x\n"), 0o644)
	if _, err := ReadList(p); err == nil {
		t.Fatal("Fall ohne Begründung ohne Fehler")
	}
}

func TestExpectRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "e.txt")
	e := &Expect{Pages: 2, Lines: []Line{
		{Page: 1, X: 56.693, HasX: true, Y: 85.1, Text: "Zeile mit  # und : Zeichen"},
		{Page: 2, Y: 99.12, Text: HashText("geheim")},
	}}
	if err := WriteExpect(p, "Kopf", e); err != nil {
		t.Fatal(err)
	}
	got, err := ReadExpect(p)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, e) {
		t.Fatalf("%+v", got)
	}
}

func TestCategory(t *testing.T) {
	for name, want := range map[string]string{
		"table_border-separation":    "Tabellen",
		"list-block_label-end":       "Listen",
		"block_keep-together":        "Keeps",
		"markers_5a":                 "Marker",
		"page-number-citation_basic": "Verweise",
		"leader_text-align":          "Leader und Links",
		"page-position_last_1":       "Seitenfolgen",
		"block_hyphenation-ladder":   "Silbentrennung",
		"block_text-align_2":         "Zeilenumbruch",
		"external-graphic_basic":     "Sonstiges",
	} {
		if got := Category(SuiteFOP, name); got != want {
			t.Errorf("%s: %s, erwartet %s", name, got, want)
		}
	}
}

func TestCompare(t *testing.T) {
	e := &Expect{Pages: 1, Lines: []Line{
		{Page: 1, X: 10, HasX: true, Y: 20, Text: "a b"},
		{Page: 1, Y: 40, Text: "c"},
	}}
	c := Case{Suite: SuiteFOP, Name: "x"}
	ok := Outcome{Pages: 1, Lines: []fopcompare.ATLine{
		{Page: 1, Text: "a  b", Baseline: 20.5, X: 10.4, HasX: true},
		{Page: 1, Text: "c", Baseline: 40},
	}}
	if d := Compare(c, ok, e, 1); len(d) != 0 {
		t.Fatalf("Abweichungen %v", d)
	}
	bad := Outcome{Pages: 2, Lines: []fopcompare.ATLine{
		{Page: 1, Text: "a b", Baseline: 22, X: 10, HasX: true},
		{Page: 1, Text: "d", Baseline: 40},
	}}
	got := Compare(c, bad, e, 1)
	want := []string{"Seiten", "verschoben", "nur in pdx", "nur in FOP"}
	if len(got) != len(want) {
		t.Fatalf("Abweichungen %v", got)
	}
	for i, w := range want {
		if !strings.Contains(string(got[i]), w) {
			t.Errorf("%d: %q ohne %q", i, got[i], w)
		}
	}
	if d := Compare(c, Outcome{Err: errors.New("kaputt")}, e, 1); len(d) != 1 || !strings.Contains(string(d[0]), "kaputt") {
		t.Errorf("Fehler nicht gemeldet: %v", d)
	}
	// W3C-Erwartungen enthalten nur Hashwerte.
	w := Case{Suite: SuiteW3C, Name: "y"}
	he := &Expect{Pages: 1, Lines: []Line{{Page: 1, Y: 20, Text: HashText("a b")}}}
	if d := Compare(w, Outcome{Pages: 1, Lines: []fopcompare.ATLine{{Page: 1, Text: "a b", Baseline: 20}}}, he, 1); len(d) != 0 {
		t.Errorf("Hashvergleich: %v", d)
	}
}
