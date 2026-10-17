// Package font lädt TrueType- und OpenType-Schriften, formt Text über
// textshape, bildet Teilmengen und bettet sie als Type0-Schrift mit
// Identity-H, /W-Array und ToUnicode-CMap in ein PDF ein.
//
// Schriften werden ausnahmslos eingebettet. Auch die Standardschriften von
// PDF werden über die Registry auf metrikgleiche Liberation-Schriften
// abgebildet.
package font

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/boxesandglue/textshape/ot"
)

// Face ist eine geladene Schrift.
type Face struct {
	data   []byte
	index  int
	font   *ot.Font
	face   *ot.Face
	shaper *ot.Shaper
	mu     sync.Mutex // der Shaper ist nicht nebenläufig nutzbar

	key  string
	upem float64

	// PostScript ist der PostScript-Name, etwa "LiberationSans-Bold".
	PostScript string
	// Family ist der Familienname aus der name-Tabelle.
	Family string
	// Weight ist die Strichstärke nach OS/2, 100 bis 900.
	Weight int
	// Italic meldet einen kursiven oder schrägen Schnitt.
	Italic bool
	// CFF meldet CFF-Konturen (OpenType/CFF) statt TrueType-Konturen.
	CFF bool
	// FixedPitch meldet eine Schrift mit fester Breite.
	FixedPitch bool
	// Ascender, Descender, LineGap, CapHeight und XHeight in Schrifteinheiten.
	Ascender, Descender, LineGap, CapHeight, XHeight int
	// BBox ist die Schriftbox in Schrifteinheiten (xMin, yMin, xMax, yMax).
	BBox [4]int
	// ItalicAngle in Grad.
	ItalicAngle float64
	// FsType sind die Einbettungsrechte aus OS/2.
	FsType uint16
}

// Load lädt eine Schrift aus TTF-, OTF- oder TTC-Daten. index wählt bei
// Sammlungen die Schrift. Load gerät bei kaputten Daten nicht in Panik,
// sondern liefert einen Fehler.
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
		data:   data,
		index:  index,
		font:   fnt,
		face:   face,
		shaper: shaper,
		key:    hex.EncodeToString(sum[:8]),
		upem:   float64(face.Upem()),

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
		// Ober- und Unterlänge aus den Typo-Metriken, wie Apache FOP sie für
		// die Zeilenhöhe nutzt. hhea bleibt der Rückfall.
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

// PostScriptName liefert den PostScript-Namen und erfüllt areatree.FontSource.
func (f *Face) PostScriptName() string { return f.PostScript }

// Key identifiziert die Schriftdaten über einen Hash.
func (f *Face) Key() string { return f.key }

// Data liefert die Bytes der Schriftdatei, etwa um sie im Designer als
// FontFace anzuzeigen. Der Aufrufer darf sie nicht verändern.
func (f *Face) Data() []byte { return f.data }

// Index ist die Nummer der Schrift in einer Sammlung (TTC), sonst 0.
func (f *Face) Index() int { return f.index }

// Upem liefert die Schrifteinheiten pro Geviert.
func (f *Face) Upem() float64 { return f.upem }

// Embeddable meldet, ob die Lizenzbits in OS/2 das Einbetten erlauben. Nur
// "Restricted License" (fsType 0x0002 ohne weitere Bits) verbietet es.
func (f *Face) Embeddable() bool { return f.FsType&0x000f != 0x0002 }

// Advance liefert die Vorschubbreite einer Glyphe in Schrifteinheiten.
func (f *Face) Advance(gid uint16) float64 {
	return float64(f.face.HorizontalAdvance(ot.GlyphID(gid)))
}

// GlyphFor liefert die Glyphe eines Zeichens ohne Shaping. ok ist false,
// wenn die Schrift das Zeichen nicht enthält.
func (f *Face) GlyphFor(r rune) (gid uint16, ok bool) {
	g, ok := f.face.Cmap().Lookup(ot.Codepoint(r))
	if !ok || g == 0 {
		return 0, false
	}
	return uint16(g), true
}

// HasRune meldet, ob die Schrift ein Zeichen enthält.
func (f *Face) HasRune(r rune) bool {
	_, ok := f.GlyphFor(r)
	return ok
}
