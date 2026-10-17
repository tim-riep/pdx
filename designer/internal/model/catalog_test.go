package model

import (
	"slices"
	"testing"

	"github.com/tim-riep/pdx/template"
)

func TestCatalog(t *testing.T) {
	c, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	sub := c["subform"]
	if sub == nil {
		t.Fatal("subform fehlt")
	}
	layout, ok := sub.Property("layout")
	if !ok || layout.Kind != "enum" || !slices.Equal(layout.Enum, []string{"positioned", "flowed"}) || !layout.Required {
		t.Errorf("subform/@layout: %+v", layout)
	}
	if x, _ := sub.Property("x"); x.Kind != "length" || x.Group != "position" {
		t.Errorf("subform/@x: %+v", x)
	}
	if id, _ := sub.Property("id"); id.Kind != "id" || !id.Required {
		t.Errorf("subform/@id: %+v", id)
	}
	if c["table/header"] == nil || c["pageMaster/header"] == nil {
		t.Error("lokale Elemente header fehlen")
	}
	if _, ok := c["pageMaster/header"].Property("height"); !ok {
		t.Error("Kopf der Seitenvorlage ohne height")
	}
	if !c["text"].Mixed {
		t.Error("text ist gemischter Inhalt")
	}

	// Jeder Knoten der Musterrechnung hat einen Typ, jedes Attribut ist bekannt.
	d, err := Open("../../../testdata/templates/musterrechnung")
	if err != nil {
		t.Fatal(err)
	}
	var walk func(n *template.Node)
	walk = func(n *template.Node) {
		if n.Kind != template.Element {
			return
		}
		et := c.TypeOf(n)
		if et == nil {
			t.Errorf("kein Typ für %s", n.Name)
			return
		}
		for _, a := range n.Attrs {
			if _, ok := et.Property(a.Name); !ok {
				t.Errorf("%s (%s): Attribut %s unbekannt", n.ID(), et.Key, a.Name)
			}
		}
		for _, ch := range n.Children {
			walk(ch)
		}
	}
	walk(d.Pkg.Template.Root)
}

func TestProjection(t *testing.T) {
	c, _ := LoadCatalog()
	d, err := Open("../../../testdata/templates/musterrechnung")
	if err != nil {
		t.Fatal(err)
	}
	s := Project(d.Pkg.Template, 7, c)
	if s.Rev != 7 || s.Root != "rechnung" && s.Nodes[s.Root].Name != "template" {
		t.Fatalf("Wurzel %q", s.Root)
	}
	netto := s.Nodes["netto"]
	if netto.Type != "text" || netto.Parent != "summen" || len(netto.Children) != 3 ||
		netto.Children[1].Ref != "netto-wert" || netto.Children[0].Text != "Nettobetrag: " {
		t.Errorf("netto: %+v", netto)
	}
	// Elemente ohne ID sind über ihren Pfad erreichbar.
	for ref, vn := range s.Nodes {
		n, err := Resolve(d.Pkg.Template, ref)
		if err != nil || n.Name != vn.Name {
			t.Errorf("Resolve(%q): %v", ref, err)
		}
	}
	if _, err := Resolve(d.Pkg.Template, "~gibtsnicht/0"); err == nil {
		t.Error("Resolve mit unbekanntem Pfad gelingt")
	}
}
