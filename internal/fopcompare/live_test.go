package fopcompare_test

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/tim-riep/pdx"
	"github.com/tim-riep/pdx/compile"
	"github.com/tim-riep/pdx/internal/fopcompare"
	"github.com/tim-riep/pdx/template/pkg"
)

// NotComparable sind Korpusfälle, die FOP nicht verarbeiten kann. Die Gründe
// stehen ausführlich in testdata/suites/DEVIATIONS.md.
var NotComparable = map[string]string{
	"barcodes": "FOP setzt pdx:barcode nicht und kann dafür keinen Area Tree schreiben",
}

// TestCorpusAgainstFOP vergleicht den Korpus mit FOP und gibt die Berichte
// aus. Er schlägt nur bei Werkzeugfehlern fehl. Ob ein Fall innerhalb der
// Toleranz liegen muss, entscheidet der Bericht von make fo-conformance.
func TestCorpusAgainstFOP(t *testing.T) {
	if !fopcompare.Enabled() {
		t.Skip("PDX_FOP=1 setzen, um mit Apache FOP zu vergleichen")
	}
	ctx := context.Background()
	files, _ := filepath.Glob("../../testdata/fo/*.fo")
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".fo")
		t.Run(name, func(t *testing.T) {
			if why, ok := NotComparable[name]; ok {
				t.Skip("nicht mit FOP vergleichbar: " + why)
			}
			res, err := pdx.RenderFile(f, pdx.Options{CreationDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
			if err != nil {
				t.Fatal(err)
			}
			fop, err := fopcompare.RunFOP(ctx, f)
			if err != nil {
				t.Fatal(err)
			}
			rep, err := fopcompare.Compare(ctx, name, res.PDF, res.Tree, fop, fopcompare.Default)
			if err != nil {
				t.Fatal(err)
			}
			t.Log("\n" + rep.String())
		})
	}
}

// TestMusterrechnungAgainstFOP setzt die Musterrechnung aus der Vorlage und
// vergleicht das FO mit FOP. Der GiroCode wird für beide entfernt, weil FOP
// pdx:barcode nicht setzen kann (siehe NotComparable).
func TestMusterrechnungAgainstFOP(t *testing.T) {
	if !fopcompare.Enabled() {
		t.Skip("PDX_FOP=1 setzen, um mit Apache FOP zu vergleichen")
	}
	ctx := context.Background()
	p, err := pkg.ReadDir("../../testdata/templates/musterrechnung")
	if err != nil {
		t.Fatal(err)
	}
	res, err := compile.Compile(p.Template, compile.Options{})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := p.File("samples/rechnung.xml")
	fo, err := res.Transform(data)
	if err != nil {
		t.Fatal(err)
	}
	fo = regexp.MustCompile(`<fo:instream-foreign-object[^>]*>.*?</fo:instream-foreign-object>`).ReplaceAll(fo, nil)
	root, _ := fopcompare.RepoRoot()
	dir, err := os.MkdirTemp(filepath.Join(root, "out"), "fop-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	file := filepath.Join(dir, "musterrechnung.fo")
	if err := os.WriteFile(file, fo, 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := pdx.RenderFile(file, pdx.Options{CreationDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	fop, err := fopcompare.RunFOPRelaxed(ctx, file)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := fopcompare.Compare(ctx, "musterrechnung", out.PDF, out.Tree, fop, fopcompare.Default)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("\n" + rep.String())
}
