package app

import (
	"strings"
	"testing"

	"github.com/tim-riep/pdx/template"
)

// inner returns the content of an element as a fragment for comparison.
func inner(n *template.Node) string {
	var b strings.Builder
	for _, c := range n.Children {
		if c.Kind == template.Text {
			b.WriteString(c.Data)
		} else {
			b.WriteString("<" + c.Name)
			if id := c.ID(); id != "" {
				b.WriteString(" " + id)
			}
			if bind := c.Get("bind", ""); bind != "" {
				b.WriteString(" " + bind)
			}
			b.WriteString(">")
		}
	}
	return b.String()
}

func TestSetContent(t *testing.T) {
	c := NewCore(nil, nil, t.TempDir())
	if _, err := NewProjectService(c).Open(muster); err != nil {
		t.Fatal(err)
	}
	ts := NewTemplateService(c)
	tpl := c.doc.Pkg.Template
	nr := tpl.ByID("rechnungsnummer")
	before := inner(nr)
	if before != "Rechnungsnummer: <field rechnungsnummer-wert inv:number>" {
		t.Fatalf("Ausgangslage %q", before)
	}

	// Change text, keep field, line break and new field.
	_, err := ts.SetContent(ContentEdit{Ref: "rechnungsnummer", Segments: []Segment{
		{Text: "Nr. "}, {Ref: "rechnungsnummer-wert"}, {Text: " vom\n"}, {Bind: "inv:issueDate"}, {Text: " & mehr"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	got := inner(tpl.ByID("rechnungsnummer"))
	if got != "Nr. <field rechnungsnummer-wert inv:number> vom<br rechnungsnummer-umbruch><field rechnungsnummer-issueDate inv:issueDate> & mehr" {
		t.Fatalf("Inhalt %q", got)
	}
	// The kept field still carries its attributes.
	if tpl.ByID("rechnungsnummer-wert").Get("fontWeight", "") != "bold" {
		t.Fatal("Attribute des Feldes verloren")
	}
	// Undo restores the old content.
	if _, err := ts.Undo(); err != nil {
		t.Fatal(err)
	}
	if got := inner(tpl.ByID("rechnungsnummer")); got != before {
		t.Fatalf("Undo: %q", got)
	}

	// Leaving out a field deletes it.
	if _, err := ts.SetContent(ContentEdit{Ref: "rechnungsnummer", Segments: []Segment{{Text: "ohne Feld"}}}); err != nil {
		t.Fatal(err)
	}
	if tpl.ByID("rechnungsnummer-wert") != nil {
		t.Fatal("entferntes Feld bleibt")
	}

	// Text resource: text only.
	if _, err := ts.SetContent(ContentEdit{Ref: "txt-de-rechnung", Segments: []Segment{{Text: "Faktura"}}}); err != nil {
		t.Fatal(err)
	}
	if got := inner(tpl.ByID("txt-de-rechnung")); got != "Faktura" {
		t.Fatalf("Textressource %q", got)
	}
	if _, err := ts.SetContent(ContentEdit{Ref: "txt-de-rechnung", Segments: []Segment{{Bind: "inv:number"}}}); err == nil {
		t.Fatal("Feld in Textressource angenommen")
	}
	// Foreign element as a chip and element without text.
	if _, err := ts.SetContent(ContentEdit{Ref: "rechnungsdatum", Segments: []Segment{{Ref: "empfaenger-name"}}}); err == nil {
		t.Fatal("fremdes Element angenommen")
	}
	if _, err := ts.SetContent(ContentEdit{Ref: "positionen", Segments: []Segment{{Text: "x"}}}); err == nil {
		t.Fatal("Text in Tabelle angenommen")
	}
}
