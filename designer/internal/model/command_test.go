package model

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tim-riep/pdx/template"
)

func muster(t testing.TB) *Document {
	t.Helper()
	d, err := Open("../../../testdata/templates/musterrechnung")
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func canon(d *Document) []byte { return template.Write(d.Pkg.Template) }

func TestCommands(t *testing.T) {
	d := muster(t)
	start := canon(d)
	steps := []struct {
		label string
		cmd   Command
		check func() bool
	}{
		{"Breite", Command{Op: "setAttr", Ref: "adressfenster", Name: "w", Value: "90mm"},
			func() bool { return d.Pkg.Template.ByID("adressfenster").Get("w", "") == "90mm" }},
		{"Ohne Abstand", Command{Op: "setAttr", Ref: "summen", Name: "spaceBefore", Value: ""},
			func() bool { _, ok := d.Pkg.Template.ByID("summen").Attr("spaceBefore"); return !ok }},
		{"Text einfügen", Command{Op: "insert", Parent: "summen", Index: 0, XML: `<text id="hinweis">Zahlbar sofort</text>`},
			func() bool { return d.Pkg.Template.ByID("hinweis").Index() == 0 }},
		{"Verschieben", Command{Op: "move", Ref: "hinweis", Parent: "inhalt", Index: -1},
			func() bool { return d.Pkg.Template.ByID("hinweis").Parent.ID() == "inhalt" }},
		{"Entfernen", Command{Op: "remove", Ref: "steuer"},
			func() bool { return d.Pkg.Template.ByID("steuer") == nil }},
		{"Umbenennen", Command{Op: "setAttr", Ref: "brutto", Name: "id", Value: "gesamt"},
			func() bool { return d.Pkg.Template.ByID("gesamt") != nil && d.Pkg.Template.ByID("brutto") == nil }},
		{"Mehrere", Command{Op: "batch", Cmds: []Command{
			{Op: "setAttr", Ref: "gesamt", Name: "fontSize", Value: "12pt"},
			{Op: "setAttr", Ref: "gesamt", Name: "color", Value: "#003366"},
		}}, func() bool { n := d.Pkg.Template.ByID("gesamt"); return n.Get("fontSize", "") == "12pt" && n.Get("color", "") == "#003366" }},
	}
	var states [][]byte
	for _, s := range steps {
		states = append(states, canon(d))
		if err := d.Apply(s.label, s.cmd); err != nil {
			t.Fatalf("%s: %v", s.label, err)
		}
		if !s.check() {
			t.Fatalf("%s: Wirkung fehlt", s.label)
		}
	}
	if h := d.History(); h.Undo != "Mehrere" || !h.Can[0] || h.Can[1] {
		t.Errorf("History: %+v", h)
	}
	// Alles zurück, Schritt für Schritt bis zum Ausgangszustand.
	for i := len(steps) - 1; i >= 0; i-- {
		if _, err := d.Undo(); err != nil {
			t.Fatalf("Undo %s: %v", steps[i].label, err)
		}
		if !bytes.Equal(canon(d), states[i]) {
			t.Fatalf("Undo %s: Zustand weicht ab", steps[i].label)
		}
	}
	if !bytes.Equal(canon(d), start) || d.Dirty() {
		t.Fatalf("nach Undo nicht im Ausgangszustand (Dirty %v)", d.Dirty())
	}
	// Und wieder vor.
	for range steps {
		if _, err := d.Redo(); err != nil {
			t.Fatal(err)
		}
	}
	if d.Pkg.Template.ByID("gesamt").Get("color", "") != "#003366" || !d.Dirty() {
		t.Error("Redo stellt den Endzustand nicht her")
	}
	if _, err := d.Redo(); err == nil {
		t.Error("Redo über das Ende hinaus gelingt")
	}
}

// TestInvalidRollback erwartet, dass ungültige Befehle nichts ändern.
func TestInvalidRollback(t *testing.T) {
	d := muster(t)
	start := canon(d)
	bad := []Command{
		{Op: "setAttr", Ref: "adressfenster", Name: "layout", Value: "irgendwie"},
		{Op: "setAttr", Ref: "adressfenster", Name: "gibtsnicht", Value: "1"},
		{Op: "setAttr", Ref: "netto", Name: "id", Value: "brutto"},
		{Op: "remove", Ref: "seiten"},
		{Op: "insert", Parent: "inhalt", XML: `<text>ohne ID</text>`},
		{Op: "insert", Parent: "inhalt", XML: `<kaputt`},
		{Op: "move", Ref: "summen", Parent: "netto"},
		{Op: "batch", Cmds: []Command{{Op: "setAttr", Ref: "summen", Name: "w", Value: "1mm"}, {Op: "remove", Ref: "gibtsnicht"}}},
		{Op: "unbekannt"},
	}
	for _, c := range bad {
		if err := d.Apply("x", c); err == nil {
			t.Errorf("%+v: kein Fehler", c)
		}
		if !bytes.Equal(canon(d), start) {
			t.Fatalf("%+v: Vorlage verändert", c)
		}
	}
	if d.Dirty() || d.History().Can[0] {
		t.Error("ungültige Befehle stehen in der Historie")
	}
}

// TestSaveAfterUndo prüft den Geändert-Zustand über Speichern und Undo.
func TestSaveAfterUndo(t *testing.T) {
	d := muster(t)
	d.Apply("a", Command{Op: "setAttr", Ref: "summen", Name: "spaceBefore", Value: "8mm"})
	if err := d.SaveAs(filepath.Join(t.TempDir(), "x.zip")); err != nil {
		t.Fatal(err)
	}
	if d.Dirty() {
		t.Fatal("nach dem Speichern geändert")
	}
	d.Undo()
	if !d.Dirty() {
		t.Error("Undo hinter den gespeicherten Stand gilt nicht als Änderung")
	}
	d.Redo()
	if d.Dirty() {
		t.Error("Redo zum gespeicherten Stand gilt als Änderung")
	}
}

func TestParseFragmentErrors(t *testing.T) {
	if _, err := parseFragment(""); err == nil || !strings.Contains(err.Error(), "kein Element") {
		t.Errorf("leeres Fragment: %v", err)
	}
}

func BenchmarkApplyUndo(b *testing.B) {
	d := muster(b)
	for b.Loop() {
		if err := d.Apply("w", Command{Op: "setAttr", Ref: "adressfenster", Name: "w", Value: "90mm"}); err != nil {
			b.Fatal(err)
		}
		if _, err := d.Undo(); err != nil {
			b.Fatal(err)
		}
	}
}

func TestValueMessages(t *testing.T) {
	d := muster(t)
	cases := map[string]Command{
		"Länge":     {Op: "setAttr", Ref: "adressfenster", Name: "x", Value: "zwanzig"},
		"Farbe":     {Op: "setAttr", Ref: "summen", Name: "color", Value: "#12"},
		"einer der": {Op: "setAttr", Ref: "adressfenster", Name: "layout", Value: "schräg"},
		"Pflicht":   {Op: "setAttr", Ref: "adressfenster", Name: "layout", Value: ""},
	}
	for want, c := range cases {
		err := d.Apply("x", c)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%+v: %v, erwartet %q", c, err, want)
		}
	}
}
