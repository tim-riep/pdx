package font

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/boxesandglue/textshape/ot"
	"github.com/boxesandglue/textshape/subset"
	"github.com/tim-riep/pdx/pdf"
)

// Usage collects the glyphs of a font used in a document and the text
// they stand for.
type Usage struct {
	Face *Face
	text map[uint16]string
}

// NewUsage creates an empty usage.
func NewUsage(f *Face) *Usage { return &Usage{Face: f, text: map[uint16]string{}} }

// Add records a glyph. The first non-empty text wins.
func (u *Usage) Add(gid uint16, text string) {
	if old, ok := u.text[gid]; !ok || (old == "" && text != "") {
		u.text[gid] = text
	}
}

// GIDs returns the used glyphs sorted.
func (u *Usage) GIDs() []uint16 {
	gs := make([]uint16, 0, len(u.text))
	for g := range u.text {
		gs = append(gs, g)
	}
	sort.Slice(gs, func(i, j int) bool { return gs[i] < gs[j] })
	return gs
}

// Embedded is a font embedded in a PDF.
type Embedded struct {
	// Ref refers to the Type0 font dictionary.
	Ref pdf.Ref
	// BaseFont is the name with subset prefix, such as "ABCDEF+LiberationSans".
	BaseFont string
	// CID maps original glyphs to CIDs (equal to the new glyphs).
	CID map[uint16]uint16
	// Width is the width per original glyph in thousandths of an em, as it
	// appears in the /W array.
	Width map[uint16]float64
}

// Embed builds the subset, writes font program, descriptor, CIDFont,
// ToUnicode and Type0 dictionary and returns the mapping for the output.
func Embed(d *pdf.Document, u *Usage) (*Embedded, error) {
	f := u.Face
	if !f.Embeddable() {
		return nil, fmt.Errorf("Schrift %s darf laut Lizenz (OS/2 fsType) nicht eingebettet werden", f.PostScript)
	}
	gids := u.GIDs()
	in := subset.NewInput()
	in.Flags |= subset.FlagNoLayoutClosure | subset.FlagDropLayoutTables
	in.AddGlyph(0)
	for _, g := range gids {
		in.AddGlyph(ot.GlyphID(g))
	}
	plan, err := subset.CreatePlan(f.font, in)
	if err != nil {
		return nil, fmt.Errorf("Teilmenge von %s: %w", f.PostScript, err)
	}
	program, err := plan.Execute()
	if err != nil {
		return nil, fmt.Errorf("Teilmenge von %s: %w", f.PostScript, err)
	}
	if f.CFF {
		// Only the CFF table is embedded, as FontFile3 /CIDFontType0C.
		check, err := ot.ParseFont(program, 0)
		if err != nil || !check.HasTable(ot.TagCFF) {
			return nil, fmt.Errorf("Teilmenge von %s: CFF-Tabelle konnte nicht gebildet werden", f.PostScript)
		}
		program, _ = check.TableData(ot.TagCFF)
	}

	e := &Embedded{CID: map[uint16]uint16{}, Width: map[uint16]float64{}}
	type cidEntry struct {
		gid   uint16
		cid   uint16
		width float64
		text  string
	}
	var entries []cidEntry
	for _, g := range gids {
		n, ok := plan.MapGlyph(ot.GlyphID(g))
		if !ok {
			return nil, fmt.Errorf("Teilmenge von %s: Glyphe %d fehlt", f.PostScript, g)
		}
		w := f.scale1000(f.Advance(g))
		e.CID[g] = uint16(n)
		e.Width[g] = w
		entries = append(entries, cidEntry{gid: g, cid: uint16(n), width: w, text: u.text[g]})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].cid < entries[j].cid })

	e.BaseFont = subsetTag(f, gids) + "+" + f.PostScript

	// font program.
	var fileKey pdf.Name
	var file *pdf.Stream
	if f.CFF {
		fileKey = "FontFile3"
		file = pdf.NewFlateStream(pdf.D("Subtype", pdf.Name("CIDFontType0C")), program)
	} else {
		fileKey = "FontFile2"
		file = pdf.NewFlateStream(pdf.D("Length1", pdf.Int(len(program))), program)
	}
	fileRef := d.Add(file)

	flags := 4 // symbolic, usual for CIDFonts
	if f.FixedPitch {
		flags |= 1
	}
	if f.Italic {
		flags |= 64
	}
	desc := pdf.D(
		"Type", pdf.Name("FontDescriptor"),
		"FontName", pdf.Name(e.BaseFont),
		"Flags", pdf.Int(flags),
		"FontBBox", pdf.Array{f.r(f.BBox[0]), f.r(f.BBox[1]), f.r(f.BBox[2]), f.r(f.BBox[3])},
		"ItalicAngle", pdf.Real(f.ItalicAngle),
		"Ascent", f.r(f.Ascender),
		"Descent", f.r(f.Descender),
		"CapHeight", f.r(f.CapHeight),
		"StemV", pdf.Int(stemV(f.Weight)),
		string(fileKey), fileRef,
	)
	if f.XHeight != 0 {
		desc.Set("XHeight", f.r(f.XHeight))
	}
	descRef := d.Add(desc)

	// /W array: consecutive CIDs in one group.
	var w pdf.Array
	for i := 0; i < len(entries); {
		j := i + 1
		for j < len(entries) && entries[j].cid == entries[j-1].cid+1 {
			j++
		}
		var ws pdf.Array
		for _, en := range entries[i:j] {
			ws = append(ws, pdf.Real(en.width))
		}
		w = append(w, pdf.Int(entries[i].cid), ws)
		i = j
	}

	subtype := pdf.Name("CIDFontType2")
	if f.CFF {
		subtype = "CIDFontType0"
	}
	cidFont := pdf.D(
		"Type", pdf.Name("Font"),
		"Subtype", subtype,
		"BaseFont", pdf.Name(e.BaseFont),
		"CIDSystemInfo", pdf.D("Registry", pdf.String("Adobe"), "Ordering", pdf.String("Identity"), "Supplement", pdf.Int(0)),
		"FontDescriptor", descRef,
		"DW", pdf.Int(0),
		"W", w,
	)
	if !f.CFF {
		cidFont.Set("CIDToGIDMap", pdf.Name("Identity"))
	}
	cidRef := d.Add(cidFont)

	// Every glyph needs a Unicode mapping (mandatory for PDF/A U and A,
	// ISO 19005-3 6.2.11.7.2). Subsequent glyphs of a cluster carry no text
	// after shaping, they get the character that the font itself assigns
	// to them in the cmap. If that is missing too, the profile decides.
	var cm []cmapEntry
	var unmapped []string
	var reverse map[ot.GlyphID]rune
	for _, en := range entries {
		text := en.text
		if text == "" && en.gid != 0 {
			if reverse == nil {
				reverse = f.face.Cmap().CollectReverseMapping()
			}
			if r, ok := reverse[ot.GlyphID(en.gid)]; ok && r != 0 && r != 0xFEFF && r != 0xFFFE {
				text = string(r)
			}
		}
		if text == "" {
			if en.gid != 0 {
				unmapped = append(unmapped, strconv.Itoa(int(en.gid)))
			}
			continue
		}
		cm = append(cm, cmapEntry{cid: en.cid, text: text})
	}
	if len(unmapped) > 0 {
		if err := d.Use(pdf.UnmappedGlyph); err != nil {
			return nil, fmt.Errorf("Schrift %s, Glyphen %s: %w", f.PostScript, strings.Join(unmapped, ", "), err)
		}
	}
	toUni := d.Add(pdf.NewFlateStream(nil, toUnicode(cm)))

	e.Ref = d.Add(pdf.D(
		"Type", pdf.Name("Font"),
		"Subtype", pdf.Name("Type0"),
		"BaseFont", pdf.Name(e.BaseFont),
		"Encoding", pdf.Name("Identity-H"),
		"DescendantFonts", pdf.Array{cidRef},
		"ToUnicode", toUni,
	))
	return e, nil
}

// scale1000 converts font units to thousandths of an em, rounded to
// three decimal places, so that /W and the content stream use the same values.
func (f *Face) scale1000(v float64) float64 {
	return math.Round(v*1000/f.upem*1000) / 1000
}

func (f *Face) r(v int) pdf.Object {
	s := f.scale1000(float64(v))
	if s == math.Trunc(s) {
		return pdf.Int(int64(s))
	}
	return pdf.Real(s)
}

// stemV estimates the stem width from the weight. The value is mandatory
// for PDF but hardly used by viewers.
func stemV(weight int) int {
	return 50 + weight*weight/4000
}

// subsetTag builds the six-letter prefix from the hash of font and
// glyph set. Equal inputs yield equal prefixes.
func subsetTag(f *Face, gids []uint16) string {
	h := sha256.New()
	h.Write([]byte(f.key))
	for _, g := range gids {
		h.Write([]byte{byte(g >> 8), byte(g)})
	}
	sum := h.Sum(nil)
	tag := make([]byte, 6)
	for i := range tag {
		tag[i] = 'A' + sum[i]%26
	}
	return string(tag)
}

type cmapEntry struct {
	cid  uint16
	text string
}

// toUnicode writes a ToUnicode CMap according to Adobe Technical Note 5411.
func toUnicode(entries []cmapEntry) []byte {
	var b bytes.Buffer
	b.WriteString("/CIDInit /ProcSet findresource begin\n12 dict begin\nbegincmap\n" +
		"/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def\n" +
		"/CMapName /Adobe-Identity-UCS def\n/CMapType 2 def\n" +
		"1 begincodespacerange\n<0000> <FFFF>\nendcodespacerange\n")
	for i := 0; i < len(entries); i += 100 {
		chunk := entries[i:min(i+100, len(entries))]
		fmt.Fprintf(&b, "%d beginbfchar\n", len(chunk))
		for _, e := range chunk {
			fmt.Fprintf(&b, "<%04X> <", e.cid)
			for _, u := range utf16.Encode([]rune(e.text)) {
				fmt.Fprintf(&b, "%04X", u)
			}
			b.WriteString(">\n")
		}
		b.WriteString("endbfchar\n")
	}
	b.WriteString("endcmap\nCMapName currentdict /CMap defineresource pop\nend\nend\n")
	return b.Bytes()
}
