// Package linebreak breaks paragraphs in the box-glue-penalty model of Knuth
// and Plass. Phase 0 provides a greedy algorithm. The interface is designed
// so that in phase 1 Knuth-Plass and hyphenation (flagged
// penalties) can be added without changing the callers.
package linebreak

import (
	"errors"

	"github.com/tim-riep/pdx/areatree"
)

// Kind is the kind of an item.
type Kind uint8

const (
	// Box is indivisible content of fixed width.
	Box Kind = iota
	// Glue is stretchable space and a break point if a box
	// precedes it.
	Glue
	// Penalty is a break point with a cost. Penalty <= ForcedBreak
	// forces the break, Penalty >= Forbidden forbids it.
	Penalty
)

const (
	// ForcedBreak forces a break.
	ForcedBreak = -10000
	// Forbidden forbids a break.
	Forbidden = 10000
)

// Item is an item of a paragraph.
type Item struct {
	Kind    Kind
	Width   areatree.Length
	Stretch areatree.Length
	Shrink  areatree.Length
	Penalty int
	// Flagged marks hyphenation points with a hyphen (for phase 1).
	Flagged bool
	// Payload refers to the content at the caller.
	Payload int
}

// Break is a chosen break point.
type Break struct {
	// Index is the position of the glue or penalty at which the line breaks.
	// The last break of a paragraph points to len(items).
	Index int
	// Ratio is the adjustment ratio of the line: positive stretches, negative
	// shrinks, relative to Stretch or Shrink.
	Ratio float64
}

// LineWidth returns the available width of line line (from 0).
type LineWidth func(line int) areatree.Length

// Breaker chooses break points.
type Breaker interface {
	Break(items []Item, width LineWidth) ([]Break, error)
}

// Greedy fills each line as far as possible. A box that alone is wider
// than the line stands in its own overflowing line.
type Greedy struct{}

// Break chooses the break points.
func (Greedy) Break(items []Item, width LineWidth) ([]Break, error) {
	if width == nil {
		return nil, errors.New("linebreak: keine Zeilenbreite")
	}
	var breaks []Break
	line := 0
	start := 0
	for start < len(items) {
		start = skipDiscardable(items, start)
		if start >= len(items) {
			break
		}
		avail := width(line)
		var w, st, sh areatree.Length
		lastBreak := -1
		var lw, lst, lsh areatree.Length // measures up to the last break point
		forced := false
		i := start
		for ; i < len(items); i++ {
			it := items[i]
			isBreak := (it.Kind == Glue && i > start && items[i-1].Kind == Box) ||
				(it.Kind == Penalty && it.Penalty < Forbidden)
			if isBreak {
				extra := areatree.Length(0)
				if it.Kind == Penalty {
					extra = it.Width // hyphen
				}
				if w+extra-sh <= avail || lastBreak < 0 && it.Kind == Penalty && it.Penalty <= ForcedBreak {
					lastBreak, lw, lst, lsh = i, w+extra, st, sh
				} else if lastBreak >= 0 {
					break
				} else {
					// The first break point already does not fit: accept the overflow.
					lastBreak, lw, lst, lsh = i, w+extra, st, sh
					break
				}
				if it.Kind == Penalty && it.Penalty <= ForcedBreak {
					forced = true
					break
				}
			}
			switch it.Kind {
			case Box, Glue:
				w += it.Width
				st += it.Stretch
				sh += it.Shrink
			}
			if w-sh > avail && lastBreak >= 0 && !forced {
				break
			}
		}
		if i >= len(items) && !forced && (w-sh <= avail || lastBreak < 0) {
			breaks = append(breaks, Break{Index: len(items), Ratio: ratio(avail, w, st, sh, true)})
			break
		}
		if lastBreak < 0 {
			lastBreak, lw, lst, lsh = len(items), w, st, sh
		}
		last := lastBreak == len(items) || (items[lastBreak].Kind == Penalty && items[lastBreak].Penalty <= ForcedBreak)
		breaks = append(breaks, Break{Index: lastBreak, Ratio: ratio(avail, lw, lst, lsh, last)})
		line++
		start = lastBreak + 1
	}
	return breaks, nil
}

// ratio computes the adjustment ratio. Last lines are not
// stretched, only shrunk if needed.
func ratio(avail, w, st, sh areatree.Length, last bool) float64 {
	switch {
	case w < avail && !last && st > 0:
		return float64(avail-w) / float64(st)
	case w > avail && sh > 0:
		return max(-1, float64(avail-w)/float64(sh))
	}
	return 0
}

// skipDiscardable skips glue and penalties at the start of a line.
func skipDiscardable(items []Item, i int) int {
	for i < len(items) && items[i].Kind != Box {
		if items[i].Kind == Penalty && items[i].Penalty <= ForcedBreak {
			return i
		}
		i++
	}
	return i
}
