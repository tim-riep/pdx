package schema

import (
	"context"
	"strings"
	"testing"

	"github.com/tim-riep/pdx"
	"github.com/tim-riep/pdx/template/pkg"
)

func load(t *testing.T) (*pkg.Package, *Schema) {
	t.Helper()
	p, err := pkg.ReadDir("../../../testdata/templates/musterrechnung")
	if err != nil {
		t.Fatal(err)
	}
	s, err := Load(p)
	if err != nil || s == nil {
		t.Fatalf("Load: %v", err)
	}
	return p, s
}

func TestTree(t *testing.T) {
	_, s := load(t)
	if len(s.Roots) != 1 || s.Roots[0].Name != "inv:invoice" || len(s.Missing) != 0 {
		t.Fatalf("Wurzeln %+v, fehlend %v", s.Roots, s.Missing)
	}
	item := s.Find("inv:invoice/inv:items/inv:item")
	if item == nil || !item.Repeated() {
		t.Fatalf("inv:item: %+v", item)
	}
	if a := s.Find("inv:invoice/inv:items/inv:item/inv:amount"); a == nil || a.Type != "decimal" {
		t.Errorf("inv:amount: %+v", a)
	}
	if d := s.Find("inv:invoice/inv:issueDate"); d == nil || d.Type != "date" {
		t.Errorf("inv:issueDate: %+v", d)
	}
}

func TestContext(t *testing.T) {
	p, _ := load(t)
	tpl := p.Template
	cases := map[string]string{
		"empfaenger-name": "inv:invoice/inv:buyer",
		"z-betrag-f":      "inv:invoice/inv:items/inv:item",
		"netto-wert":      "inv:invoice",
	}
	for id, want := range cases {
		got, err := Context(tpl, tpl.ByID(id))
		if err != nil || got != want {
			t.Errorf("%s: %q, %v, erwartet %q", id, got, err, want)
		}
	}
	if r := Relative("inv:invoice/inv:buyer", "inv:invoice/inv:seller/inv:name"); r != "../inv:seller/inv:name" {
		t.Errorf("Relative: %q", r)
	}
	if r := Relative("inv:invoice", "inv:invoice/inv:totals/inv:net"); r != "inv:totals/inv:net" {
		t.Errorf("Relative: %q", r)
	}
}

// TestPlaceholder checks that the template can be typeset with the placeholder
// data. The data is strictly valid only once go-xslt supports facets such as
// pattern (tim-riep/go-xslt#3), for the design view this is enough.
func TestPlaceholder(t *testing.T) {
	p, s := load(t)
	data := s.Placeholder()
	if !strings.Contains(string(data), "<inv:description>description</inv:description>") {
		t.Errorf("Texte zeigen nicht den Feldnamen:\n%.400s", data)
	}
	tpl, err := pdx.CompileTemplate(p, "")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := tpl.Layout(data, pdx.TemplateOptions{Options: pdx.Options{Context: context.Background()}})
	if err != nil || len(doc.Pages) == 0 {
		t.Fatalf("Setzen mit Platzhaltern: %v", err)
	}
}
