package layout

import "github.com/tim-riep/pdx/areatree"

// breakPage chooses the end of the page that starts at start. It returns the
// index of the chosen penalty or len(items) if the rest fits.
//
// Among all penalties up to which the content fits in avail, the one with
// the lowest cost wins, on a tie the latest. Keeps with integer
// strength are thus relaxed in ascending strength (XSL 1.1
// 4.8). If there are only forbidden points, the content overflows the
// page, a keep with always is never broken. If even the first box does
// not fit, the page breaks at the first allowed point after it.
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
				// If the page starts in the body of a table, the
				// repeated header stands above it.
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
			// If the content does not fit, the break is made at the next allowed
			// point. A keep with always is preserved, the content then
			// overflows the page (XSL 1.1 4.8: keeps must be satisfied
			// as long as no break or stronger keep conflicts with them).
			if h > avail && it.penalty < penaltyInf {
				return i
			}
			if h > avail {
				continue
			}
			// If the page breaks in the body of a table, the footer stands below it.
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
				continue // forbidden point
			}
			if best < 0 || it.penalty <= bestPen {
				best, bestPen = i, it.penalty
			}
		}
	}
	return len(items)
}
