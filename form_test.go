package pdx

import (
	"errors"
	"strings"
	"testing"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/internal/testutil"
	"github.com/tim-riep/pdx/pdf/reader"
	"github.com/tim-riep/pdx/template"
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

// TestFormAccessible schreibt das Formular barrierefrei als PDF/A-3a mit
// PDF/UA-1 und als PDF/A-4f mit PDF/UA-2: je Widget ein Form-Element mit
// Objektreferenz, Tab-Reihenfolge nach Struktur. Fehlt eine Bezeichnung,
// nennt die Vorprüfung alle betroffenen Felder.
func TestFormAccessible(t *testing.T) {
	tpl, data := loadForm(t)
	att := areatree.Attachment{Name: "antrag.xml", MIMEType: "text/xml", Relationship: areatree.Data, Description: "Antragsdaten", Data: data}
	for _, prof := range []string{"pdfa-3a,pdfua-1", "pdfa-4f,pdfua-2"} {
		t.Run(prof, func(t *testing.T) {
			res, err := tpl.Render(data, TemplateOptions{Options: Options{CreationDate: templateDate, Profile: prof, Attachments: []areatree.Attachment{att}}})
			if err != nil {
				t.Fatal(err)
			}
			got := structure(t, res.PDF)
			ua := prof[len(prof)-1:]
			testutil.Golden(t, "testdata/golden/structure/formular-ua"+ua+".txt", got)
			if n := strings.Count(string(got), "\n    Form\n") + strings.Count(string(got), "\n      Form\n"); n != 12 {
				t.Errorf("%d Form-Elemente, erwartet 12", n)
			}
			if !strings.Contains(string(got), `Annotation Widget "Anrede Frau"`) {
				t.Error("Optionsfeld ohne Contents")
			}
			testutil.Conform(t, "formular-ua"+ua+".pdf", res.PDF, prof)
		})
	}

	// Ohne Bezeichnungen meldet die Vorprüfung jedes Feld.
	src := string(template.Write(tpl.Package.Template))
	for _, l := range []string{` label="Name"`, ` label="E-Mail-Adresse"`, ` label="Unterschrift"`} {
		src = strings.Replace(src, l, "", 1)
	}
	parsed, err := template.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	q, err := pkg.New(parsed, tpl.Package.Files)
	if err != nil {
		t.Fatal(err)
	}
	_, err = RenderTemplate(q, data, TemplateOptions{Options: Options{CreationDate: templateDate, Profile: "pdfa-3a,pdfua-1"}})
	var es template.Errors
	if !errors.As(err, &es) {
		t.Fatalf("keine Befunde: %v", err)
	}
	ids := map[string]bool{}
	for _, e := range es {
		if strings.Contains(e.Msg, "Formularfeld ohne Bezeichnung") {
			ids[e.ID] = true
		}
	}
	if !ids["name"] || !ids["email"] || !ids["unterschrift"] || len(ids) != 3 {
		t.Errorf("Befunde %v", es)
	}
}

// TestExtractData liest ein ausgefülltes Formular zurück: Werte aus den
// Feldern, alles andere aus den Grunddaten, Reihenfolge nach Daten-XSD.
func TestExtractData(t *testing.T) {
	tpl, base := loadForm(t)
	filled := strings.NewReplacer(
		"<anrede>Frau</anrede>", "<anrede>divers</anrede>",
		"<name>Erika Musterfrau</name>", "<name>Alex Beispiel</name>",
		"<tarif>Komfort</tarif>", "<tarif>Premium</tarif>",
		"<newsletter>false</newsletter>", "<newsletter>true</newsletter>",
		"<vorname>Max</vorname>", "<vorname>Moritz</vorname>",
	).Replace(string(base))
	// Neue Werte für Felder, deren Knoten in den Grunddaten fehlen.
	filled = strings.Replace(filled, "<land>DE</land>", "<email>alex@beispiel.example</email>\n  <land>AT</land>", 1)
	res, err := tpl.Render([]byte(filled), TemplateOptions{Options: Options{CreationDate: templateDate, Profile: "pdfa-3a,pdfua-1"}})
	if err != nil {
		t.Fatal(err)
	}
	out, err := tpl.ExtractData(res.PDF, base)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, want := range []string{
		"<anrede>divers</anrede>", "<name>Alex Beispiel</name>", "<email>alex@beispiel.example</email>",
		"<land>AT</land>", "<tarif>Premium</tarif>", "<newsletter>true</newsletter>",
		"<vorname>Erika</vorname><geburtsjahr>1984</geburtsjahr>", "<vorname>Moritz</vorname>",
	} {
		if !strings.Contains(strings.Join(strings.Fields(string(out)), ""), strings.Join(strings.Fields(want), "")) {
			t.Errorf("%q fehlt in\n%s", want, out)
		}
	}
	// email steht nach dem XSD vor land, auch wenn es neu angelegt wurde.
	if strings.Index(string(out), "<email>") > strings.Index(string(out), "<land>") {
		t.Errorf("Reihenfolge nach XSD verletzt:\n%s", out)
	}
	if err := tpl.Validate(out); err != nil {
		t.Errorf("ungültig: %v", err)
	}

	// Ohne Grunddaten entsteht das Dokument aus den Feldern.
	out, err = tpl.ExtractData(res.PDF, nil)
	if err != nil || !strings.Contains(string(out), "<a:name>Alex Beispiel</a:name>") || !strings.Contains(string(out), "<a:vorname>Moritz</a:vorname>") {
		t.Errorf("ohne Grunddaten: %v\n%s", err, out)
	}

	// Werte außerhalb der Enumeration und zu lange Namen meldet die Prüfung
	// gegen das XSD auf einmal, die Daten liegen trotzdem vor.
	bad := strings.Replace(string(base), "<land>DE</land>", "<land>FR</land>", 1)
	bad = strings.Replace(bad, "<name>Erika Musterfrau</name>", "<name>"+strings.Repeat("x", 45)+"</name>", 1)
	res, err = tpl.Render([]byte(bad), TemplateOptions{Options: Options{CreationDate: templateDate, Profile: "pdfa-3b"}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = tpl.ExtractData(res.PDF, base)
	e, ok := IsExtractError(err)
	if !ok || !strings.Contains(e.Error(), `"FR" is not in the enumeration`) || !strings.Contains(e.Error(), "length 45 > 40") || !strings.Contains(string(e.Data), "<land>FR</land>") {
		t.Errorf("Fehlerliste: %v", err)
	}
}

// FuzzExtractData verlangt, dass beliebige Eingaben weder abstürzen noch
// hängen. Startwerte sind ein gerendertes Formular und Bruchstücke davon.
func FuzzExtractData(f *testing.F) {
	tpl, base := loadForm(f)
	res, err := tpl.Render(base, TemplateOptions{Options: Options{CreationDate: templateDate, Profile: "pdfa-3b"}})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(res.PDF)
	f.Add(res.PDF[:len(res.PDF)/2])
	f.Add([]byte("%PDF-1.7\ntrailer\n<< /Root 1 0 R >>\nstartxref\n0\n%%EOF\n"))
	f.Fuzz(func(t *testing.T, b []byte) {
		tpl.ExtractData(b, base)
	})
}
