package check

import (
	"testing"

	"github.com/tim-riep/pdx/dataschema"
	"github.com/tim-riep/pdx/template/pkg"
)

func TestMusterrechnung(t *testing.T) {
	if fs := Path("../testdata/templates/musterrechnung"); len(fs) > 0 {
		t.Fatalf("Befunde: %v", fs)
	}
}

// TestBinding erwartet einen Befund mit der ID eines falsch gebundenen Felds.
func TestBinding(t *testing.T) {
	p, err := pkg.ReadDir("../testdata/templates/musterrechnung")
	if err != nil {
		t.Fatal(err)
	}
	p.Template.ByID("netto-wert").SetAttr("bind", "inv:totals/inv:gibtsnicht")
	fs := Package(p)
	if !dataschema.HasErrors(fs) || fs[0].ID != "netto-wert" {
		t.Fatalf("Befunde: %v", fs)
	}
}

func TestOpenError(t *testing.T) {
	fs := Path("gibt-es-nicht.zip")
	if len(fs) != 1 || fs[0].Level != dataschema.Error {
		t.Fatalf("Befunde: %v", fs)
	}
}
