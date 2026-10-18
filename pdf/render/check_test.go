package render

import (
	"errors"
	"reflect"
	"testing"

	"github.com/tim-riep/pdx/areatree"
)

// TestCheckAllErrors verlangt alle Befunde der Vorprüfung auf einmal, je
// mit Vorlagen-ID.
func TestCheckAllErrors(t *testing.T) {
	block := func(id, src, role string, kids ...areatree.Area) *areatree.Container {
		return &areatree.Container{Kind: areatree.KindBlock, ID: id, Source: src, Role: role, Children: kids}
	}
	cell := func(id, src string) *areatree.Container {
		c := block(id, src, "")
		c.Tag = &areatree.Tag{Row: 1, Section: "body"}
		return c
	}
	doc := &areatree.Document{
		Pages: []*areatree.Page{{
			Number: 1, Width: areatree.Points(595), Height: areatree.Points(842),
			Areas: []areatree.Area{
				block("titel", "fo:block@1:1", "H2"),
				block("logo-box", "fo:block@2:1", "", &areatree.Image{ID: "logo"}),
				block("unter", "fo:block@3:1", "H4"),
				block("tabelle", "fo:table@4:1", "", cell("z1", "fo:table-cell@5:1")),
				&areatree.Container{Kind: areatree.KindRegion, ID: "kopf", Source: "fo:region-before@6:1",
					Tag: &areatree.Tag{Static: true},
					Children: []areatree.Area{&areatree.Link{URI: "https://example.org/"}}},
			},
		}},
	}
	// Leere Blöcke verschwinden aus dem Baum, also bekommen sie Text.
	for _, a := range doc.Pages[0].Areas[:4] {
		c := a.(*areatree.Container)
		if c.ID != "logo-box" {
			target := c
			if c.ID == "tabelle" {
				target = c.Children[0].(*areatree.Container)
			}
			target.Children = append(target.Children, &areatree.GlyphRun{})
		}
	}
	_, err := Render(doc, Options{Profile: lookup(t, "pdfa-3a,pdfua-1"), CreationDate: testDate})
	var ce CheckErrors
	if !errors.As(err, &ce) {
		t.Fatalf("keine Befunde der Vorprüfung: %v", err)
	}
	var got []string
	for _, e := range ce {
		got = append(got, e.Error())
	}
	want := []string{
		"Dokumenttitel fehlt (template/@title oder fo:declarations), ISO 14289-1 7.1 verlangt dc:title",
		"Dokumentsprache fehlt (template/@lang oder xml:lang am fo:root), ISO 14289-1 7.2, ISO 19005-3 6.7",
		"[titel] erste Überschrift ist H2 statt H1, ISO 14289-1 7.4.2",
		"[logo] Abbildung ohne Alternativtext (alt oder altBind), ISO 14289-1 7.3, ISO 19005-3 6.7",
		"[unter] Überschrift H4 folgt auf H2, Ebene H3 fehlt, ISO 14289-1 7.4.2",
		"[tabelle] Tabelle ohne Kopfzellen (Kopfbereich oder scope an Zellen), ISO 14289-1 7.5",
		"[kopf] Verweis in Kopf, Fuß oder Artefakt, ISO 14289-1 7.18.1 verlangt, dass jede Annotation im Strukturbaum steht",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Befunde:\n%q\nerwartet:\n%q", got, want)
	}

	// PDF/A-3a allein verlangt Sprache und Alternativtexte, nicht die
	// Regeln von PDF/UA.
	_, err = Render(doc, Options{Profile: lookup(t, "pdfa-3a"), CreationDate: testDate})
	if !errors.As(err, &ce) || len(ce) != 2 {
		t.Errorf("PDF/A-3a: %v", err)
	}
	// Ohne Anspruch auf Barrierefreiheit gibt es keine Vorprüfung. Das
	// Bild ohne Daten scheitert erst beim Schreiben.
	_, err = Render(doc, Options{Profile: pdfa3b(t), CreationDate: testDate, Tagged: true})
	if errors.As(err, &ce) {
		t.Errorf("Vorprüfung ohne Anspruch: %v", err)
	}
}

func TestListNumbering(t *testing.T) {
	for _, c := range []struct {
		labels string
		pdf2   bool
		want   string
	}{
		{"1.2.3.", false, "Decimal"}, {"a)b)", true, "LowerAlpha"}, {"A.B.", false, "UpperAlpha"},
		{"••", false, "Disc"}, {"––", true, "Unordered"},
	} {
		if got := listNumbering(c.labels, c.pdf2); got != c.want {
			t.Errorf("%q: %s statt %s", c.labels, got, c.want)
		}
	}
}
