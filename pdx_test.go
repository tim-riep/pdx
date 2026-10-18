package pdx

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/internal/testutil"
	"github.com/tim-riep/pdx/pdf/render"
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

// matrixProfiles sind die Profile der Konformitätsmatrix neben pdfa-3b, das
// TestCorpus mit Golden Files prüft. Sie wächst mit jedem neuen Profil.
var matrixProfiles = []string{"pdfa-2b", "pdfa-2u", "pdfa-3u", "pdfa-3a,pdfua-1"}

// inaccessible sind Korpusfälle, die PDF/UA bewusst nicht erfüllen
// (Abbildungen ohne Alternativtext, Tabellen ohne Kopfzellen). Die
// Vorprüfung muss sie mit Befund ablehnen.
var inaccessible = map[string]bool{"barcodes": true, "bilder": true, "rechnung": true, "tabellen": true}

// TestCorpusProfiles rendert jeden Fall in jedem Profil der Matrix. Mit
// PDX_CONFORMANCE=1 prüft veraPDF jeden beanspruchten Standard. Anhänge, die
// ein Profil nicht erlaubt (unter PDF/A-2 alles außer PDF/A), müssen mit der
// Klausel abgelehnt werden.
func TestCorpusProfiles(t *testing.T) {
	files, err := filepath.Glob("testdata/fo/*.fo")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".fo")
		for _, prof := range matrixProfiles {
			t.Run(name+"/"+prof, func(t *testing.T) {
				t.Parallel()
				res, err := RenderFile(f, Options{Profile: prof, CreationDate: corpusDate})
				if err != nil {
					if strings.HasPrefix(prof, "pdfa-2") && strings.Contains(err.Error(), "ISO 19005-2 6.8") {
						return // Anhang ohne PDF/A, wie verlangt abgelehnt
					}
					var ce render.CheckErrors
					if inaccessible[name] && errors.As(err, &ce) {
						return // nicht barrierefrei, wie verlangt abgelehnt
					}
					t.Fatal(err)
				}
				if inaccessible[name] && strings.Contains(prof, "pdfua") {
					t.Fatal("nicht barrierefreier Fall wurde nicht abgelehnt")
				}
				if _, err := testutil.CheckStructure(res.PDF); err != nil {
					t.Fatal(err)
				}
				testutil.Conform(t, name+"-"+prof+".pdf", res.PDF, prof)
			})
		}
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
