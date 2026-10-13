package font

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/boxesandglue/textshape/ot"
	"github.com/tim-riep/pdx/areatree"
)

// ShapeOptions steuert das Shaping.
type ShapeOptions struct {
	// Features schaltet OpenType-Features, etwa "-liga" oder "smcp".
	// Ohne Angabe gelten die Vorgaben von textshape (kern, liga und andere).
	Features []string
}

// MissingGlyphError meldet Zeichen, die die Schrift nicht enthält. PDF/A
// verbietet die Verwendung der .notdef-Glyphe, deshalb ist das ein Fehler.
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

// Shape formt Text in einer Schriftgröße zu positionierten Glyphen. Die erste
// Glyphe eines Clusters trägt den Text des Clusters, Folgeglyphen bleiben leer.
// Text in Phase 0 läuft immer von links nach rechts.
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
	f.mu.Lock()
	defer f.mu.Unlock()
	f.shaper.Shape(buf, feats)

	// Clustergrenzen bestimmen, damit jede Glyphe ihren Text erhält.
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
			YOffset: areatree.Length(math.Round(-float64(p.YOffset) * scale)), // Area Tree: y nach unten
		}
		if !done[gi.Cluster] {
			done[gi.Cluster] = true
			g.Text = string(runes[gi.Cluster:clusterEnd[gi.Cluster]])
		}
		out[i] = g
	}
	return out, nil
}

// Metrics liefert Ascent, Descent (positiv) und Zeilenabstand der Schrift in
// einer Größe.
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
