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

// Das Formular ist sauber, Bindings liefert die Enumerationen für die
// Auswahlfelder.
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

// Fehler an Formularfeldern, alle auf einmal mit Vorlagen-ID.
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
