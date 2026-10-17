package app

import (
	"context"
	"testing"

	"github.com/tim-riep/pdx"
	"github.com/tim-riep/pdx/template"
	"github.com/tim-riep/pdx/template/pkg"
)

func cells(row *template.Node) int {
	n := 0
	for _, c := range row.Elements("cell") {
		n += colspan(c)
	}
	return n
}

// TestTableOps baut den Tabellenaufbau der Musterrechnung nach: Tabelle aus
// dem Datenknoten, Übertrag in einer zweiten Kopfzeile über vier Spalten,
// Fuß mit Zwischensumme, und setzt das Ergebnis.
func TestTableOps(t *testing.T) {
	c := NewCore(nil, nil, t.TempDir())
	if _, err := NewProjectService(c).Open(muster); err != nil {
		t.Fatal(err)
	}
	ts := NewTemplateService(c)
	res, err := ts.Drop(Drop{Path: "inv:invoice/inv:items/inv:item", Parent: "inhalt", Index: -1})
	if err != nil {
		t.Fatal(err)
	}
	tbl := c.doc.Pkg.Template.ByID(res.Select)
	check := func(step string) {
		t.Helper()
		ncols := len(tbl.Elements("column"))
		for _, r := range rowsOf(tbl) {
			if cells(r) != ncols {
				t.Fatalf("%s: Zeile %s hat %d Zellen, Tabelle %d Spalten", step, r.ID(), cells(r), ncols)
			}
		}
	}
	check("Anfang")

	if _, err := ts.Table(TableOp{Op: "addColumn", Ref: tbl.ID(), Index: 1}); err != nil {
		t.Fatal(err)
	}
	check("Spalte eingefügt")
	if _, err := ts.Table(TableOp{Op: "removeColumn", Ref: tbl.ID(), Index: 1}); err != nil {
		t.Fatal(err)
	}
	check("Spalte entfernt")

	head := tbl.First("header")
	res, err = ts.Table(TableOp{Op: "addRow", Ref: head.ID(), Index: -1})
	if err != nil {
		t.Fatal(err)
	}
	row := c.doc.Pkg.Template.ByID(res.Select)
	first := row.Elements("cell")[0]
	if _, err := ts.Table(TableOp{Op: "span", Ref: first.ID(), Count: 4}); err != nil {
		t.Fatal(err)
	}
	check("verbunden")
	if len(row.Elements("cell")) != 2 {
		t.Fatalf("nach dem Verbinden %d Zellen, erwartet 2", len(row.Elements("cell")))
	}
	if _, err := ts.Table(TableOp{Op: "sum", Ref: row.Elements("cell")[1].ID(), Kind: "carryOver"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ts.Table(TableOp{Op: "addSection", Ref: tbl.ID(), Kind: "footer"}); err != nil {
		t.Fatal(err)
	}
	check("Fuß")
	foot := tbl.First("footer").First("row")
	res, err = ts.Table(TableOp{Op: "sum", Ref: foot.Elements("cell")[4].ID(), Kind: "subtotal"})
	if err != nil {
		t.Fatal(err)
	}
	if sum := c.doc.Pkg.Template.ByID(res.Select); sum.Get("of", "") != "inv:amount" || sum.Get("format", "") != "betrag" {
		t.Errorf("Zwischensumme: of %q, format %q", sum.Get("of", ""), sum.Get("format", ""))
	}
	if _, err := ts.Table(TableOp{Op: "sum", Ref: foot.Elements("cell")[1].ID(), Kind: "total"}); err == nil {
		t.Error("Summe über eine Textspalte ohne Fehler")
	}
	if _, err := ts.Table(TableOp{Op: "addSection", Ref: tbl.ID(), Kind: "footer"}); err == nil {
		t.Error("zweiter Fuß ohne Fehler")
	}

	// Das Ergebnis lässt sich setzen.
	p, err := pkg.New(c.doc.Pkg.Template, c.doc.Pkg.Files)
	if err != nil {
		t.Fatal(err)
	}
	tpl, err := pdx.CompileTemplate(p, "")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := p.File("samples/rechnung.xml")
	if _, err := tpl.Layout(data, pdx.TemplateOptions{Options: pdx.Options{Context: context.Background()}}); err != nil {
		t.Fatal(err)
	}
}
