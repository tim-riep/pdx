package linebreak

import (
	"math"

	"github.com/tim-riep/pdx/areatree"
)

// KnuthPlass bricht einen Absatz nach dem Verfahren von Knuth und Plass
// (1981) um: Unter allen zulässigen Umbruchfolgen wird die mit den
// geringsten Gesamtkosten (Demerits) gewählt.
//
// Die Parameter folgen Apache FOP 2.11, damit beide Engines vergleichbar
// umbrechen: Demerits (1 + 100·|r|³ + p)², 50 für aufeinanderfolgende
// Trennungen und für Zeilen inkompatibler Fitness-Klassen, Schwellen für das
// Anpassungsverhältnis r von 1, 5 und 20. Die letzte Zeile eines Absatzes
// und jede Zeile vor einem erzwungenen Umbruch darf um die volle Zeilenbreite
// gedehnt werden. Findet auch die letzte Schwelle keine Lösung, entstehen
// überlange Zeilen statt eines Fehlers. Greedy bleibt als Rückfall erhalten.
//
// Flattersatz bildet der Aufrufer wie FOP über die Item-Folge ab: an jedem
// Wortzwischenraum Glue(0, +R), Penalty(0), Glue(w, −R).
type KnuthPlass struct {
	// Tolerances sind die Schwellen für r. Leer bedeutet 1, 5, 20.
	Tolerances []float64
	// FitnessDemerit kostet ein Wechsel zwischen nicht benachbarten
	// Fitness-Klassen. 0 bedeutet 50.
	FitnessDemerit float64
	// FlaggedDemerit kostet die zweite geflaggte Penalty in Folge. 0
	// bedeutet 50.
	FlaggedDemerit float64
}

type kpNode struct {
	pos      int // Index des Umbruchs, -1 für den Absatzanfang
	line     int // Zahl der Zeilen bis hier
	fit      int
	w, y, z  float64 // Summen ab dem Zeilenanfang nach diesem Umbruch
	demerits float64
	ratio    float64
	prev     *kpNode
	flagged  bool
}

// Break wählt die Umbruchstellen.
func (k KnuthPlass) Break(items []Item, width LineWidth) ([]Break, error) {
	if len(items) == 0 {
		return nil, nil
	}
	tols := k.Tolerances
	if len(tols) == 0 {
		tols = []float64{1, 5, 20}
	}
	// Wie FOP zuerst ohne Trennstellen (geflaggte Penalties), dann mit.
	flagged := false
	for _, it := range items {
		if it.Kind == Penalty && it.Flagged {
			flagged = true
			break
		}
	}
	if flagged {
		if bs, ok := k.run(items, width, tols[0], false, false); ok {
			return bs, nil
		}
	}
	for i, tol := range tols {
		if bs, ok := k.run(items, width, tol, i == len(tols)-1, true); ok {
			return bs, nil
		}
	}
	return Greedy{}.Break(items, width)
}

func (k KnuthPlass) run(items []Item, width LineWidth, tol float64, force, allowFlagged bool) ([]Break, bool) {
	fitDem := k.FitnessDemerit
	if fitDem == 0 {
		fitDem = 50
	}
	flagDem := k.FlaggedDemerit
	if flagDem == 0 {
		flagDem = 50
	}
	n := len(items)
	// Präfixsummen über Boxen und Glue.
	W := make([]float64, n+1)
	Y := make([]float64, n+1)
	Z := make([]float64, n+1)
	for i, it := range items {
		W[i+1], Y[i+1], Z[i+1] = W[i], Y[i], Z[i]
		if it.Kind != Penalty {
			W[i+1] += float64(it.Width)
			Y[i+1] += float64(it.Stretch)
			Z[i+1] += float64(it.Shrink)
		}
	}
	// after liefert den Beginn der Zeile nach einem Umbruch bei pos: Glue
	// und Penalties danach entfallen bis zur ersten Box.
	after := func(pos int) int {
		j := pos + 1
		for j < n && items[j].Kind != Box {
			if items[j].Kind == Penalty && items[j].Penalty <= ForcedBreak && j > pos {
				break
			}
			j++
		}
		return j
	}
	start := &kpNode{pos: -1, fit: 1}
	s0 := after(-1)
	start.w, start.y, start.z = W[s0], Y[s0], Z[s0]
	active := []*kpNode{start}
	var final []*kpNode

	legal := func(i int) bool {
		if i == n {
			return true
		}
		it := items[i]
		switch it.Kind {
		case Penalty:
			return it.Penalty < Forbidden && (allowFlagged || !it.Flagged)
		case Glue:
			return i > 0 && items[i-1].Kind == Box
		}
		return false
	}

	for i := 0; i <= n; i++ {
		if !legal(i) {
			continue
		}
		forced := i == n || items[i].Kind == Penalty && items[i].Penalty <= ForcedBreak
		var pen float64
		var penW float64
		flagged := false
		if i < n && items[i].Kind == Penalty {
			pen = float64(items[i].Penalty)
			penW = float64(items[i].Width)
			flagged = items[i].Flagged
		}
		type cand struct {
			d     float64
			node  *kpNode
			ratio float64
		}
		var best [4]*cand
		var deact *kpNode // bester entfernter Knoten für den Notfall
		var deactD float64
		keep := active[:0]
		for _, a := range active {
			avail := float64(width(a.line))
			L := W[i] - a.w + penW
			st := Y[i] - a.y
			sh := Z[i] - a.z
			if forced {
				st += avail // Füllung der letzten Zeile wie bei FOP
			}
			var r float64
			switch {
			case L < avail:
				if st > 0 {
					r = (avail - L) / st
				} else {
					r = math.Inf(1)
				}
			case L > avail:
				if sh > 0 {
					r = (avail - L) / sh
				} else {
					r = math.Inf(-1)
				}
			}
			remove := r < -1 || forced
			if !remove {
				keep = append(keep, a)
			}
			if r < -1 {
				if deact == nil || a.demerits < deactD {
					deact, deactD = a, a.demerits
				}
				continue
			}
			if r > tol {
				continue
			}
			f := 1 + 100*math.Pow(math.Abs(r), 3)
			var d float64
			switch {
			case pen >= 0:
				d = (f + pen) * (f + pen)
			case forced:
				d = f * f
			default:
				d = f*f - pen*pen
			}
			if flagged && a.flagged {
				d += flagDem
			}
			fit := fitness(r)
			if a.pos >= 0 && abs(fit-a.fit) > 1 {
				d += fitDem
			}
			d += a.demerits
			if best[fit] == nil || d < best[fit].d {
				best[fit] = &cand{d: d, node: a, ratio: r}
			}
		}
		active = keep
		minD := math.Inf(1)
		for _, c := range best {
			if c != nil && c.d < minD {
				minD = c.d
			}
		}
		s := after(i)
		if i == n {
			s = n
		}
		added := false
		for fit, c := range best {
			if c == nil || c.d > minD+fitDem {
				continue
			}
			nd := &kpNode{pos: i, line: c.node.line + 1, fit: fit, demerits: c.d, ratio: c.ratio,
				prev: c.node, flagged: flagged, w: W[s], y: Y[s], z: Z[s]}
			added = true
			if i == n {
				final = append(final, nd)
			} else {
				active = append(active, nd)
			}
		}
		if !added && len(active) == 0 && i < n {
			if !force || deact == nil {
				return nil, false
			}
			// Notfall: überlange Zeile an dieser Stelle.
			nd := &kpNode{pos: i, line: deact.line + 1, fit: 0, demerits: deact.demerits + 1e9, ratio: -1,
				prev: deact, flagged: flagged, w: W[s], y: Y[s], z: Z[s]}
			active = append(active, nd)
		}
		if !added && i == n && force && len(final) == 0 && deact != nil {
			final = append(final, &kpNode{pos: n, line: deact.line + 1, demerits: deact.demerits + 1e9, ratio: -1, prev: deact})
		}
	}
	if len(final) == 0 {
		return nil, false
	}
	bestNode := final[0]
	for _, f := range final[1:] {
		if f.demerits < bestNode.demerits {
			bestNode = f
		}
	}
	var out []Break
	for nd := bestNode; nd != nil && nd.pos >= 0; nd = nd.prev {
		r := nd.ratio
		if math.IsInf(r, 0) {
			r = 0
		}
		out = append(out, Break{Index: nd.pos, Ratio: r})
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	// Die letzte Zeile und Zeilen vor erzwungenen Umbrüchen werden nicht
	// gedehnt, das Verhältnis gilt nur für die Kostenrechnung.
	for i := range out {
		idx := out[i].Index
		if (idx == n || items[idx].Kind == Penalty && items[idx].Penalty <= ForcedBreak) && out[i].Ratio > 0 {
			out[i].Ratio = 0
		}
	}
	return out, true
}

// fitness ordnet r den Klassen eng, normal, locker und sehr locker zu.
func fitness(r float64) int {
	switch {
	case r < -0.5:
		return 0
	case r <= 0.5:
		return 1
	case r <= 1:
		return 2
	}
	return 3
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// RaggedStretch ist die Dehnbarkeit, die eine Zeile im Flattersatz an ihrer
// Umbruchstelle erhält (3 × 3336 Millipunkt wie bei Apache FOP).
const RaggedStretch areatree.Length = 3 * 3336
