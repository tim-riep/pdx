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

// NotComparable lists corpus cases that FOP cannot process. The reasons are
// described in detail in testdata/suites/DEVIATIONS.md.
var NotComparable = map[string]string{
	"barcodes": "FOP setzt pdx:barcode nicht und kann dafür keinen Area Tree schreiben",
}

// TestCorpusAgainstFOP compares the corpus with FOP and prints the reports.
// It fails only on tool errors. Whether a case must lie within the
// tolerance is decided by the report of make fo-conformance.
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

// TestMusterrechnungAgainstFOP typesets the sample invoice from the template
// and compares the FO with FOP. The GiroCode is removed for both, because FOP
// cannot typeset pdx:barcode (see NotComparable).
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
