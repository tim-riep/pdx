package profile

import (
	"fmt"

	"github.com/tim-riep/pdx/pdf"
)

func init() {
	register("pdfua-1", func() Profile { return PDFUA{Part: 1} })
}

// PDFUA ist PDF/UA-1 nach ISO 14289-1:2014. Es verlangt Tagged PDF mit
// vollständigem Strukturbaum, Titel, Sprache und Alternativtexten. Das
// prüft der Renderer vor dem Schreiben, das Profil selbst verweigert nur
// Glyphen ohne Unicode-Zuordnung (7.21.7). Es lässt sich mit PDF/A-2 und
// PDF/A-3 kombinieren, die ebenfalls auf PDF 1.7 beruhen.
type PDFUA struct{ Part int }

// Name liefert "pdfua-1".
func (p PDFUA) Name() string { return fmt.Sprintf("pdfua-%d", p.Part) }

// Allow prüft eine Funktion gegen ISO 14289-1.
func (p PDFUA) Allow(f pdf.Feature) error {
	switch f {
	case pdf.UnmappedGlyph:
		return deny(p, f, "ISO 14289-1 7.21.7 verlangt für jede Glyphe eine Unicode-Zuordnung")
	}
	return nil
}

func (p PDFUA) claim() Claim { return Claim{Version: "1.7", UA: p.Part} }

// Prepare ergänzt XMP mit pdfuaid und DisplayDocTitle.
func (p PDFUA) Prepare(ctx *Context) error { return prepareClaim(ctx, p.claim()) }
