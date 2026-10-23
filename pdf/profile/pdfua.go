package profile

import (
	"fmt"

	"github.com/tim-riep/pdx/pdf"
)

func init() {
	register("pdfua-1", func() Profile { return PDFUA{Part: 1} })
	register("pdfua-2", func() Profile { return PDFUA{Part: 2} })
}

// PDFUA is PDF/UA-1 according to ISO 14289-1:2014 or PDF/UA-2 according to
// ISO 14289-2:2024. It requires Tagged PDF with a complete structure tree,
// title, language and alternative texts. The renderer checks this before
// writing, the profile itself only refuses glyphs without a
// Unicode mapping. PDF/UA-1 is based on PDF 1.7 and can be combined with
// PDF/A-2 and PDF/A-3, PDF/UA-2 is based on PDF 2.0 and fits
// PDF/A-4.
type PDFUA struct{ Part int }

// Name returns "pdfua-1" or "pdfua-2".
func (p PDFUA) Name() string { return fmt.Sprintf("pdfua-%d", p.Part) }

// Allow checks a feature against ISO 14289.
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

// Prepare adds XMP with pdfuaid and DisplayDocTitle.
func (p PDFUA) Prepare(ctx *Context) error { return prepareClaim(ctx, p.claim()) }
