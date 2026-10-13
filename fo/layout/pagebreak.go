package layout

import "github.com/tim-riep/pdx/areatree"

// breakPage wählt das Ende der Seite, die bei start beginnt. Es liefert den
// Index der gewählten Penalty oder len(items), wenn der Rest passt.
//
// Unter allen Penalties, bis zu denen der Inhalt in avail passt, gewinnt die
// mit den geringsten Kosten, bei Gleichstand die späteste. Keeps mit
// ganzzahliger Stärke werden so in aufsteigender Stärke gelockert (XSL 1.1
// 4.8). Gibt es nur verbotene Stellen, wird die späteste genommen, statt
// Inhalt zu verlieren. Passt schon die erste Box nicht, bricht die Seite an
// der ersten möglichen Stelle danach.
func breakPage(items []item, start int, avail areatree.Length) int {
	var h areatree.Length
	seenBox := false
	best, bestPen := -1, 0
	var last *item
	for i := start; i < len(items); i++ {
		it := items[i]
		switch it.kind {
		case itBox:
			if !seenBox {
				// Beginnt die Seite im Rumpf einer Tabelle, steht der
				// wiederholte Kopf darüber.
				if r := it.row; r != nil && r.tbl != nil && r.sec == secBody && !r.tbl.omitHeader &&
					!(r == r.tbl.firstBody && it.slice == 0) {
					h += r.tbl.headerH
				}
			}
			h += it.h
			seenBox = true
			last = &items[i]
			if h > avail && best >= 0 {
				return best
			}
		case itGlue:
			if seenBox {
				h += it.h
			}
		case itPenalty:
			if !seenBox {
				continue
			}
			if it.penalty <= penaltyForced {
				return i
			}
			if h > avail {
				return i
			}
			// Bricht die Seite im Rumpf einer Tabelle, steht der Fuß darunter.
			var reserve areatree.Length
			if last != nil && last.row != nil {
				if t := last.row.tbl; t != nil && last.row.sec == secBody && !t.omitFooter && t.continuesAfter(last.row, last.slice) {
					reserve = t.footerH
				}
			}
			if h+reserve > avail {
				continue
			}
			if best < 0 || it.penalty <= bestPen {
				best, bestPen = i, it.penalty
			}
		}
	}
	return len(items)
}
