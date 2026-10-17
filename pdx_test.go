package pdx

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/internal/testutil"
)

var corpusDate = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// TestCorpus rendert jeden Fall in testdata/fo und vergleicht Area Tree und
// PDF mit den Golden Files. Mit PDX_CONFORMANCE=1 prüft veraPDF jede Datei.
func TestCorpus(t *testing.T) {
	files, err := filepath.Glob("testdata/fo/*.fo")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 7 {
		t.Fatalf("Korpus unvollständig: %d Fälle", len(files))
	}
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".fo")
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			res, err := RenderFile(f, Options{Profile: "pdfa-3b", CreationDate: corpusDate})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := testutil.CheckStructure(res.PDF); err != nil {
				t.Fatal(err)
			}
			testutil.Golden(t, filepath.Join("testdata", "golden", name+".txt"), areatree.Dump(res.Tree))
			testutil.Golden(t, filepath.Join("testdata", "golden", name+".pdf"), res.PDF)
			testutil.Conform(t, name+".pdf", res.PDF, "pdfa-3b")
		})
	}
}

// TestReproducible rendert jeden Fall zweimal und verlangt identische Bytes.
func TestReproducible(t *testing.T) {
	files, _ := filepath.Glob("testdata/fo/*.fo")
	for _, f := range files {
		a, err := RenderFile(f, Options{CreationDate: corpusDate})
		if err != nil {
			t.Fatal(err)
		}
		b, err := RenderFile(f, Options{CreationDate: corpusDate})
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(a.PDF, b.PDF) {
			t.Errorf("%s: zwei Läufe ergeben unterschiedliche Bytes", f)
		}
		c, _ := RenderFile(f, Options{CreationDate: corpusDate.Add(time.Second)})
		if bytes.Equal(a.PDF, c.PDF) {
			t.Errorf("%s: anderes Erstellungsdatum ändert die Datei nicht", f)
		}
	}
}

func TestRenderErrors(t *testing.T) {
	cases := map[string]string{
		`<fo:root xmlns:fo="http://www.w3.org/1999/XSL/Format"><fo:layout-master-set><fo:simple-page-master master-name="a"><fo:region-body/></fo:simple-page-master></fo:layout-master-set><fo:page-sequence master-reference="b"><fo:flow flow-name="xsl-region-body"/></fo:page-sequence></fo:root>`:                                                                                    "Seitenvorlage \"b\" nicht gefunden",
		`<fo:root xmlns:fo="http://www.w3.org/1999/XSL/Format"><fo:layout-master-set><fo:simple-page-master master-name="a"><fo:region-body/></fo:simple-page-master></fo:layout-master-set><fo:page-sequence master-reference="a"><fo:flow flow-name="xsl-region-body"><fo:block><fo:external-graphic src="http://example.org/a.png"/></fo:block></fo:flow></fo:page-sequence></fo:root>`: "nur lokale Dateien",
	}
	for in, want := range cases {
		_, err := Render(strings.NewReader(in), Options{CreationDate: corpusDate})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("erwartet %q, erhalten %v", want, err)
		}
	}
}
