package template

import (
	"bytes"
	"os"
	"testing"
)

// TestEditHelpers checks inserting, removing, cloning and attributes on the
// sample invoice. After undoing, the template is unchanged.
func TestEditHelpers(t *testing.T) {
	src, err := os.ReadFile(sample)
	if err != nil {
		t.Fatal(err)
	}
	tpl, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	want := Write(tpl)

	summen := tpl.ByID("summen")
	netto := tpl.ByID("netto")
	i := netto.Index()
	if summen.RemoveChild(netto) != i || netto.Parent != nil {
		t.Fatal("RemoveChild liefert nicht die alte Position")
	}
	tpl.Reindex()
	if tpl.ByID("netto") != nil {
		t.Error("netto nach dem Entfernen noch im Index")
	}
	if err := tpl.Validate(); err != nil {
		t.Errorf("Vorlage ohne netto ungültig: %v", err)
	}

	dup := netto.Clone()
	dup.SetAttr("id", "netto-kopie")
	for _, c := range dup.Children {
		if c.Kind == Element {
			c.SetAttr("id", c.ID()+"-kopie")
		}
	}
	summen.InsertChild(0, dup)
	summen.InsertChild(i+1, netto)
	tpl.Reindex()
	if tpl.ByID("netto-kopie") == nil || tpl.ByID("netto") != netto {
		t.Fatal("Index nach dem Einfügen falsch")
	}
	if err := tpl.Validate(); err != nil {
		t.Errorf("Vorlage mit Kopie ungültig: %v", err)
	}

	summen.RemoveChild(dup)
	if v, ok := summen.RemoveAttr("spaceBefore"); !ok || v != "4mm" {
		t.Errorf("RemoveAttr: %q %v", v, ok)
	}
	summen.SetAttr("spaceBefore", "4mm")
	tpl.Reindex()
	if got := Write(tpl); !bytes.Equal(got, want) {
		t.Errorf("Vorlage nach Rückgängigmachen verändert:\n%s", got)
	}
}

// TestParseTree reads a well-formed but invalid template.
func TestParseTree(t *testing.T) {
	src := []byte(`<template xmlns="https://github.com/tim-riep/pdx/ns/template/1" id="t" version="1"><body id="b"><text/></body></template>`)
	if _, err := Parse(src); err == nil {
		t.Fatal("Parse akzeptiert eine ungültige Vorlage")
	}
	tpl, err := ParseTree(src)
	if err != nil {
		t.Fatal(err)
	}
	if tpl.ByID("b") == nil {
		t.Error("ParseTree baut keinen Index")
	}
}
