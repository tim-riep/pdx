package font

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/boxesandglue/textshape/ot"
	"github.com/tim-riep/pdx/areatree"
)

// ShapeOptions controls shaping.
type ShapeOptions struct {
	// Features toggles OpenType features, such as "-liga" or "smcp".
	// If not given, the textshape defaults apply (kern, liga and others).
	Features []string
}

// MissingGlyphError reports characters the font does not contain. PDF/A
// forbids the use of the .notdef glyph, so this is an error.
type MissingGlyphError struct {
	Font  string
	Runes []rune
}

func (e *MissingGlyphError) Error() string {
	parts := make([]string, len(e.Runes))
	for i, r := range e.Runes {
		parts[i] = fmt.Sprintf("U+%04X %q", r, r)
	}
	return fmt.Sprintf("Schrift %s enthält die Zeichen %s nicht", e.Font, strings.Join(parts, ", "))
}

// Shape shapes text at a font size into positioned glyphs. The first
// glyph of a cluster carries the text of the cluster, subsequent glyphs stay empty.
// Text in phase 0 always runs left to right.
func (f *Face) Shape(text string, size areatree.Length, opts ShapeOptions) (glyphs []areatree.Glyph, err error) {
	defer func() {
		if r := recover(); r != nil {
			glyphs, err = nil, fmt.Errorf("Shaping mit %s fehlgeschlagen: %v", f.PostScript, r)
		}
	}()
	if text == "" {
		return nil, nil
	}
	runes := []rune(text)
	var missing []rune
	for _, r := range runes {
		if isIgnorable(r) {
			continue
		}
		if !f.HasRune(r) && !containsRune(missing, r) {
			missing = append(missing, r)
		}
	}
	if len(missing) > 0 {
		return nil, &MissingGlyphError{Font: f.PostScript, Runes: missing}
	}

	buf := ot.NewBuffer()
	buf.AddString(text)
	buf.SetDirection(ot.DirectionLTR)
	buf.GuessSegmentProperties()
	buf.Flags = ot.BufferFlagBOT | ot.BufferFlagEOT | ot.BufferFlagRemoveDefaultIgnorables
	var feats []ot.Feature
	if len(opts.Features) > 0 {
		feats = ot.ParseFeatures(strings.Join(opts.Features, ","))
	}
	sh, err := f.shaper()
	if err != nil {
		return nil, fmt.Errorf("Shaper für %s: %w", f.PostScript, err)
	}
	sh.Shape(buf, feats)
	f.shapers.Put(sh)

	// Determine cluster boundaries so that every glyph gets its text.
	starts := make([]int, 0, len(buf.Info))
	seen := map[int]bool{}
	for _, gi := range buf.Info {
		if !seen[gi.Cluster] {
			seen[gi.Cluster] = true
			starts = append(starts, gi.Cluster)
		}
	}
	sort.Ints(starts)
	clusterEnd := map[int]int{}
	for i, s := range starts {
		end := len(runes)
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		clusterEnd[s] = end
	}

	scale := float64(size) / f.upem
	out := make([]areatree.Glyph, len(buf.Info))
	done := map[int]bool{}
	for i, gi := range buf.Info {
		if gi.GlyphID == 0 {
			return nil, &MissingGlyphError{Font: f.PostScript, Runes: []rune{runeAt(runes, gi.Cluster)}}
		}
		p := buf.Pos[i]
		g := areatree.Glyph{
			ID:      uint16(gi.GlyphID),
			Advance: areatree.Length(math.Round(float64(p.XAdvance) * scale)),
			XOffset: areatree.Length(math.Round(float64(p.XOffset) * scale)),
			YOffset: areatree.Length(math.Round(-float64(p.YOffset) * scale)), // area tree: y downward
		}
		if !done[gi.Cluster] {
			done[gi.Cluster] = true
			g.Text = string(runes[gi.Cluster:clusterEnd[gi.Cluster]])
		}
		out[i] = g
	}
	return out, nil
}

// Metrics returns ascent, descent (positive) and line spacing of the font at
// a given size.
func (f *Face) Metrics(size areatree.Length) (ascent, descent, lineGap areatree.Length) {
	s := float64(size) / f.upem
	return areatree.Length(math.Round(float64(f.Ascender) * s)),
		areatree.Length(math.Round(-float64(f.Descender) * s)),
		areatree.Length(math.Round(float64(f.LineGap) * s))
}

func isIgnorable(r rune) bool {
	return r == 0x200b || r == 0x200c || r == 0x200d || r == 0x00ad || r == 0xfeff
}

func containsRune(rs []rune, r rune) bool {
	for _, x := range rs {
		if x == r {
			return true
		}
	}
	return false
}

func runeAt(rs []rune, i int) rune {
	if i >= 0 && i < len(rs) {
		return rs[i]
	}
	return 0xfffd
}
