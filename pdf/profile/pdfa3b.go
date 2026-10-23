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

// PDFA is PDF/A-2 or PDF/A-3 (ISO 19005-2:2011, ISO 19005-3:2012) at
// level B, U or A, and PDF/A-4 (ISO 19005-4:2020) without a level or as
// 4f with arbitrary attachments. Level A requires Tagged PDF (6.7), which
// the renderer switches on through the claim. PDF/A-4 is based on PDF 2.0.
// The clauses of both parts are numbered the same, PDF/A-3
// additionally allows arbitrary attachments (6.8).
type PDFA struct {
	Part  int
	Level string
}

// Name returns for example "pdfa-3u".
func (p PDFA) Name() string { return fmt.Sprintf("pdfa-%d%s", p.Part, strings.ToLower(p.Level)) }

// Allow checks a feature against the part and the level.
func (p PDFA) Allow(f pdf.Feature) error { return allowPDFA(p, p.Part, p.Level, f) }

func (p PDFA) claim() Claim {
	v := "1.7"
	if p.Part == 4 {
		v = "2.0"
	}
	return Claim{Version: v, PDFA: p.Part, Level: p.Level}
}

// Prepare adds the output intent and XMP metadata.
func (p PDFA) Prepare(ctx *Context) error { return prepareClaim(ctx, p.claim()) }

// CheckAttachment requires under PDF/A-2 that an attachment is itself a
// PDF/A-1 or PDF/A-2 document (ISO 19005-2 6.8), under PDF/A-4 without a
// level a PDF/A-1, -2 or -4 (ISO 19005-4 6.9). PDF/A-3 and 4f allow
// arbitrary attachments.
func (p PDFA) CheckAttachment(name string, data []byte) error {
	switch {
	case p.Part == 2:
		return checkPDFAAttachment(p.Name(), name, data, "ISO 19005-2 6.8", "1", "2")
	case p.Part == 4 && p.Level == "":
		return checkPDFAAttachment(p.Name(), name, data, "ISO 19005-4 6.9", "1", "2", "4")
	}
	return nil
}

// pdfaClauses are the references per part. ISO 19005-4 is restructured,
// the numbers come from the rules of veraPDF 1.30.3.
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

// allowPDFA are the rules of PDF/A-2, PDF/A-3 and PDF/A-4. The levels U
// and A require a Unicode mapping per glyph (6.2.11.7.2), PDF/A-4
// has no level U.
func allowPDFA(p pdf.Policy, part int, level string, f pdf.Feature) error {
	cs := pdfaClauses[part]
	if cs == nil {
		cs = pdfaClauses[2] // part 3 is numbered like part 2
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

// pdfaPart finds the identifier pdfaid:part in the XMP of a PDF.
var pdfaPart = regexp.MustCompile(`pdfaid:part(?:>|="|='|\s*=\s*["'])\s*([0-9])`)

// checkPDFAAttachment checks whether an attachment is a PDF/A document of one
// of the allowed parts.
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

// PDFA3B is PDF/A-3b according to ISO 19005-3:2012, conformance level B.
//
// The profile refuses encryption (6.1.3), LZW (6.1.7.2), external
// content (6.1.7.1, 6.8) and unembedded fonts (6.2.11.4.1).
// Transparency and file attachments are allowed in PDF/A-3. Prepare adds the
// output intent GTS_PDFA1 with an sRGB profile (6.2.3) and the XMP metadata
// with pdfaid (6.6.2), matching the info dictionary (6.6.2.3).
type PDFA3B struct{}

// Name returns "pdfa-3b".
func (PDFA3B) Name() string { return "pdfa-3b" }

// Allow checks a feature against ISO 19005-3.
func (p PDFA3B) Allow(f pdf.Feature) error { return allowPDFA(p, 3, "B", f) }

func (PDFA3B) claim() Claim { return Claim{Version: "1.7", PDFA: 3, Level: "B"} }

// Prepare adds the output intent and XMP metadata.
func (p PDFA3B) Prepare(ctx *Context) error { return prepareClaim(ctx, p.claim()) }

// pdfaRev is the year of the edition for pdfaid:rev, required from PDF/A-4 on
// (ISO 19005-4 6.6.2.2).
var pdfaRev = map[int]string{4: "2020"}

// uaRev is the year of the edition for pdfuaid:rev, required in PDF/UA-2
// (ISO 14289-2 5).
var uaRev = map[int]string{2: "2024"}

// prepareClaim adds the required objects of a claim: output intent
// for PDF/A and the XMP metadata with the identifiers of all parts.
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
		// The title appears instead of the file name (ISO 14289-1 7.1).
		ctx.Catalog.Set("ViewerPreferences", pdf.D("DisplayDocTitle", pdf.Bool(true)))
	}

	m := ctx.Meta
	pkt := &xmp.Packet{
		PDFAPart: c.PDFA, PDFAConformance: c.Level, UAPart: c.UA,
		PDFARev: pdfaRev[c.PDFA], UARev: uaRev[c.UA],
		Title: m.Title, Author: m.Author, Subject: m.Subject, Keywords: m.Keywords,
		Creator: m.Creator, Producer: ctx.Producer,
		Created: ctx.Date, Modified: ctx.Date,
		// PDF/A-4 no longer requires a description of custom schemas.
		Schemas: m.XMP, Extension: c.PDFA > 0 && c.PDFA < 4,
	}
	data, err := pkt.Bytes()
	if err != nil {
		return err
	}
	// Uncompressed, so that simple tools also find the metadata.
	ctx.Catalog.Set("Metadata", d.Add(pdf.NewStream(pdf.D("Type", pdf.Name("Metadata"), "Subtype", pdf.Name("XML")), data)))
	return nil
}
