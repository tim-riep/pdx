package profile

import (
	"fmt"

	"github.com/tim-riep/pdx/pdf"
)

func init() {
	register("pdfua-1", func() Profile { return PDFUA{Part: 1} })
	register("pdfua-2", func() Profile { return PDFUA{Part: 2} })
}

// PDFUA ist PDF/UA-1 nach ISO 14289-1:2014 oder PDF/UA-2 nach
// ISO 14289-2:2024. Es verlangt Tagged PDF mit vollständigem Strukturbaum,
// Titel, Sprache und Alternativtexten. Das prüft der Renderer vor dem
// Schreiben, das Profil selbst verweigert nur Glyphen ohne
// Unicode-Zuordnung. PDF/UA-1 beruht auf PDF 1.7 und lässt sich mit PDF/A-2
// und PDF/A-3 kombinieren, PDF/UA-2 beruht auf PDF 2.0 und passt zu
// PDF/A-4.
type PDFUA struct{ Part int }

// Name liefert "pdfua-1" oder "pdfua-2".
func (p PDFUA) Name() string { return fmt.Sprintf("pdfua-%d", p.Part) }

// Allow prüft eine Funktion gegen ISO 14289.
func (p PDFUA) Allow(f pdf.Feature) error {
	switch f {
	case pdf.UnmappedGlyph:
		clause := "ISO 14289-1 7.21.7"
		if p.Part == 2 {
			clause = "ISO 14289-2 8.4.5.8"
		}
		return deny(p, f, clause+" verlangt für jede Glyphe eine Unicode-Zuordnung")
	}
	return nil
}

func (p PDFUA) claim() Claim {
	v := "1.7"
	if p.Part == 2 {
		v = "2.0"
	}
	return Claim{Version: v, UA: p.Part}
}

// Prepare ergänzt XMP mit pdfuaid und DisplayDocTitle.
func (p PDFUA) Prepare(ctx *Context) error { return prepareClaim(ctx, p.claim()) }
