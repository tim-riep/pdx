package font

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math"
	"sort"
	"unicode/utf16"

	"github.com/boxesandglue/textshape/ot"
	"github.com/boxesandglue/textshape/subset"
	"github.com/tim-riep/pdx/pdf"
)

// Usage sammelt die in einem Dokument genutzten Glyphen einer Schrift und den
// Text, für den sie stehen.
type Usage struct {
	Face *Face
	text map[uint16]string
}

// NewUsage legt eine leere Nutzung an.
func NewUsage(f *Face) *Usage { return &Usage{Face: f, text: map[uint16]string{}} }

// Add vermerkt eine Glyphe. Der erste nicht leere Text gewinnt.
func (u *Usage) Add(gid uint16, text string) {
	if old, ok := u.text[gid]; !ok || (old == "" && text != "") {
		u.text[gid] = text
	}
}

// GIDs liefert die genutzten Glyphen sortiert.
func (u *Usage) GIDs() []uint16 {
	gs := make([]uint16, 0, len(u.text))
	for g := range u.text {
		gs = append(gs, g)
	}
	sort.Slice(gs, func(i, j int) bool { return gs[i] < gs[j] })
	return gs
}

// Embedded ist eine in ein PDF eingebettete Schrift.
type Embedded struct {
	// Ref verweist auf das Type0-Font-Dictionary.
	Ref pdf.Ref
	// BaseFont ist der Name mit Teilmengen-Präfix, etwa "ABCDEF+LiberationSans".
	BaseFont string
	// CID bildet Original-Glyphen auf CIDs (gleich neuen Glyphen) ab.
	CID map[uint16]uint16
	// Width ist die Breite je Original-Glyphe in Tausendstel Geviert, wie sie
	// im /W-Array steht.
	Width map[uint16]float64
}

// Embed bildet die Teilmenge, schreibt Schriftprogramm, Deskriptor, CIDFont,
// ToUnicode und Type0-Dictionary und liefert die Abbildung für die Ausgabe.
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
		// Eingebettet wird nur die CFF-Tabelle als FontFile3 /CIDFontType0C.
		check, err := ot.ParseFont(program, 0)
		if err != nil || !check.HasTable(ot.TagCFF) {
			return nil, fmt.Errorf("Teilmenge von %s: CFF-Tabelle konnte nicht gebildet werden", f.PostScript)
		}
		program, _ = check.TableData(ot.TagCFF)
	}

	e := &Embedded{CID: map[uint16]uint16{}, Width: map[uint16]float64{}}
	type cidEntry struct {
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
		entries = append(entries, cidEntry{cid: uint16(n), width: w, text: u.text[g]})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].cid < entries[j].cid })

	e.BaseFont = subsetTag(f, gids) + "+" + f.PostScript

	// Schriftprogramm.
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

	flags := 4 // symbolisch, für CIDFonts üblich
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

	// /W-Array: zusammenhängende CIDs in einer Gruppe.
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

	var cm []cmapEntry
	for _, en := range entries {
		if en.text != "" {
			cm = append(cm, cmapEntry{cid: en.cid, text: en.text})
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

// scale1000 rechnet Schrifteinheiten in Tausendstel Geviert um, gerundet auf
// drei Nachkommastellen, damit /W und Content Stream dieselben Werte nutzen.
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

// stemV schätzt die Stammstärke aus der Strichstärke. Der Wert ist für PDF
// Pflicht, wird aber von Betrachtern kaum genutzt.
func stemV(weight int) int {
	return 50 + weight*weight/4000
}

// subsetTag bildet das sechsbuchstabige Präfix aus dem Hash von Schrift und
// Glyphenmenge. Gleiche Eingaben ergeben gleiche Präfixe.
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

// toUnicode schreibt eine ToUnicode-CMap nach Adobe Technical Note 5411.
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
