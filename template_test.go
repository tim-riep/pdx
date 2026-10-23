package pdx

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/font"
	"github.com/tim-riep/pdx/internal/testutil"
	"github.com/tim-riep/pdx/pdf/profile"
	"github.com/tim-riep/pdx/pdf/render"
	"github.com/tim-riep/pdx/template"
	"github.com/tim-riep/pdx/template/pkg"
)

const musterDir = "testdata/templates/musterrechnung"

func loadMuster(t testing.TB) (*pkg.Package, []byte) {
	t.Helper()
	p, err := pkg.ReadDir(musterDir)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := p.File("samples/rechnung.xml")
	return p, data
}

var templateDate = time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC)

func TestRenderTemplate(t *testing.T) {
	p, data := loadMuster(t)
	res, err := RenderTemplate(p, data, TemplateOptions{Options: Options{CreationDate: templateDate}, Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(res.Tree.Pages); n < 3 {
		t.Errorf("%d Seiten, erwartet mindestens 3", n)
	}
	if len(res.Tree.Attachments) != 1 || res.Tree.Attachments[0].Name != "rechnung.xml" ||
		!bytes.Equal(res.Tree.Attachments[0].Data, data) {
		t.Errorf("Datenanhang fehlt oder ist falsch: %d Anhänge", len(res.Tree.Attachments))
	}
	if _, err := testutil.CheckStructure(res.PDF); err != nil {
		t.Fatal(err)
	}
	testutil.Golden(t, "testdata/golden/musterrechnung.pdf", res.PDF)
	if d := os.Getenv("PDX_OUT"); d != "" {
		os.WriteFile(d+"/musterrechnung.pdf", res.PDF, 0o644)
	}
	testutil.Conform(t, "musterrechnung.pdf", res.PDF, DefaultProfile)

	// The same input yields the same file.
	again, err := RenderTemplate(p, data, TemplateOptions{Options: Options{CreationDate: templateDate}})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(again.PDF, res.PDF) {
		t.Error("zwei Läufe ergeben verschiedene PDF-Dateien")
	}
}

func TestRenderTemplateStrict(t *testing.T) {
	p, data := loadMuster(t)
	bad := bytes.Replace(data, []byte("<currency>EUR</currency>"), []byte("<currency>Euro</currency>"), 1)
	if bytes.Equal(bad, data) {
		t.Fatal("Beispieldaten ohne <currency>EUR</currency>")
	}
	tpl, err := CompileTemplate(p, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tpl.Render(bad, TemplateOptions{Options: Options{CreationDate: templateDate}, Strict: true}); err == nil ||
		!strings.Contains(err.Error(), "invoice.xsd") {
		t.Fatalf("strenge Prüfung meldet %v", err)
	}
	// Without strict validation it is typeset, the currency appears nowhere in the layout.
	if _, err := tpl.Render(bad, TemplateOptions{Options: Options{CreationDate: templateDate}}); err != nil {
		t.Fatal(err)
	}
}

func TestRenderTemplateLayoutErrorID(t *testing.T) {
	p, data := loadMuster(t)
	bad := bytes.Replace(data, []byte("<iban>"), []byte("<iban>XX"), 1)
	if bytes.Equal(bad, data) {
		t.Fatal("Beispieldaten ohne IBAN")
	}
	_, err := RenderTemplate(p, bad, TemplateOptions{Options: Options{CreationDate: templateDate}})
	var te *template.Error
	if !errors.As(err, &te) || te.ID != "girocode" {
		t.Fatalf("Fehler %v, erwartet ID girocode", err)
	}
}

func BenchmarkCompileInvoice(b *testing.B) {
	p, _ := loadMuster(b)
	for b.Loop() {
		if _, err := CompileTemplate(p, ""); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRenderInvoice(b *testing.B) {
	p, data := loadMuster(b)
	tpl, err := CompileTemplate(p, "")
	if err != nil {
		b.Fatal(err)
	}
	opts := TemplateOptions{Options: Options{CreationDate: templateDate}}
	for b.Loop() {
		if _, err := tpl.Render(data, opts); err != nil {
			b.Fatal(err)
		}
	}
}

// TestTemplateIDsInAreaTree checks that the areas carry the IDs of their
// template elements, including fields inside a text.
func TestTemplateIDsInAreaTree(t *testing.T) {
	p, data := loadMuster(t)
	res, err := RenderTemplate(p, data, TemplateOptions{Options: Options{CreationDate: templateDate}})
	if err != nil {
		t.Fatal(err)
	}
	containers := map[string]bool{}
	runs := map[string]string{} // run ID to its text
	paths := map[string]bool{}
	var walk func(as []areatree.Area)
	walk = func(as []areatree.Area) {
		for _, a := range as {
			switch v := a.(type) {
			case *areatree.Container:
				containers[v.ID] = true
				walk(v.Children)
			case *areatree.GlyphRun:
				if v.ID != "" {
					var s strings.Builder
					for _, g := range v.Glyphs {
						s.WriteString(g.Text)
					}
					runs[v.ID] += s.String()
				}
			case *areatree.Path:
				paths[v.ID] = true
			}
		}
	}
	for _, pg := range res.Tree.Pages {
		walk(pg.Areas)
	}
	for _, id := range []string{"positionen", "z-leistung", "adressfenster", "summen", "netto"} {
		if !containers[id] {
			t.Errorf("kein Container mit ID %q", id)
		}
	}
	if got := runs["netto-wert"]; !strings.ContainsAny(got, "0123456789") {
		t.Errorf("Feld netto-wert: Run mit Text %q, erwartet einen Betrag", got)
	}
	if _, ok := runs["netto"]; ok {
		t.Error("Runs des Absatzes netto tragen eine eigene ID, erwartet leer")
	}
	if !paths["girocode"] {
		t.Error("Barcode ohne ID girocode")
	}
}

// TestTemplateLayout checks that Layout without PDF yields the same area tree
// as Render and that a cancelled context stops the typesetting.
func TestTemplateLayout(t *testing.T) {
	p, data := loadMuster(t)
	tpl, err := CompileTemplate(p, "")
	if err != nil {
		t.Fatal(err)
	}
	reg := font.NewRegistry()
	res, err := tpl.Render(data, TemplateOptions{Options: Options{CreationDate: templateDate, Fonts: reg}})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := tpl.Layout(data, TemplateOptions{Options: Options{Fonts: reg}})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(areatree.Dump(doc), areatree.Dump(res.Tree)) {
		t.Error("Layout und Render liefern verschiedene Area Trees")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := tpl.Layout(data, TemplateOptions{Options: Options{Fonts: reg, Context: ctx}}); !errors.Is(err, context.Canceled) {
		t.Errorf("abgebrochener Kontext: Fehler %v, erwartet context.Canceled", err)
	}
}

// BenchmarkLayoutInvoice measures transformation and layout without PDF with
// a shared font registry, as the designer uses it for the preview.
func BenchmarkLayoutInvoice(b *testing.B) {
	p, data := loadMuster(b)
	tpl, err := CompileTemplate(p, "")
	if err != nil {
		b.Fatal(err)
	}
	opts := TemplateOptions{Options: Options{Fonts: font.NewRegistry()}}
	for b.Loop() {
		if _, err := tpl.Layout(data, opts); err != nil {
			b.Fatal(err)
		}
	}
}

// TestPackageFonts checks that fonts under assets/ of a package are used
// without further configuration and their bytes are attached to the face.
func TestPackageFonts(t *testing.T) {
	p, data := loadMuster(t)
	ttf, err := os.ReadFile("testdata/fonts/source-serif-4/SourceSerif4-Regular.otf")
	if err != nil {
		t.Fatal(err)
	}
	p.Files["assets/SourceSerif4-Regular.otf"] = ttf
	p.Template.ByID("st-basis").SetAttr("fontFamily", "Source Serif 4")
	tpl, err := CompileTemplate(p, "")
	if err != nil {
		t.Fatal(err)
	}
	reg := font.NewRegistry()
	for range 2 { // a second run on the same registry does not register twice
		doc, err := tpl.Layout(data, TemplateOptions{Options: Options{Fonts: reg}})
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, f := range doc.Fonts {
			if face, ok := f.Source.(*font.Face); ok && bytes.Equal(face.Data(), ttf) {
				found = true
			}
		}
		if !found {
			t.Fatal("Schrift aus assets/ wird nicht genutzt")
		}
	}
}

// TestRenderTrace checks the operator trace: same output with and
// without trace, areas with template IDs, IDAt finds the innermost area.
func TestRenderTrace(t *testing.T) {
	p, data := loadMuster(t)
	tpl, err := CompileTemplate(p, "")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := tpl.Layout(data, TemplateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	prof, _ := profile.Lookup(DefaultProfile)
	opts := render.Options{Profile: prof, CreationDate: templateDate, Producer: "test"}
	plain, err := render.Render(doc, opts)
	if err != nil {
		t.Fatal(err)
	}
	tr := &render.Trace{}
	opts.Trace = tr
	traced, err := render.Render(doc, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(plain, traced) {
		t.Error("die Spur verändert die Ausgabe")
	}
	if len(tr.Pages) != len(doc.Pages) {
		t.Fatalf("Spur mit %d Seiten, Dokument hat %d", len(tr.Pages), len(doc.Pages))
	}
	seen := map[string]bool{}
	for pi, pg := range tr.Pages {
		for _, r := range pg.Ops {
			seen[r.ID] = true
			if r.ID == "netto-wert" || r.ID == "girocode" {
				if got := tr.IDAt(pi, r.First); got != r.ID {
					t.Errorf("IDAt(%d, %d) = %q, erwartet %q", pi, r.First, got, r.ID)
				}
			}
		}
	}
	for _, id := range []string{"positionen", "z-betrag", "netto-wert", "girocode"} {
		if !seen[id] {
			t.Errorf("kein Operatorbereich für %q", id)
		}
	}
}

// Without a value in the call the profile of the template applies.
func TestTemplateProfile(t *testing.T) {
	p, err := pkg.ReadDir("testdata/templates/musterrechnung")
	if err != nil {
		t.Fatal(err)
	}
	tpl, err := CompileTemplate(p, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := tpl.Profile(); got != DefaultProfile {
		t.Fatalf("Vorgabe %q", got)
	}
	p.Template.Root.SetAttr("profile", "pdfa-3u")
	if got := tpl.Profile(); got != "pdfa-3u" {
		t.Fatalf("Profil der Vorlage %q", got)
	}
	data, _ := p.File("samples/rechnung.xml")
	res, err := tpl.Render(data, TemplateOptions{Options: Options{CreationDate: corpusDate}})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(res.PDF, []byte("<pdfaid:conformance>U</pdfaid:conformance>")) {
		t.Fatal("Profil der Vorlage nicht angewandt")
	}
}

// Parallel runs with a shared template and font registry yield
// the same bytes as a single run (render service, shaper pool).
func TestParallelRenderDeterministic(t *testing.T) {
	p, data := loadMuster(t)
	tpl, err := CompileTemplate(p, "")
	if err != nil {
		t.Fatal(err)
	}
	reg := font.NewRegistry()
	opts := TemplateOptions{Options: Options{Profile: "pdfa-3b", CreationDate: templateDate, Fonts: reg}}
	want, err := tpl.Render(data, opts)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan string, 16)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 2 {
				res, err := tpl.Render(data, opts)
				if err != nil {
					errs <- err.Error()
					return
				}
				if !bytes.Equal(res.PDF, want.PDF) {
					errs <- "andere Bytes als im einzelnen Lauf"
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
}

func BenchmarkParallelRender(b *testing.B) {
	p, data := loadMuster(b)
	tpl, err := CompileTemplate(p, "")
	if err != nil {
		b.Fatal(err)
	}
	opts := TemplateOptions{Options: Options{Profile: "pdfa-3b", CreationDate: templateDate, Fonts: font.NewRegistry()}}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := tpl.Render(data, opts); err != nil {
				b.Fatal(err)
			}
		}
	})
}
