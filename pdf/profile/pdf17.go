package profile

import "github.com/tim-riep/pdx/pdf"

func init() {
	register("pdf-1.7", func() Profile { return PDF17{} })
	register("pdf-2.0", func() Profile { return PDF20{} })
}

// PDF17 ist schlichtes PDF 1.7 ohne Konformitätsanspruch. Auch hier bettet pdx
// alle Schriften ein und schreibt weder JavaScript noch XFA.
type PDF17 struct{}

// Name liefert "pdf-1.7".
func (PDF17) Name() string { return "pdf-1.7" }

// Allow erlaubt alles außer nicht eingebetteten Schriften und externen Inhalten.
func (p PDF17) Allow(f pdf.Feature) error {
	switch f {
	case pdf.UnembeddedFont:
		return deny(p, f, "pdx bettet jede Schrift ein")
	case pdf.ExternalContent:
		return deny(p, f, "pdx schreibt nur in sich geschlossene Dateien")
	}
	return nil
}

// Prepare ergänzt nichts.
func (PDF17) Prepare(*Context) error { return nil }

// PDF20 ist schlichtes PDF 2.0 (ISO 32000-2) ohne Konformitätsanspruch.
// Statt des Info-Dictionarys stehen die Metadaten im XMP.
type PDF20 struct{}

// Name liefert "pdf-2.0".
func (PDF20) Name() string { return "pdf-2.0" }

// Allow erlaubt dasselbe wie pdf-1.7.
func (PDF20) Allow(f pdf.Feature) error {
	if err := (PDF17{}).Allow(f); err != nil {
		return deny(PDF20{}, f, err.(*pdf.FeatureError).Reason)
	}
	return nil
}

func (PDF20) claim() Claim { return Claim{Version: "2.0"} }

// Prepare schreibt die XMP-Metadaten.
func (p PDF20) Prepare(ctx *Context) error { return prepareClaim(ctx, p.claim()) }
