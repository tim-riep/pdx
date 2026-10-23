package profile

import "github.com/tim-riep/pdx/pdf"

func init() {
	register("pdf-1.7", func() Profile { return PDF17{} })
	register("pdf-2.0", func() Profile { return PDF20{} })
}

// PDF17 is plain PDF 1.7 without a conformance claim. Here too pdx embeds
// all fonts and writes neither JavaScript nor XFA.
type PDF17 struct{}

// Name returns "pdf-1.7".
func (PDF17) Name() string { return "pdf-1.7" }

// Allow allows everything except unembedded fonts and external content.
func (p PDF17) Allow(f pdf.Feature) error {
	switch f {
	case pdf.UnembeddedFont:
		return deny(p, f, "pdx bettet jede Schrift ein")
	case pdf.ExternalContent:
		return deny(p, f, "pdx schreibt nur in sich geschlossene Dateien")
	}
	return nil
}

// Prepare adds nothing.
func (PDF17) Prepare(*Context) error { return nil }

// PDF20 is plain PDF 2.0 (ISO 32000-2) without a conformance claim.
// Instead of the info dictionary, the metadata is in the XMP.
type PDF20 struct{}

// Name returns "pdf-2.0".
func (PDF20) Name() string { return "pdf-2.0" }

// Allow allows the same as pdf-1.7.
func (PDF20) Allow(f pdf.Feature) error {
	if err := (PDF17{}).Allow(f); err != nil {
		return deny(PDF20{}, f, err.(*pdf.FeatureError).Reason)
	}
	return nil
}

func (PDF20) claim() Claim { return Claim{Version: "2.0"} }

// Prepare writes the XMP metadata.
func (p PDF20) Prepare(ctx *Context) error { return prepareClaim(ctx, p.claim()) }
