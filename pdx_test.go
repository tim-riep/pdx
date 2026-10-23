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

// TestCorpus renders every case in testdata/fo and compares area tree and
// PDF with the golden files. With PDX_CONFORMANCE=1 veraPDF checks every file.
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

// matrixProfiles are the profiles of the conformance matrix besides pdfa-3b,
// which TestCorpus checks with golden files. It grows with every new profile.
var matrixProfiles = []string{"pdfa-2b", "pdfa-2u", "pdfa-3u", "pdfa-3a,pdfua-1", "pdfa-4", "pdfa-4f", "pdfa-4,pdfua-2"}

// inaccessible are corpus cases that deliberately do not satisfy PDF/UA
// (figures without alternate text, tables without header cells). The
// pre-check must reject them with a finding.
var inaccessible = map[string]bool{"barcodes": true, "bilder": true, "rechnung": true, "tabellen": true}

// TestCorpusProfiles renders every case in every profile of the matrix. With
// PDX_CONFORMANCE=1 veraPDF checks every claimed standard. Attachments that
// a profile does not allow (under PDF/A-2 and PDF/A-4 everything except
// PDF/A), and PDF/A-4f without an attachment must be rejected with the clause.
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
					// Attachments the profile does not allow (PDF/A-2 and
					// PDF/A-4 only PDF/A), and 4f without an attachment, are
					// rejected by the writer.
					if strings.Contains(err.Error(), "ISO 19005-2 6.8") || strings.Contains(err.Error(), "ISO 19005-4 6.9") {
						return
					}
					var ce render.CheckErrors
					if inaccessible[name] && errors.As(err, &ce) {
						return // not accessible, rejected as required
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

// TestReproducible renders every case twice and requires identical bytes.
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
