package fopcompare_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tim-riep/pdx"
	"github.com/tim-riep/pdx/internal/fopcompare"
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
