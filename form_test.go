package pdx

import (
	"strings"
	"testing"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/internal/testutil"
	"github.com/tim-riep/pdx/pdf/reader"
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

// formProfiles sind die Profile ohne Barrierefreiheit, in denen das
// Formular gelten muss. PDF/A-4f verlangt einen Anhang und fehlt deshalb.
var formProfiles = []string{"pdfa-3b", "pdfa-2b", "pdfa-2u", "pdfa-3u", "pdfa-4", "pdf-1.7"}

// TestFormRender schreibt das Formular in jedem Profil: AcroForm mit allen
// Feldern, je Widget ein Erscheinungsbild, nie NeedAppearances. Mit
// PDX_CONFORMANCE=1 prüft veraPDF jede Datei.
func TestFormRender(t *testing.T) {
	tpl, data := loadForm(t)
	for _, prof := range formProfiles {
		t.Run(prof, func(t *testing.T) {
			res, err := tpl.Render(data, TemplateOptions{Options: Options{CreationDate: templateDate, Profile: prof}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := testutil.CheckStructure(res.PDF); err != nil {
				t.Fatal(err)
			}
			f, err := reader.Open(res.PDF)
			if err != nil {
				t.Fatal(err)
			}
			acro := f.Dict(f.Root()["AcroForm"])
			if acro == nil || acro["NeedAppearances"] != nil || acro["XFA"] != nil {
				t.Fatalf("AcroForm %v", acro)
			}
			fields := f.Array(acro["Fields"])
			// anrede (Gruppe), name, email, land, tarif, newsletter,
			// bemerkung, zweimal vorname, unterschrift
			if len(fields) != 10 {
				t.Errorf("%d Felder, erwartet 10", len(fields))
			}
			types := map[string]int{}
			var visit func(o reader.Object)
			visit = func(o reader.Object) {
				d := f.Dict(o)
				if kids := f.Array(d["Kids"]); len(kids) > 0 {
					for _, k := range kids {
						visit(k)
					}
				}
				if d["Subtype"] == reader.Name("Widget") {
					ap := f.Dict(d["AP"])
					if ap == nil || ap["N"] == nil || ap["D"] != nil || ap["R"] != nil {
						t.Errorf("Widget ohne gültiges AP: %v", d)
					}
					if d["A"] != nil || d["AA"] != nil {
						t.Errorf("Widget mit Aktion: %v", d)
					}
					ft := d["FT"]
					if ft == nil {
						ft = f.Dict(d["Parent"])["FT"]
					}
					types[string(ft.(reader.Name))]++
				}
			}
			for _, fl := range fields {
				visit(fl)
			}
			if types["Btn"] != 4 || types["Tx"] != 5 || types["Ch"] != 2 || types["Sig"] != 1 {
				t.Errorf("Feldtypen %v", types)
			}
			if prof != "pdf-1.7" {
				testutil.Conform(t, "formular-"+prof+".pdf", res.PDF, prof)
			}
		})
	}
	// PDF/A-4f ohne Anhang wird abgelehnt.
	if _, err := tpl.Render(data, TemplateOptions{Options: Options{CreationDate: templateDate, Profile: "pdfa-4f"}}); err == nil {
		t.Error("PDF/A-4f ohne Anhang angenommen")
	}
}
