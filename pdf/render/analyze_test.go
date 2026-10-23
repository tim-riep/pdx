package render

import (
	"testing"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/pdf/profile"
)

// Analyze returns the tree with template IDs and the findings, without PDF.
func TestAnalyze(t *testing.T) {
	doc := &areatree.Document{
		Meta: areatree.Metadata{Title: "T", Lang: "de"},
		Pages: []*areatree.Page{{Number: 1, Areas: []areatree.Area{
			&areatree.Container{Kind: areatree.KindBlock, ID: "titel", Source: "fo:block@1:1", Role: "H1",
				Children: []areatree.Area{&areatree.GlyphRun{Glyphs: []areatree.Glyph{{Text: "Rech"}, {Text: "nung"}}}}},
			&areatree.Container{Kind: areatree.KindBlock, ID: "logo-box", Source: "fo:block@2:1",
				Children: []areatree.Area{&areatree.Image{ID: "logo"}}},
		}}},
	}
	root, errs := Analyze(doc, profile.Claim{Version: "1.7", PDFA: 3, Level: "A", UA: 1})
	if root.Type != "Document" || len(root.Kids) != 2 {
		t.Fatalf("%+v", root)
	}
	h := root.Kids[0]
	if h.Type != "H1" || h.ID != "titel" || h.Text != "Rechnung" || h.Page != 1 {
		t.Errorf("Überschrift %+v", h)
	}
	if f := root.Kids[1].Kids[0]; f.Type != "Figure" || f.ID != "logo" {
		t.Errorf("Abbildung %+v", f)
	}
	if len(errs) != 1 || errs[0].ID != "logo" {
		t.Errorf("Befunde %v", errs)
	}
	// Without an accessibility claim, no findings.
	if _, errs := Analyze(doc, profile.Claim{Version: "1.7", PDFA: 3, Level: "B"}); len(errs) != 0 {
		t.Errorf("Befunde ohne Anspruch: %v", errs)
	}
}
