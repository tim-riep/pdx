package pdx

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tim-riep/pdx/internal/testutil"
	"github.com/tim-riep/pdx/pdf/reader"
	"github.com/tim-riep/pdx/template"
	"github.com/tim-riep/pdx/template/pkg"
)

// structure liest den Strukturbaum einer Datei und verlangt, dass Baum,
// ParentTree und markierter Inhalt zusammenpassen.
func structure(t *testing.T, pdf []byte) []byte {
	t.Helper()
	f, err := reader.Open(pdf)
	if err != nil {
		t.Fatal(err)
	}
	st, err := f.ReadStructure()
	if err != nil {
		t.Fatal(err)
	}
	if st.Root == nil {
		t.Fatal("kein Strukturbaum")
	}
	for _, p := range st.Problems {
		t.Error(p)
	}
	var b bytes.Buffer
	st.Format(&b)
	return b.Bytes()
}

// TestCorpusTagged rendert jeden Fall als Tagged PDF. Der Strukturbaum muss
// widerspruchsfrei sein und dem Golden File entsprechen. Mit
// PDX_CONFORMANCE=1 prüft veraPDF, dass PDF/A-3b weiter erfüllt ist.
func TestCorpusTagged(t *testing.T) {
	files, err := filepath.Glob("testdata/fo/*.fo")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".fo")
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			res, err := RenderFile(f, Options{Profile: "pdfa-3b", CreationDate: corpusDate, Tagged: true})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := testutil.CheckStructure(res.PDF); err != nil {
				t.Fatal(err)
			}
			testutil.Golden(t, filepath.Join("testdata", "golden", "structure", name+".txt"), structure(t, res.PDF))
			again, _ := RenderFile(f, Options{Profile: "pdfa-3b", CreationDate: corpusDate, Tagged: true})
			if !bytes.Equal(res.PDF, again.PDF) {
				t.Error("Tagged PDF nicht reproduzierbar")
			}
			testutil.Conform(t, name+"-tagged.pdf", res.PDF, "pdfa-3b")
		})
	}
}

func TestTemplateStructure(t *testing.T) {
	p, data := loadMuster(t)
	res, err := RenderTemplate(p, data, TemplateOptions{Options: Options{CreationDate: templateDate, Tagged: true}})
	if err != nil {
		t.Fatal(err)
	}
	got := structure(t, res.PDF)
	testutil.Golden(t, "testdata/golden/structure/musterrechnung.txt", got)
	for _, want := range []string{"H1 \"Rechnung\"", "THead", "TH scope=Column", "TBody"} {
		if !bytes.Contains(got, []byte(want)) {
			t.Errorf("%q fehlt im Strukturbaum", want)
		}
	}
	testutil.Conform(t, "musterrechnung-tagged.pdf", res.PDF, DefaultProfile)
}

// TestTemplateAccessible rendert die Musterrechnung als PDF/A-3a mit
// PDF/UA-1. Fehlt der Alternativtext des GiroCodes, nennt der Befund die
// Vorlagen-ID.
func TestTemplateAccessible(t *testing.T) {
	p, data := loadMuster(t)
	opts := TemplateOptions{Options: Options{CreationDate: templateDate, Profile: "pdfa-3a,pdfua-1"}}
	res, err := RenderTemplate(p, data, opts)
	if err != nil {
		t.Fatal(err)
	}
	structure(t, res.PDF)
	for _, want := range []string{"pdfuaid:part>1<", "pdfaid:conformance>A<", "/DisplayDocTitle true"} {
		if !bytes.Contains(res.PDF, []byte(want)) {
			t.Errorf("%q fehlt", want)
		}
	}
	testutil.Conform(t, "musterrechnung-ua.pdf", res.PDF, "pdfa-3a,pdfua-1")

	// Ohne Alternativtext muss der Befund die ID des GiroCodes nennen.
	src := string(template.Write(p.Template))
	alt := ` altBind="concat('GiroCode zur Überweisung von ', inv:totals/inv:gross, ' Euro an ', inv:seller/inv:name)"`
	if !strings.Contains(src, alt) {
		t.Fatal("altBind des GiroCodes nicht gefunden")
	}
	parsed, err := template.Parse([]byte(strings.Replace(src, alt, "", 1)))
	if err != nil {
		t.Fatal(err)
	}
	q, err := pkg.New(parsed, p.Files)
	if err != nil {
		t.Fatal(err)
	}
	_, err = RenderTemplate(q, data, opts)
	var es template.Errors
	if !errors.As(err, &es) || len(es) != 1 || es[0].ID != "girocode" {
		t.Fatalf("Befund ohne Vorlagen-ID girocode: %v", err)
	}
}
