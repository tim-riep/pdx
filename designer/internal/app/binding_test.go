package app

import (
	"strings"
	"testing"

	"github.com/tim-riep/pdx/designer/internal/model"
	"github.com/tim-riep/pdx/designer/internal/schema"
	"github.com/tim-riep/pdx/template"
)

func TestFragment(t *testing.T) {
	el := template.NewElement("field", template.Attr{Name: "id", Value: "a"}, template.Attr{Name: "bind", Value: "inv:x"})
	if got := fragment(el); got != `<field id="a" bind="inv:x"/>` {
		t.Errorf("fragment: %q", got)
	}
}

// TestDrop prüft die Regeln für gezogene Datenknoten an der Musterrechnung.
func TestDrop(t *testing.T) {
	c := NewCore(nil, nil, t.TempDir())
	if _, err := NewProjectService(c).Open(muster); err != nil {
		t.Fatal(err)
	}
	ts := NewTemplateService(c)
	d := c.doc
	s, err := schema.Load(d.Pkg)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		drop  Drop
		check func(n *template.Node) string
	}{
		{Drop{Path: "inv:invoice/inv:dueDate", Parent: "summen", Index: -1}, func(n *template.Node) string {
			if n.Name != "field" || n.Get("bind", "") != "inv:dueDate" || n.Get("format", "") != "datum" {
				return "Datum ohne Format"
			}
			return ""
		}},
		{Drop{Path: "inv:invoice/inv:totals/inv:net", Parent: "summen", Index: 0}, func(n *template.Node) string {
			if n.Get("format", "") != "betrag" || n.Get("bind", "") != "inv:totals/inv:net" {
				return "Betrag ohne Format"
			}
			return ""
		}},
		{Drop{Path: "inv:invoice/inv:items/inv:item", Parent: "inhalt", Index: -1}, func(n *template.Node) string {
			if n.Name != "table" || n.Get("bind", "") != "inv:items/inv:item" || len(n.Elements("column")) != 5 {
				return "Tabelle falsch"
			}
			row := n.First("rows").First("row")
			if f := row.Elements("cell")[4].First("field"); f.Get("bind", "") != "inv:amount" || f.Get("format", "") != "betrag" {
				return "Spalte Betrag falsch"
			}
			return ""
		}},
		{Drop{Path: "inv:invoice/inv:seller/inv:name", Parent: "empfaenger", Index: -1}, func(n *template.Node) string {
			if n.Get("bind", "") != "../inv:seller/inv:name" {
				return "relativer Pfad falsch: " + n.Get("bind", "")
			}
			return ""
		}},
		{Drop{Path: "inv:invoice/inv:paymentTerms", Parent: "adressfenster", X: 5, Y: 30}, func(n *template.Node) string {
			if n.Get("x", "") != "5mm" || n.Get("y", "") != "30mm" || n.Get("w", "") == "" {
				return "positioniert ohne Lage"
			}
			if s.Find("inv:invoice/inv:paymentTerms").Min == 0 && n.Get("presence", "") == "" {
				return "optional ohne Präsenz"
			}
			return ""
		}},
	}
	for _, tc := range cases {
		res, err := ts.Drop(tc.drop)
		if err != nil {
			t.Fatalf("%s: %v", tc.drop.Path, err)
		}
		n := d.Pkg.Template.ByID(res.Select)
		if n == nil {
			t.Fatalf("%s: eingefügtes Element %q fehlt", tc.drop.Path, res.Select)
		}
		if msg := tc.check(n); msg != "" {
			t.Errorf("%s: %s\n%s", tc.drop.Path, msg, fragment(n.Clone()))
		}
	}
	// Die Vorlage bleibt gültig und die Bindungen passen zum Schema.
	if err := d.Pkg.Template.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := ts.Drop(Drop{Path: "inv:invoice/inv:items/inv:item", Parent: "netto"}); err == nil || !strings.Contains(err.Error(), "einfache") {
		t.Errorf("Tabelle in Text: %v", err)
	}
	_ = model.Command{}
}

func TestImportSchema(t *testing.T) {
	c := NewCore(nil, nil, t.TempDir())
	NewProjectService(c).New()
	ss := NewSchemaService(c)
	tree, err := ss.Tree()
	if err != nil || tree != nil {
		t.Fatalf("neue Vorlage hat ein Schema: %v", err)
	}
	if _, err := ss.Import(muster+"/schema/invoice.xsd", ""); err != nil {
		t.Fatal(err)
	}
	root := c.doc.Pkg.Template.Root
	if root.Get("schema", "") != "schema/invoice.xsd" || root.Get("dataRoot", "") != "d:invoice" {
		t.Fatalf("schema %q, dataRoot %q", root.Get("schema", ""), root.Get("dataRoot", ""))
	}
	tree, err = ss.Tree()
	if err != nil || tree == nil || tree.Find("d:invoice/d:items/d:item") == nil {
		t.Fatalf("Datenbaum nach Import: %v", err)
	}
	// Felder lassen sich danach ziehen.
	res, err := NewTemplateService(c).Drop(Drop{Path: "d:invoice/d:number", Parent: "inhalt", Index: -1})
	if err != nil || c.doc.Pkg.Template.ByID(res.Select).Get("bind", "") != "d:number" {
		t.Fatalf("Drop nach Import: %v", err)
	}
	ts := NewTemplateService(c)
	ts.Undo()
	ts.Undo()
	if _, ok := c.doc.Pkg.Files["schema/invoice.xsd"]; ok || c.doc.Pkg.Template.Root.First("namespaces") != nil {
		t.Error("Undo entfernt Schema und Namespace nicht")
	}
	if _, err := ss.Import("/gibt/es/nicht.xsd", ""); err == nil {
		t.Error("Import einer fehlenden Datei gelingt")
	}
}
