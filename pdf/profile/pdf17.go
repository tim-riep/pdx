package profile

import "github.com/tim-riep/pdx/pdf"

func init() { register("pdf-1.7", func() Profile { return PDF17{} }) }

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
