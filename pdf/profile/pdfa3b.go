package profile

import (
	"bytes"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/tim-riep/pdx/icc"
	"github.com/tim-riep/pdx/pdf"
	"github.com/tim-riep/pdx/pdf/xmp"
)

func init() {
	register("pdfa-3b", func() Profile { return PDFA3B{} })
	register("pdfa-2b", func() Profile { return PDFA{Part: 2, Level: "B"} })
	register("pdfa-2u", func() Profile { return PDFA{Part: 2, Level: "U"} })
	register("pdfa-3u", func() Profile { return PDFA{Part: 3, Level: "U"} })
	register("pdfa-3a", func() Profile { return PDFA{Part: 3, Level: "A"} })
	register("pdfa-4", func() Profile { return PDFA{Part: 4} })
	register("pdfa-4f", func() Profile { return PDFA{Part: 4, Level: "F"} })
}

// PDFA ist PDF/A-2 oder PDF/A-3 (ISO 19005-2:2011, ISO 19005-3:2012) in der
// Stufe B, U oder A sowie PDF/A-4 (ISO 19005-4:2020) ohne Stufe oder als
// 4f mit beliebigen Anhängen. Stufe A verlangt Tagged PDF (6.7), das
// schaltet der Renderer über den Anspruch ein. PDF/A-4 beruht auf PDF 2.0. Die Klauseln beider Teile sind gleich nummeriert, PDF/A-3
// erlaubt zusätzlich beliebige Anhänge (6.8).
type PDFA struct {
	Part  int
	Level string
}

// Name liefert etwa "pdfa-3u".
func (p PDFA) Name() string { return fmt.Sprintf("pdfa-%d%s", p.Part, strings.ToLower(p.Level)) }

// Allow prüft eine Funktion gegen den Teil und die Stufe.
func (p PDFA) Allow(f pdf.Feature) error { return allowPDFA(p, p.Part, p.Level, f) }

func (p PDFA) claim() Claim {
	v := "1.7"
	if p.Part == 4 {
		v = "2.0"
	}
	return Claim{Version: v, PDFA: p.Part, Level: p.Level}
}

// Prepare ergänzt Output Intent und XMP-Metadaten.
func (p PDFA) Prepare(ctx *Context) error { return prepareClaim(ctx, p.claim()) }

// CheckAttachment verlangt unter PDF/A-2, dass ein Anhang selbst ein
// PDF/A-1- oder PDF/A-2-Dokument ist (ISO 19005-2 6.8), unter PDF/A-4 ohne
// Stufe ein PDF/A-1, -2 oder -4 (ISO 19005-4 6.9). PDF/A-3 und 4f erlauben
// beliebige Anhänge.
func (p PDFA) CheckAttachment(name string, data []byte) error {
	switch {
	case p.Part == 2:
		return checkPDFAAttachment(p.Name(), name, data, "ISO 19005-2 6.8", "1", "2")
	case p.Part == 4 && p.Level == "":
		return checkPDFAAttachment(p.Name(), name, data, "ISO 19005-4 6.9", "1", "2", "4")
	}
	return nil
}

// pdfaClauses sind die Fundstellen je Teil. ISO 19005-4 ist neu gegliedert,
// die Nummern stammen aus den Regeln von veraPDF 1.30.3.
var pdfaClauses = map[int]map[pdf.Feature]string{
	2: {
		pdf.Encryption: "6.1.3", pdf.LZW: "6.1.7.2", pdf.ExternalContent: "6.1.7.1",
		pdf.UnembeddedFont: "6.2.11.4.1", pdf.JavaScript: "6.6.1", pdf.XFA: "6.4.2", pdf.UnmappedGlyph: "6.2.11.7.2",
	},
	4: {
		pdf.Encryption: "6.1.3", pdf.LZW: "6.1.6.2", pdf.ExternalContent: "6.1.6.1",
		pdf.UnembeddedFont: "6.2.10.4.1", pdf.XFA: "6.4.2",
	},
}

var pdfaReasons = map[pdf.Feature]string{
	pdf.Encryption:      "verbietet das Encrypt-Dictionary",
	pdf.LZW:             "verbietet LZWDecode",
	pdf.ExternalContent: "verbietet Streams in externen Dateien",
	pdf.UnembeddedFont:  "verlangt eingebettete Schriften",
	pdf.JavaScript:      "verbietet JavaScript",
	pdf.XFA:             "verbietet XFA",
	pdf.UnmappedGlyph:   "verlangt für jede Glyphe eine Unicode-Zuordnung",
}

// allowPDFA sind die Regeln von PDF/A-2, PDF/A-3 und PDF/A-4. Die Stufen U
// und A verlangen eine Unicode-Zuordnung je Glyphe (6.2.11.7.2), PDF/A-4
// kennt keine Stufe U.
func allowPDFA(p pdf.Policy, part int, level string, f pdf.Feature) error {
	cs := pdfaClauses[part]
	if cs == nil {
		cs = pdfaClauses[2] // Teil 3 ist wie Teil 2 nummeriert
	}
	c, ok := cs[f]
	if !ok {
		return nil
	}
	if f == pdf.UnmappedGlyph && level != "U" && level != "A" {
		return nil
	}
	return deny(p, f, fmt.Sprintf("ISO 19005-%d %s %s", part, c, pdfaReasons[f]))
}

// pdfaPart findet die Kennung pdfaid:part im XMP eines PDF.
var pdfaPart = regexp.MustCompile(`pdfaid:part(?:>|="|='|\s*=\s*["'])\s*([0-9])`)

// checkPDFAAttachment prüft, ob ein Anhang ein PDF/A-Dokument eines der
// erlaubten Teile ist.
func checkPDFAAttachment(profile, name string, data []byte, clause string, parts ...string) error {
	allowed := "PDF/A-" + strings.Join(parts, ", PDF/A-")
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		return fmt.Errorf("Profil %s: Anhang %q ist kein PDF, %s erlaubt nur Anhänge in %s", profile, name, clause, allowed)
	}
	m := pdfaPart.FindSubmatch(data)
	if m == nil || !slices.Contains(parts, string(m[1])) {
		return fmt.Errorf("Profil %s: Anhang %q ist kein Dokument in %s (%s)", profile, name, allowed, clause)
	}
	return nil
}

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
func (p PDFA3B) Allow(f pdf.Feature) error { return allowPDFA(p, 3, "B", f) }

func (PDFA3B) claim() Claim { return Claim{Version: "1.7", PDFA: 3, Level: "B"} }

// Prepare ergänzt Output Intent und XMP-Metadaten.
func (p PDFA3B) Prepare(ctx *Context) error { return prepareClaim(ctx, p.claim()) }

// pdfaRev ist das Jahr der Ausgabe für pdfaid:rev, Pflicht ab PDF/A-4
// (ISO 19005-4 6.6.2.2).
var pdfaRev = map[int]string{4: "2020"}

// uaRev ist das Jahr der Ausgabe für pdfuaid:rev, Pflicht in PDF/UA-2
// (ISO 14289-2 5).
var uaRev = map[int]string{2: "2024"}

// prepareClaim ergänzt die Pflichtobjekte eines Anspruchs: Output Intent
// für PDF/A und die XMP-Metadaten mit den Kennungen aller Teile.
func prepareClaim(ctx *Context, c Claim) error {
	if c.PDFA == 0 && c.UA == 0 && c.Version != "2.0" {
		return nil
	}
	d := ctx.Doc
	if c.PDFA > 0 {
		profile := d.Add(pdf.NewFlateStream(pdf.D("N", pdf.Int(3)), icc.SRGB()))
		ctx.Catalog.Set("OutputIntents", pdf.Array{pdf.D(
			"Type", pdf.Name("OutputIntent"),
			"S", pdf.Name("GTS_PDFA1"),
			"OutputConditionIdentifier", pdf.String(icc.SRGBDescription),
			"Info", pdf.String(icc.SRGBDescription),
			"RegistryName", pdf.String("http://www.color.org"),
			"DestOutputProfile", profile,
		)})
	}
	if c.UA > 0 {
		// Der Titel erscheint statt des Dateinamens (ISO 14289-1 7.1).
		ctx.Catalog.Set("ViewerPreferences", pdf.D("DisplayDocTitle", pdf.Bool(true)))
	}

	m := ctx.Meta
	pkt := &xmp.Packet{
		PDFAPart: c.PDFA, PDFAConformance: c.Level, UAPart: c.UA,
		PDFARev: pdfaRev[c.PDFA], UARev: uaRev[c.UA],
		Title: m.Title, Author: m.Author, Subject: m.Subject, Keywords: m.Keywords,
		Creator: m.Creator, Producer: ctx.Producer,
		Created: ctx.Date, Modified: ctx.Date,
		// PDF/A-4 verlangt keine Beschreibung eigener Schemata mehr.
		Schemas: m.XMP, Extension: c.PDFA > 0 && c.PDFA < 4,
	}
	data, err := pkt.Bytes()
	if err != nil {
		return err
	}
	// Unkomprimiert, damit auch einfache Werkzeuge die Metadaten finden.
	ctx.Catalog.Set("Metadata", d.Add(pdf.NewStream(pdf.D("Type", pdf.Name("Metadata"), "Subtype", pdf.Name("XML")), data)))
	return nil
}
