package app

import (
	"maps"
	"strings"
	"testing"

	"github.com/tim-riep/pdx/designer/internal/model"
	"github.com/tim-riep/pdx/template"
)

// waitReport wartet auf einen Bericht zur Revision rev.
func waitReport(t *testing.T, r *recorder, rev uint64) Report {
	t.Helper()
	return r.wait(t, EventProblems, func(d any) bool { rep, ok := d.(Report); return ok && rep.Rev == rev }).(Report)
}

func TestValidation(t *testing.T) {
	rec := &recorder{}
	c := NewCore(rec, nil, t.TempDir())
	if _, err := NewProjectService(c).Open(muster); err != nil {
		t.Fatal(err)
	}
	rep := waitReport(t, rec, 0)
	for _, p := range rep.Problems {
		if p.Level == "Fehler" {
			t.Fatalf("Musterrechnung mit Fehler: %+v", p)
		}
	}

	// Falsche Bindung: Befund mit Vorlagen-ID.
	ts := NewTemplateService(c)
	res, err := ts.Apply("Bindung", model.Command{Op: "setAttr", Ref: "z-betrag-f", Name: "bind", Value: "inv:betrag"})
	if err != nil {
		t.Fatal(err)
	}
	rep = waitReport(t, rec, res.Rev)
	var found *Problem
	for i, p := range rep.Problems {
		if p.Ref == "z-betrag-f" {
			found = &rep.Problems[i]
		}
	}
	if found == nil || found.Source != "vorlage" || !strings.Contains(found.Msg, "betrag") {
		t.Fatalf("Befund zur Bindung fehlt: %+v", rep.Problems)
	}
	if got := NewValidationService(c).Problems(); got.Rev != res.Rev || len(got.Problems) != len(rep.Problems) {
		t.Fatalf("Problems() %+v", got)
	}

	// Präsenz mit unbekanntem Knoten: Hinweis am Element.
	res, err = ts.Apply("Präsenz", model.Command{Op: "setAttr", Ref: "titel", Name: "presence", Value: "exists(inv:gibtsnicht)"})
	if err != nil {
		t.Fatal(err)
	}
	rep = waitReport(t, rec, res.Rev)
	found = nil
	for i, p := range rep.Problems {
		if p.Ref == "titel" {
			found = &rep.Problems[i]
		}
	}
	if found == nil || found.Level != "Hinweis" || found.Msg != "Präsenz: inv:gibtsnicht gibt es unter inv:invoice nicht" {
		t.Fatalf("Hinweis zur Präsenz fehlt: %+v", rep.Problems)
	}
	if _, err := ts.Undo(); err != nil {
		t.Fatal(err)
	}

	// Undo: der alte Stand ist wieder ohne Befund.
	res, err = ts.Undo()
	if err != nil {
		t.Fatal(err)
	}
	rep = waitReport(t, rec, res.Rev)
	for _, p := range rep.Problems {
		if p.Ref == "z-betrag-f" {
			t.Fatalf("Befund nach Undo: %+v", p)
		}
	}
}

func TestProblemsOfSamples(t *testing.T) {
	c := NewCore(nil, nil, t.TempDir())
	if _, err := NewProjectService(c).Open(muster); err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	files := maps.Clone(c.doc.Pkg.Files)
	canon := template.Write(c.doc.Pkg.Template)
	c.mu.Unlock()
	files["samples/kaputt.xml"] = []byte(`<inv:invoice xmlns:inv="https://pdx.example/ns/invoice/1"><inv:falsch/></inv:invoice>`)
	ps := runChecks(canon, files)
	var hit bool
	for _, p := range ps {
		if p.Source == "beispiel" && p.File == "samples/kaputt.xml" && p.Ref == "" {
			hit = true
		}
	}
	if !hit {
		t.Fatalf("Befund zu Beispieldaten fehlt: %+v", ps)
	}
}
