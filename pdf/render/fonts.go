package render

import (
	"fmt"

	"github.com/tim-riep/pdx/areatree"
)

// fontSet verwaltet die eingebetteten Schriften. Die Einbettung folgt im
// Schritt Schriften.
type fontSet struct{ r *renderer }

func newFontSet(r *renderer) *fontSet { return &fontSet{r: r} }

func (fs *fontSet) embed() error { return nil }

func (pc *pageCtx) glyphs(g *areatree.GlyphRun) error {
	return fmt.Errorf("Glyph-Runs werden noch nicht unterstützt")
}
