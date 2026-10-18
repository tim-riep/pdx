package pdx

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tim-riep/pdx/internal/testutil"
	"github.com/tim-riep/pdx/pdf/reader"
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
