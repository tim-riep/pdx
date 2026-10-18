package pdx

import (
	"strings"
	"testing"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/internal/testutil"
	"github.com/tim-riep/pdx/template/pkg"
)

const formDir = "testdata/templates/formular"

func loadForm(t testing.TB) (*Template, []byte) {
	t.Helper()
	p, err := pkg.ReadDir(formDir)
	if err != nil {
		t.Fatal(err)
	}
	tpl, err := CompileTemplate(p, "")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := p.File("samples/antrag.xml")
	return tpl, data
}

func widgets(doc *areatree.Document) []*areatree.Widget {
	var out []*areatree.Widget
	for _, p := range doc.Pages {
		areatree.Walk(p.Areas, func(a areatree.Area) bool {
			if w, ok := a.(*areatree.Widget); ok {
				out = append(out, w)
			}
			return true
		})
	}
	return out
}

// TestFormLayout setzt das Formular: je Feld ein Widget mit Wert, Optionen,
// Kontextpfad und den Glyphen des Erscheinungsbilds.
func TestFormLayout(t *testing.T) {
	tpl, data := loadForm(t)
	doc, err := tpl.Layout(data, TemplateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	testutil.Golden(t, "testdata/golden/formular.txt", areatree.Dump(doc))
	ws := widgets(doc)
	byName := map[string][]*areatree.Widget{}
	for _, w := range ws {
		byName[w.Field.Name] = append(byName[w.Field.Name], w)
	}
	if n := len(byName["anrede"]); n != 3 {
		t.Errorf("Optionsfelder %d, erwartet 3", n)
	}
	name := byName["name"]
	if len(name) != 1 || name[0].Field.Value != "Erika Musterfrau" || name[0].Field.MaxLength != 40 || len(name[0].Runs) != 1 {
		t.Fatalf("Textfeld %+v", name)
	}
	if w := name[0]; w.W < 250*areatree.Pt || w.H < 19*areatree.Pt || w.ID != "name" {
		t.Errorf("Größe oder ID %+v", w.Rect)
	}
	if l := byName["tarif"]; len(l) != 1 || l[0].Selected == nil || len(l[0].Runs) != 3 {
		t.Errorf("Liste %+v", l)
	}
	if c := byName["land"]; len(c) != 1 || runText(c[0].Runs[0]) != "Deutschland" {
		t.Errorf("Kombinationsfeld %+v", c)
	}
	if v := byName["vorname"]; len(v) != 2 || !strings.HasSuffix(v[1].Field.Context, "person[2]") || v[1].Field.Value != "Max" {
		t.Errorf("wiederholte Felder %+v", v)
	}
	if s := byName["unterschrift"]; len(s) != 1 || s[0].Field.Type != "signature" {
		t.Errorf("Signaturfeld %+v", s)
	}
}

func runText(r *areatree.GlyphRun) string {
	var b strings.Builder
	for _, g := range r.Glyphs {
		b.WriteString(g.Text)
	}
	return b.String()
}
