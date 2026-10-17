package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/tim-riep/pdx/pdf/render"
	"github.com/tim-riep/pdx/template"
	"github.com/tim-riep/pdx/template/pkg"
)

func input(t testing.TB) Input {
	t.Helper()
	p, err := pkg.ReadDir("../../../testdata/templates/musterrechnung")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := p.File("samples/rechnung.xml")
	return Input{Canon: template.Write(p.Template), Files: p.Files, Data: data}
}

func TestRun(t *testing.T) {
	in := input(t)
	p := New()
	res, err := p.Run(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Problems) > 0 || len(res.Pages) != 3 || !res.Compiled || len(res.Fonts) == 0 {
		t.Fatalf("erster Lauf: %+v", res)
	}
	b, ok := p.PageJSON(res.Pages[0].Hash)
	if !ok {
		t.Fatal("Seite 1 nicht im Cache")
	}
	var page Page
	if err := json.Unmarshal(b, &page); err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, e := range page.Els {
		ids[e.ID] = true
	}
	for _, id := range []string{"adressfenster", "positionen", "z-leistung"} {
		if !ids[id] {
			t.Errorf("Seite 1 ohne Bereich %q", id)
		}
	}
	if f, ok := p.Font(res.Fonts[0].Key); !ok || len(f) < 1000 {
		t.Error("Schrift nicht abrufbar")
	}

	// Zweiter Lauf ohne Änderung: kein Kompilieren, gleiche Seiten.
	again, err := p.Run(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if again.Compiled {
		t.Error("unveränderte Vorlage neu kompiliert")
	}
	for i := range res.Pages {
		if again.Pages[i].Hash != res.Pages[i].Hash {
			t.Errorf("Seite %d ohne Änderung mit neuem Hash", i+1)
		}
	}

	// Änderung nur auf der letzten Seite.
	in.Canon = bytes.Replace(in.Canon, []byte(`Nettobetrag:`), []byte(`Netto:`), 1)
	changed, err := p.Run(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if !changed.Compiled {
		t.Error("geänderte Vorlage nicht neu kompiliert")
	}
	for i := 0; i < 2; i++ {
		if changed.Pages[i].Hash != res.Pages[i].Hash {
			t.Errorf("Seite %d geändert, obwohl die Änderung auf Seite 3 liegt", i+1)
		}
	}
	if changed.Pages[2].Hash == res.Pages[2].Hash {
		t.Error("Seite 3 unverändert")
	}
}

func TestRunCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := New().Run(ctx, input(t)); err == nil {
		t.Error("abgebrochener Lauf ohne Fehler")
	}
}

func TestRunProblems(t *testing.T) {
	in := input(t)
	in.Canon = bytes.Replace(in.Canon, []byte(`format="betrag"`), []byte(`format="gibtsnicht"`), 1)
	res, err := New().Run(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Problems) == 0 || res.Problems[0].ID == "" {
		t.Errorf("Befunde: %+v", res.Problems)
	}
}

// BenchmarkDesignerEdit misst eine Änderung an der Vorlage bis zu den neuen
// Seiten: Kompilieren, Transformieren, Setzen, Übersetzen und Hashen.
func BenchmarkDesignerEdit(b *testing.B) {
	in := input(b)
	p := New()
	variants := [][]byte{in.Canon, bytes.Replace(in.Canon, []byte(`Nettobetrag:`), []byte(`Netto:`), 1)}
	i := 0
	for b.Loop() {
		in.Canon = variants[i%2]
		i++
		if _, err := p.Run(context.Background(), in); err != nil {
			b.Fatal(err)
		}
	}
}

func TestPDF(t *testing.T) {
	in := input(t)
	var tr render.Trace
	pdf, probs, err := New().PDF(context.Background(), in, time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC), &tr)
	if err != nil || len(probs) != 0 {
		t.Fatalf("PDF: %v %v", err, probs)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) || !bytes.Contains(pdf, []byte("/EmbeddedFiles")) {
		t.Fatalf("kein PDF mit Anhang (%d Bytes)", len(pdf))
	}
	if len(tr.Pages) == 0 || len(tr.Pages[0].Ops) == 0 {
		t.Fatalf("Spur leer: %+v", tr)
	}
	ids := map[string]bool{}
	for _, r := range tr.Pages[0].Ops {
		ids[r.ID] = true
	}
	if !ids["z-betrag-f"] {
		t.Fatalf("Spur ohne z-betrag-f: %v", ids)
	}
}
