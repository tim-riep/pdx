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

// structure reads the structure tree of a file and requires that tree,
// ParentTree and marked content match.
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

// TestCorpusTagged renders every case as Tagged PDF. The structure tree must
// be consistent and match the golden file. With
// PDX_CONFORMANCE=1 veraPDF checks that PDF/A-3b is still satisfied.
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

// TestTemplateAccessible renders the sample invoice as PDF/A-3a with
// PDF/UA-1. If the alternate text of the GiroCode is missing, the finding
// names the template ID.
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

	// Without alternate text the finding must name the ID of the GiroCode.
	src := string(template.Write(p.Template))
	alt := ` altBind="concat('GiroCode zur Überweisung von ', format-number(xs:decimal(inv:totals/inv:gross), '#.##0,00', 'fmt-de'), ' Euro an ', inv:seller/inv:name)"`
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

// accessibleProfiles are the combinations for accessible documents.
var accessibleProfiles = []string{"pdfa-3a,pdfua-1", "pdfa-4,pdfua-2"}

// TestAccessible renders the cases in testdata/accessible under PDF/UA-1
// and PDF/UA-2. The structure tree must be consistent and match the golden
// file, with PDX_CONFORMANCE=1 veraPDF checks every standard.
func TestAccessible(t *testing.T) {
	files, err := filepath.Glob("testdata/accessible/*.fo")
	if err != nil || len(files) == 0 {
		t.Fatalf("keine Fälle: %v", err)
	}
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".fo")
		for _, prof := range accessibleProfiles {
			t.Run(name+"/"+prof, func(t *testing.T) {
				t.Parallel()
				res, err := RenderFile(f, Options{Profile: prof, CreationDate: corpusDate})
				if err != nil {
					t.Fatal(err)
				}
				ua := prof[strings.LastIndex(prof, "-")+1:]
				testutil.Golden(t, filepath.Join("testdata", "golden", "structure", name+"-ua"+ua+".txt"), structure(t, res.PDF))
				testutil.Conform(t, name+"-ua"+ua+".pdf", res.PDF, prof)
			})
		}
	}
}

// TestTemplateUA2 renders the sample invoice as PDF/A-4f with PDF/UA-2:
// structure elements in the PDF 2.0 namespace, attachment with description.
func TestTemplateUA2(t *testing.T) {
	p, data := loadMuster(t)
	res, err := RenderTemplate(p, data, TemplateOptions{Options: Options{CreationDate: templateDate, Profile: "pdfa-4f,pdfua-2"}})
	if err != nil {
		t.Fatal(err)
	}
	structure(t, res.PDF)
	for _, want := range []string{"%PDF-2.0", "<pdfuaid:rev>2024</pdfuaid:rev>", "/Namespaces [", "(http://iso.org/pdf2/ssn)"} {
		if !bytes.Contains(res.PDF, []byte(want)) {
			t.Errorf("%q fehlt", want)
		}
	}
	testutil.Conform(t, "musterrechnung-ua2.pdf", res.PDF, "pdfa-4f,pdfua-2")
}
