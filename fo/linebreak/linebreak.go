// Package linebreak bricht Absätze im Box-Glue-Penalty-Modell von Knuth und
// Plass um. Phase 0 liefert einen Greedy-Algorithmus. Die Schnittstelle ist so
// geschnitten, dass in Phase 1 Knuth-Plass und Silbentrennung (geflaggte
// Penalties) ohne Änderung der Aufrufer hinzukommen.
package linebreak

import (
	"errors"

	"github.com/tim-riep/pdx/areatree"
)

// Kind ist die Art eines Elements.
type Kind uint8

const (
	// Box ist unteilbarer Inhalt fester Breite.
	Box Kind = iota
	// Glue ist dehnbarer Zwischenraum und eine Umbruchstelle, wenn eine Box
	// vorausgeht.
	Glue
	// Penalty ist eine Umbruchstelle mit Kosten. Penalty <= ForcedBreak
	// erzwingt den Umbruch, Penalty >= Forbidden verbietet ihn.
	Penalty
)

const (
	// ForcedBreak erzwingt einen Umbruch.
	ForcedBreak = -10000
	// Forbidden verbietet einen Umbruch.
	Forbidden = 10000
)

// Item ist ein Element eines Absatzes.
type Item struct {
	Kind    Kind
	Width   areatree.Length
	Stretch areatree.Length
	Shrink  areatree.Length
	Penalty int
	// Flagged markiert Trennstellen mit Trennstrich (für Phase 1).
	Flagged bool
	// Payload verweist auf den Inhalt beim Aufrufer.
	Payload int
}

// Break ist eine gewählte Umbruchstelle.
type Break struct {
	// Index ist die Position des Glue oder Penalty, an der umbrochen wird.
	// Der letzte Break eines Absatzes zeigt auf len(items).
	Index int
	// Ratio ist das Anpassungsverhältnis der Zeile: positiv dehnen, negativ
	// stauchen, bezogen auf Stretch bzw. Shrink.
	Ratio float64
}

// LineWidth liefert die verfügbare Breite der Zeile line (ab 0).
type LineWidth func(line int) areatree.Length

// Breaker wählt Umbruchstellen.
type Breaker interface {
	Break(items []Item, width LineWidth) ([]Break, error)
}

// Greedy füllt jede Zeile so weit wie möglich. Eine Box, die allein breiter
// als die Zeile ist, steht in einer eigenen, überlaufenden Zeile.
type Greedy struct{}

// Break wählt die Umbruchstellen.
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
		var lw, lst, lsh areatree.Length // Maße bis zur letzten Umbruchstelle
		forced := false
		i := start
		for ; i < len(items); i++ {
			it := items[i]
			isBreak := (it.Kind == Glue && i > start && items[i-1].Kind == Box) ||
				(it.Kind == Penalty && it.Penalty < Forbidden)
			if isBreak {
				extra := areatree.Length(0)
				if it.Kind == Penalty {
					extra = it.Width // Trennstrich
				}
				if w+extra-sh <= avail || lastBreak < 0 && it.Kind == Penalty && it.Penalty <= ForcedBreak {
					lastBreak, lw, lst, lsh = i, w+extra, st, sh
				} else if lastBreak >= 0 {
					break
				} else {
					// Erste Umbruchstelle passt schon nicht: Überlauf hinnehmen.
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

// ratio berechnet das Anpassungsverhältnis. Letzte Zeilen werden nicht
// gedehnt, nur gestaucht, wenn nötig.
func ratio(avail, w, st, sh areatree.Length, last bool) float64 {
	switch {
	case w < avail && !last && st > 0:
		return float64(avail-w) / float64(st)
	case w > avail && sh > 0:
		return max(-1, float64(avail-w)/float64(sh))
	}
	return 0
}

// skipDiscardable überspringt Glue und Penalties am Zeilenanfang.
func skipDiscardable(items []Item, i int) int {
	for i < len(items) && items[i].Kind != Box {
		if items[i].Kind == Penalty && items[i].Penalty <= ForcedBreak {
			return i
		}
		i++
	}
	return i
}
