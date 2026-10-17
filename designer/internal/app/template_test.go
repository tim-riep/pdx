package app

import (
	"maps"
	"testing"

	"github.com/tim-riep/pdx/designer/internal/model"
)

// applyPatch wendet einen Patch auf eine Projektion an, wie die Oberfläche.
func applyPatch(s model.Snapshot, p model.Patch) model.Snapshot {
	out := model.Snapshot{Rev: p.Rev, Root: p.Root, Nodes: maps.Clone(s.Nodes)}
	for _, r := range p.Removed {
		delete(out.Nodes, r)
	}
	for _, n := range p.Set {
		out.Nodes[n.Ref] = n
	}
	return out
}

func equalSnap(t *testing.T, a, b model.Snapshot) {
	t.Helper()
	if a.Root != b.Root || len(a.Nodes) != len(b.Nodes) {
		t.Fatalf("Projektionen verschieden: %d und %d Knoten", len(a.Nodes), len(b.Nodes))
	}
	for ref, n := range a.Nodes {
		m, ok := b.Nodes[ref]
		if !ok || n.Name != m.Name || n.Parent != m.Parent || !maps.Equal(n.Attrs, m.Attrs) || len(n.Children) != len(m.Children) {
			t.Fatalf("Knoten %s verschieden", ref)
		}
	}
}

func TestApplyPatches(t *testing.T) {
	rec := &recorder{}
	c := NewCore(rec, nil, t.TempDir())
	if _, err := NewProjectService(c).Open(muster); err != nil {
		t.Fatal(err)
	}
	ts := NewTemplateService(c)
	snap, err := ts.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	steps := []func() (Result, error){
		func() (Result, error) {
			return ts.Apply("Breite", model.Command{Op: "setAttr", Ref: "adressfenster", Name: "w", Value: "90mm"})
		},
		func() (Result, error) {
			return ts.Apply("Text", model.Command{Op: "insert", Parent: "summen", Index: 0, XML: `<text id="hinweis">Zahlbar sofort</text>`})
		},
		func() (Result, error) { return ts.Apply("Weg", model.Command{Op: "remove", Ref: "steuer"}) },
		ts.Undo, ts.Undo, ts.Redo,
	}
	for i, step := range steps {
		res, err := step()
		if err != nil {
			t.Fatalf("Schritt %d: %v", i, err)
		}
		p := rec.wait(t, EventTemplateChanged, func(d any) bool { return d.(model.Patch).Rev == res.Rev }).(model.Patch)
		if p.From != snap.Rev {
			t.Fatalf("Schritt %d: Patch von %d, Projektion bei %d", i, p.From, snap.Rev)
		}
		snap = applyPatch(snap, p)
		full, _ := NewTemplateService(c).Snapshot()
		equalSnap(t, snap, full)
	}
	if h := ts.History(); h.Undo != "Text" || h.Redo != "Weg" {
		t.Errorf("History: %+v", h)
	}
	res, err := ts.Apply("Neu", model.Command{Op: "insert", Parent: "inhalt", Index: -1, XML: `<text id="ende">Ende</text>`})
	if err != nil || res.Select != "ende" {
		t.Errorf("Select nach Einfügen: %q, %v", res.Select, err)
	}
	if _, err := ts.Apply("Falsch", model.Command{Op: "remove", Ref: "gibtsnicht"}); err == nil {
		t.Error("ungültiger Befehl ohne Fehler")
	}
	if !NewProjectService(c).State().Dirty {
		t.Error("nach Befehlen nicht geändert")
	}
}
