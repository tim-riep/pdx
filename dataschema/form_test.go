package dataschema

import (
	"slices"
	"strings"
	"testing"

	"github.com/tim-riep/pdx/template"
	"github.com/tim-riep/pdx/template/pkg"
)

func loadFormular(t *testing.T) (*pkg.Package, Model) {
	t.Helper()
	p, err := pkg.ReadDir("../testdata/templates/formular")
	if err != nil {
		t.Fatal(err)
	}
	main, files := p.Schemas()
	m, err := FromXSD(main, files)
	if err != nil {
		t.Fatal(err)
	}
	return p, m
}

// The form is clean, Bindings returns the enumerations for the choice
// fields.
func TestFormularBindings(t *testing.T) {
	p, m := loadFormular(t)
	if fs := Check(p.Template, m); len(fs) > 0 {
		t.Fatalf("Befunde: %v", fs)
	}
	e := PackageEnums(p)
	if !slices.Equal(e["anrede"], []string{"Frau", "Herr", "divers"}) || !slices.Equal(e["tarif"], []string{"Basis", "Komfort", "Premium"}) {
		t.Errorf("Enumerationen %v", e)
	}
	if d := Bindings(p.Template, m)["vorname"]; d == nil || d.Name.Local != "vorname" {
		t.Errorf("Bindung im wiederholten Teilformular: %+v", d)
	}
}

// Errors on form fields, all at once with template ID.
func TestFormularBroken(t *testing.T) {
	p, m := loadFormular(t)
	src := string(template.Write(p.Template))
	for _, r := range [][2]string{
		{`bind="a:email"`, `bind="concat(a:email, '')"`},
		{`<listBox id="tarif" bind="a:tarif"`, `<listBox id="tarif" bind="a:bemerkung"`},
		{`label="Schweiz" value="CH"`, `label="Schweiz" value="FR"`},
	} {
		if !strings.Contains(src, r[0]) {
			t.Fatalf("%q fehlt", r[0])
		}
		src = strings.Replace(src, r[0], r[1], 1)
	}
	tpl, err := template.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	fs := Check(tpl, m)
	want := map[string]string{
		"email":   "kein einfacher Pfad",
		"tarif":   "weder option-Elemente noch eine Enumeration",
		"land-ch": `Option "FR" steht nicht in der Enumeration`,
	}
	for id, msg := range want {
		found := false
		for _, f := range fs {
			if f.ID == id && f.Level == Error && strings.Contains(f.Msg, msg) {
				found = true
			}
		}
		if !found {
			t.Errorf("[%s] %q fehlt in %v", id, msg, fs)
		}
	}
}

// The maximum length comes from the facet maxLength of the data XSD.
func TestFormularMaxLength(t *testing.T) {
	p, m := loadFormular(t)
	if d := Bindings(p.Template, m)["name"]; d == nil || d.MaxLength != 40 {
		t.Fatalf("maxLength von a:name: %+v", d)
	}
	if f := PackageFacets(p); f.MaxLengths["name"] != 40 || f.MaxLengths["email"] != 0 {
		t.Errorf("Facetten %v", f.MaxLengths)
	}
	// A greater length in the template than in the XSD is a notice.
	src := strings.Replace(string(template.Write(p.Template)), `maxLength="40"`, `maxLength="60"`, 1)
	tpl, err := template.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range Check(tpl, m) {
		if f.ID == "name" && f.Level == Warning && strings.Contains(f.Msg, "maxLength 60 ist größer als die Höchstlänge 40") {
			found = true
		}
	}
	if !found {
		t.Errorf("Hinweis fehlt: %v", Check(tpl, m))
	}
}
