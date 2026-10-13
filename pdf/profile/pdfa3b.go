package profile

import (
	"github.com/tim-riep/pdx/icc"
	"github.com/tim-riep/pdx/pdf"
	"github.com/tim-riep/pdx/pdf/xmp"
)

func init() { register("pdfa-3b", func() Profile { return PDFA3B{} }) }

// PDFA3B ist PDF/A-3b nach ISO 19005-3:2012, Konformitätsstufe B.
//
// Das Profil verweigert Verschlüsselung (6.1.3), LZW (6.1.7.2), externe
// Inhalte (6.1.7.1, 6.8) und nicht eingebettete Schriften (6.2.11.4.1).
// Transparenz und Dateianhänge sind in PDF/A-3 erlaubt. Prepare ergänzt das
// Output Intent GTS_PDFA1 mit sRGB-Profil (6.2.3) und die XMP-Metadaten mit
// pdfaid (6.6.2), passend zum Info-Dictionary (6.6.2.3).
type PDFA3B struct{}

// Name liefert "pdfa-3b".
func (PDFA3B) Name() string { return "pdfa-3b" }

// Allow prüft eine Funktion gegen ISO 19005-3.
func (p PDFA3B) Allow(f pdf.Feature) error {
	switch f {
	case pdf.Encryption:
		return deny(p, f, "ISO 19005-3 6.1.3 verbietet das Encrypt-Dictionary")
	case pdf.LZW:
		return deny(p, f, "ISO 19005-3 6.1.7.2 verbietet LZWDecode")
	case pdf.ExternalContent:
		return deny(p, f, "ISO 19005-3 6.1.7.1 verbietet Streams in externen Dateien")
	case pdf.UnembeddedFont:
		return deny(p, f, "ISO 19005-3 6.2.11.4.1 verlangt eingebettete Schriften")
	case pdf.JavaScript:
		return deny(p, f, "ISO 19005-3 6.6.1 verbietet JavaScript")
	case pdf.XFA:
		return deny(p, f, "ISO 19005-3 6.4.2 verbietet XFA")
	}
	return nil
}

// Prepare ergänzt Output Intent und XMP-Metadaten.
func (p PDFA3B) Prepare(ctx *Context) error {
	return preparePDFA(ctx, 3, "B")
}

func preparePDFA(ctx *Context, part int, conformance string) error {
	d := ctx.Doc
	profile := d.Add(pdf.NewFlateStream(pdf.D("N", pdf.Int(3)), icc.SRGB()))
	ctx.Catalog.Set("OutputIntents", pdf.Array{pdf.D(
		"Type", pdf.Name("OutputIntent"),
		"S", pdf.Name("GTS_PDFA1"),
		"OutputConditionIdentifier", pdf.String(icc.SRGBDescription),
		"Info", pdf.String(icc.SRGBDescription),
		"RegistryName", pdf.String("http://www.color.org"),
		"DestOutputProfile", profile,
	)})

	m := ctx.Meta
	pkt := &xmp.Packet{
		PDFAPart: part, PDFAConformance: conformance,
		Title: m.Title, Author: m.Author, Subject: m.Subject, Keywords: m.Keywords,
		Creator: m.Creator, Producer: ctx.Producer,
		Created: ctx.Date, Modified: ctx.Date,
		Schemas: m.XMP, Extension: true,
	}
	data, err := pkt.Bytes()
	if err != nil {
		return err
	}
	// Unkomprimiert, damit auch einfache Werkzeuge die Metadaten finden.
	ctx.Catalog.Set("Metadata", d.Add(pdf.NewStream(pdf.D("Type", pdf.Name("Metadata"), "Subtype", pdf.Name("XML")), data)))
	return nil
}
