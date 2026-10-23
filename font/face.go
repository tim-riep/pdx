// Package font loads TrueType and OpenType fonts, shapes text via
// textshape, builds subsets and embeds them into a PDF as Type0 fonts with
// Identity-H, /W array and ToUnicode CMap.
//
// Fonts are embedded without exception. Even the standard fonts of
// PDF are mapped via the registry to metric-compatible Liberation
// fonts.
package font

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/boxesandglue/textshape/ot"
)

// Face is a loaded font.
type Face struct {
	data  []byte
	index int
	font  *ot.Font
	face  *ot.Face
	// shapers holds shapers for reuse. A shaper cannot be used
	// concurrently, with a pool parallel runs shape the same font
	// at the same time instead of one after the other.
	shapers sync.Pool

	key  string
	upem float64

	// PostScript is the PostScript name, such as "LiberationSans-Bold".
	PostScript string
	// Family is the family name from the name table.
	Family string
	// Weight is the weight according to OS/2, 100 to 900.
	Weight int
	// Italic reports an italic or oblique face.
	Italic bool
	// CFF reports CFF outlines (OpenType/CFF) instead of TrueType outlines.
	CFF bool
	// FixedPitch reports a fixed-width font.
	FixedPitch bool
	// Ascender, Descender, LineGap, CapHeight and XHeight in font units.
	Ascender, Descender, LineGap, CapHeight, XHeight int
	// BBox is the font box in font units (xMin, yMin, xMax, yMax).
	BBox [4]int
	// ItalicAngle in Grad.
	ItalicAngle float64
	// FsType holds the embedding permissions from OS/2.
	FsType uint16
}

// Load loads a font from TTF, OTF or TTC data. For collections index
// selects the font. Load does not panic on broken data but
// returns an error.
func Load(data []byte, index int) (f *Face, err error) {
	defer func() {
		if r := recover(); r != nil {
			f, err = nil, fmt.Errorf("Schrift nicht lesbar: %v", r)
		}
	}()
	fnt, err := ot.ParseFont(data, index)
	if err != nil {
		return nil, fmt.Errorf("Schrift nicht lesbar: %w", err)
	}
	for _, tag := range []ot.Tag{ot.TagHead, ot.TagHhea, ot.TagHmtx, ot.TagCmap} {
		if !fnt.HasTable(tag) {
			return nil, fmt.Errorf("Schrift unvollständig: Tabelle %s fehlt", tag)
		}
	}
	if !fnt.HasTable(ot.TagGlyf) && !fnt.HasTable(ot.TagCFF) {
		return nil, fmt.Errorf("Schrift hat weder glyf- noch CFF-Konturen (CFF2 und Bitmap-Schriften werden nicht unterstützt)")
	}
	face, err := ot.NewFace(fnt)
	if err != nil {
		return nil, fmt.Errorf("Schrift nicht lesbar: %w", err)
	}
	if face.Upem() == 0 || face.Cmap() == nil {
		return nil, fmt.Errorf("Schrift unvollständig: unitsPerEm oder cmap fehlt")
	}
	shaper, err := ot.NewShaperFromFace(face)
	if err != nil {
		return nil, fmt.Errorf("Shaper nicht initialisierbar: %w", err)
	}
	sum := sha256.Sum256(data)
	f = &Face{
		data:  data,
		index: index,
		font:  fnt,
		face:  face,
		key:   hex.EncodeToString(sum[:8]),
		upem:  float64(face.Upem()),

		PostScript:  face.PostscriptName(),
		Family:      face.FamilyName(),
		Weight:      int(face.WeightClass()),
		Italic:      face.IsItalic() || face.ItalicAngle() != 0,
		CFF:         face.IsCFF(),
		FixedPitch:  face.IsFixedPitch(),
		Ascender:    int(face.Ascender()),
		Descender:   int(face.Descender()),
		LineGap:     int(face.LineGap()),
		CapHeight:   int(face.CapHeight()),
		XHeight:     int(face.XHeight()),
		ItalicAngle: float64(face.ItalicAngle()) / 65536,
	}
	f.shapers.Put(shaper)
	x0, y0, x1, y1 := face.BBox()
	f.BBox = [4]int{int(x0), int(y0), int(x1), int(y1)}
	if f.Weight == 0 {
		f.Weight = 400
	}
	if f.PostScript == "" {
		f.PostScript = "Schrift-" + f.key
	}
	if f.CapHeight == 0 {
		f.CapHeight = f.Ascender
	}
	if os2, err := fnt.TableData(ot.TagOS2); err == nil && len(os2) >= 10 {
		f.FsType = binary.BigEndian.Uint16(os2[8:10])
		// Ascender and descender from the typo metrics, as Apache FOP uses them for
		// the line height. hhea remains the fallback.
		if len(os2) >= 74 {
			asc := int(int16(binary.BigEndian.Uint16(os2[68:70])))
			desc := int(int16(binary.BigEndian.Uint16(os2[70:72])))
			gap := int(int16(binary.BigEndian.Uint16(os2[72:74])))
			if asc > 0 && desc < 0 {
				f.Ascender, f.Descender, f.LineGap = asc, desc, gap
			}
		}
	}
	return f, nil
}

// PostScriptName returns the PostScript name and satisfies areatree.FontSource.
func (f *Face) PostScriptName() string { return f.PostScript }

// Key identifies the font data by a hash.
func (f *Face) Key() string { return f.key }

// Data returns the bytes of the font file, for example to show it in the
// designer as a FontFace. The caller must not modify them.
func (f *Face) Data() []byte { return f.data }

// Index is the number of the font in a collection (TTC), else 0.
func (f *Face) Index() int { return f.index }

// Upem returns the font units per em.
func (f *Face) Upem() float64 { return f.upem }

// Embeddable reports whether the license bits in OS/2 allow embedding. Only
// "Restricted License" (fsType 0x0002 without further bits) forbids it.
func (f *Face) Embeddable() bool { return f.FsType&0x000f != 0x0002 }

// Advance returns the advance width of a glyph in font units.
func (f *Face) Advance(gid uint16) float64 {
	return float64(f.face.HorizontalAdvance(ot.GlyphID(gid)))
}

// GlyphFor returns the glyph of a character without shaping. ok is false
// if the font does not contain the character.
func (f *Face) GlyphFor(r rune) (gid uint16, ok bool) {
	g, ok := f.face.Cmap().Lookup(ot.Codepoint(r))
	if !ok || g == 0 {
		return 0, false
	}
	return uint16(g), true
}

// HasRune reports whether the font contains a character.
func (f *Face) HasRune(r rune) bool {
	_, ok := f.GlyphFor(r)
	return ok
}

// shaper returns a free shaper. A new one gets its own
// copy of the parsed font so that shapers share no state.
func (f *Face) shaper() (*ot.Shaper, error) {
	if s, ok := f.shapers.Get().(*ot.Shaper); ok {
		return s, nil
	}
	fnt, err := ot.ParseFont(f.data, f.index)
	if err != nil {
		return nil, err
	}
	face, err := ot.NewFace(fnt)
	if err != nil {
		return nil, err
	}
	return ot.NewShaperFromFace(face)
}
