package layout

import "github.com/tim-riep/pdx/areatree"

// breakPage wählt das Ende der Seite, die bei start beginnt. Es liefert den
// Index der gewählten Penalty oder len(items), wenn der Rest passt.
//
// Unter allen Penalties, bis zu denen der Inhalt in avail passt, gewinnt die
// mit den geringsten Kosten, bei Gleichstand die späteste. Keeps mit
// ganzzahliger Stärke werden so in aufsteigender Stärke gelockert (XSL 1.1
// 4.8). Gibt es nur verbotene Stellen, läuft der Inhalt über die Seite
// hinaus, ein Keep mit always wird nie gebrochen. Passt schon die erste Box
// nicht, bricht die Seite an der ersten erlaubten Stelle danach.
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
			} else {
				h += it.keep
			}
		case itPenalty:
			if !seenBox {
				continue
			}
			if it.penalty <= penaltyForced {
				return i
			}
			// Passt der Inhalt nicht, wird an der nächsten erlaubten Stelle
			// umbrochen. Ein Keep mit always bleibt erhalten, der Inhalt läuft
			// dann über die Seite hinaus (XSL 1.1 4.8: Keeps müssen erfüllt
			// werden, solange kein Umbruch oder stärkerer Keep dagegen steht).
			if h > avail && it.penalty < penaltyInf {
				return i
			}
			if h > avail {
				continue
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
			if it.penalty >= penaltyInf {
				continue // verbotene Stelle
			}
			if best < 0 || it.penalty <= bestPen {
				best, bestPen = i, it.penalty
			}
		}
	}
	return len(items)
}
