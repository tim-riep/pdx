package fopcompare

import (
	"context"
	"fmt"
	"image"
	"math"
	"strings"

	"github.com/tim-riep/pdx/areatree"
)

// LineDiff ist eine abweichende Zeile.
type LineDiff struct {
	Page     int
	PDX, FOP string // Text, leer wenn die Zeile auf einer Seite fehlt
	DX, DY   float64
}

// BlockDiff ist ein Block mit abweichender Lage.
type BlockDiff struct {
	Page               int
	FirstLine          string
	DTop, DBottom      float64
	LinesPDX, LinesFOP int
}

// PageDiff ist das Ergebnis des Pixelvergleichs einer Seite.
type PageDiff struct {
	Page  int
	Ratio float64
}

// Report ist das Ergebnis eines Vergleichs.
type Report struct {
	Name               string
	Tol                Tolerance
	PagesPDX, PagesFOP int
	LinesCompared      int
	Lines              []LineDiff
	BlocksCompared     int
	Blocks             []BlockDiff
	Pixels             []PageDiff
}

// OK meldet, ob alle Vergleiche innerhalb der Toleranz liegen.
func (r *Report) OK() bool {
	if r.PagesPDX != r.PagesFOP || len(r.Lines) > 0 || len(r.Blocks) > 0 {
		return false
	}
	for _, p := range r.Pixels {
		if p.Ratio > r.Tol.MaxPixelRatio {
			return false
		}
	}
	return true
}

// String fasst den Bericht zusammen.
func (r *Report) String() string {
	var b strings.Builder
	state := "innerhalb der Toleranz"
	if !r.OK() {
		state = "ABWEICHEND"
	}
	fmt.Fprintf(&b, "%s: %s (Seiten pdx %d, FOP %d, Zeilen %d verglichen, %d abweichend, Blöcke %d verglichen, %d abweichend)\n",
		r.Name, state, r.PagesPDX, r.PagesFOP, r.LinesCompared, len(r.Lines), r.BlocksCompared, len(r.Blocks))
	for i, l := range r.Lines {
		if i == 10 {
			fmt.Fprintf(&b, "  und %d weitere Zeilen\n", len(r.Lines)-10)
			break
		}
		switch {
		case l.PDX == "":
			fmt.Fprintf(&b, "  Seite %d: Zeile nur in FOP: %q\n", l.Page, l.FOP)
		case l.FOP == "":
			fmt.Fprintf(&b, "  Seite %d: Zeile nur in pdx: %q\n", l.Page, l.PDX)
		default:
			fmt.Fprintf(&b, "  Seite %d: %q verschoben um %.2f pt waagerecht, %.2f pt senkrecht\n", l.Page, l.PDX, l.DX, l.DY)
		}
	}
	for i, bl := range r.Blocks {
		if i == 10 {
			fmt.Fprintf(&b, "  und %d weitere Blöcke\n", len(r.Blocks)-10)
			break
		}
		fmt.Fprintf(&b, "  Seite %d: Block %q oben %.2f pt, unten %.2f pt, Zeilen pdx %d FOP %d\n",
			bl.Page, short(bl.FirstLine), bl.DTop, bl.DBottom, bl.LinesPDX, bl.LinesFOP)
	}
	for _, p := range r.Pixels {
		mark := ""
		if p.Ratio > r.Tol.MaxPixelRatio {
			mark = " (über der Grenze)"
		}
		fmt.Fprintf(&b, "  Seite %d: %.3f %% abweichende Pixel%s\n", p.Page, p.Ratio*100, mark)
	}
	return b.String()
}

func short(s string) string {
	r := []rune(s)
	if len(r) > 40 {
		return string(r[:40]) + "…"
	}
	return s
}

// Compare vergleicht die Ausgabe von pdx mit der von FOP.
func Compare(ctx context.Context, name string, pdxPDF []byte, pdxTree *areatree.Document, fop *FOPResult, tol Tolerance) (*Report, error) {
	r := &Report{Name: name, Tol: tol}
	pl, pn, err := Lines(ctx, pdxPDF)
	if err != nil {
		return nil, err
	}
	fl, fn, err := Lines(ctx, fop.PDF)
	if err != nil {
		return nil, err
	}
	r.PagesPDX, r.PagesFOP = pn, fn
	fb, fal, _, err := ParseAreaTreeLines(fop.AreaTree)
	if err != nil {
		return nil, fmt.Errorf("FOP-Area-Tree: %w", err)
	}
	// Text und senkrechte Lage (Grundlinie) kommen aus den Area Trees.
	// Poppler leitet Zeilenoberkanten aus dem /Ascent des FontDescriptor ab,
	// den beide Engines verschieden schreiben, und ordnet Zeilen nach eigener
	// Lesereihenfolge. Waagerecht ist Poppler verlässlich und liefert X.
	compareLines(r, withX(PDXLines(pdxTree), pl), withX(fal, fl), tol)
	compareBlocks(r, PDXBlocks(pdxTree), fb, tol)

	pp, err := Rasterize(ctx, pdxPDF, tol.DPI)
	if err != nil {
		return nil, err
	}
	fp, err := Rasterize(ctx, fop.PDF, tol.DPI)
	if err != nil {
		return nil, err
	}
	for i := 0; i < min(len(pp), len(fp)); i++ {
		r.Pixels = append(r.Pixels, PageDiff{Page: i + 1, Ratio: PixelDiff(pp[i], fp[i], tol)})
	}
	return r, nil
}

// cmpLine ist eine Zeile für den Vergleich. HasX ist gesetzt, wenn Poppler
// eine textgleiche Zeile gefunden hat.
type cmpLine struct {
	Page int
	Text string
	X, Y float64
	HasX bool
}

func norm(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "\u00a0", " ")), " ")
}

// withX ergänzt Zeilen aus dem Area Tree um X aus Poppler, zugeordnet über
// Seite, Text und Vorkommen.
func withX(at []ATLine, pl []Line) []cmpLine {
	type key struct {
		page int
		text string
	}
	xs := map[key][]float64{}
	for _, l := range pl {
		k := key{l.Page, norm(l.Text)}
		xs[k] = append(xs[k], l.X)
	}
	var out []cmpLine
	for _, l := range at {
		// Zeilen ohne Text (etwa nur ein Bild) werden über ihre Blöcke und
		// den Pixelvergleich geprüft.
		if norm(l.Text) == "" {
			continue
		}
		k := key{l.Page, norm(l.Text)}
		c := cmpLine{Page: l.Page, Text: norm(l.Text), Y: l.Baseline}
		if q := xs[k]; len(q) > 0 {
			c.X, c.HasX = q[0], true
			xs[k] = q[1:]
		}
		out = append(out, c)
	}
	return out
}

// PDXLines liest die Zeilen mit Grundlinie aus dem Area Tree von pdx.
func PDXLines(d *areatree.Document) []ATLine {
	var out []ATLine
	first := 0
	if len(d.Pages) > 0 {
		first = d.Pages[0].Number
	}
	for _, p := range d.Pages {
		areatree.Walk(p.Areas, func(a areatree.Area) bool {
			c, ok := a.(*areatree.Container)
			if !ok || c.Kind != areatree.KindLine {
				return true
			}
			base := -1.0
			areatree.Walk(c.Children, func(x areatree.Area) bool {
				if g, ok := x.(*areatree.GlyphRun); ok && base < 0 {
					base = g.Origin.Y.Pt()
				}
				return true
			})
			if base >= 0 {
				out = append(out, ATLine{Page: p.Number - first + 1, Text: lineText(c), Baseline: base})
			}
			return false
		})
	}
	return out
}

// compareLines ordnet die Zeilen je Seite über Text und Vorkommen zu und
// vergleicht die Lage der Paare.
func compareLines(r *Report, pdx, fop []cmpLine, tol Tolerance) {
	type key struct {
		page int
		text string
	}
	rest := map[key][]cmpLine{}
	var order []key
	for _, l := range fop {
		k := key{l.Page, l.Text}
		if _, ok := rest[k]; !ok {
			order = append(order, k)
		}
		rest[k] = append(rest[k], l)
	}
	for _, a := range pdx {
		k := key{a.Page, a.Text}
		q := rest[k]
		if len(q) == 0 {
			r.Lines = append(r.Lines, LineDiff{Page: a.Page, PDX: a.Text})
			continue
		}
		b := q[0]
		rest[k] = q[1:]
		r.LinesCompared++
		dy := a.Y - b.Y
		dx := 0.0
		if a.HasX && b.HasX {
			dx = a.X - b.X
		}
		if math.Abs(dx) > tol.Position || math.Abs(dy) > tol.Position {
			r.Lines = append(r.Lines, LineDiff{Page: a.Page, PDX: a.Text, FOP: b.Text, DX: dx, DY: dy})
		}
	}
	for _, k := range order {
		for _, b := range rest[k] {
			r.Lines = append(r.Lines, LineDiff{Page: b.Page, FOP: b.Text})
		}
	}
}

func onPage(ls []Line, p int) []Line {
	var out []Line
	for _, l := range ls {
		if l.Page == p {
			out = append(out, l)
		}
	}
	return out
}

// lcs liefert die Indexpaare einer längsten gemeinsamen Teilfolge.
func lcs(n, m int, eq func(i, j int) bool) [][2]int {
	t := make([][]int, n+1)
	for i := range t {
		t[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if eq(i, j) {
				t[i][j] = t[i+1][j+1] + 1
			} else {
				t[i][j] = max(t[i+1][j], t[i][j+1])
			}
		}
	}
	var out [][2]int
	for i, j := 0, 0; i < n && j < m; {
		switch {
		case eq(i, j):
			out = append(out, [2]int{i, j})
			i++
			j++
		case t[i+1][j] >= t[i][j+1]:
			i++
		default:
			j++
		}
	}
	return out
}

// PDXBlocks liest die Blöcke mit eigenen Zeilen aus dem Area Tree von pdx.
func PDXBlocks(d *areatree.Document) []Block {
	var out []Block
	for _, p := range d.Pages {
		var visit func(areas []areatree.Area)
		visit = func(areas []areatree.Area) {
			for _, a := range areas {
				c, ok := a.(*areatree.Container)
				if !ok {
					continue
				}
				if c.Kind == areatree.KindBlock {
					b := Block{Page: p.Number, Top: -1}
					for _, ch := range c.Children {
						if l, ok := ch.(*areatree.Container); ok && l.Kind == areatree.KindLine {
							if b.Lines == 0 {
								b.FirstLine, b.Top = lineText(l), l.Y.Pt()
							}
							b.Lines++
							b.Bottom = (l.Y + l.H).Pt()
						}
					}
					if b.Lines > 0 {
						out = append(out, b)
					}
				}
				visit(c.Children)
			}
		}
		visit(p.Areas)
	}
	// Seitenzahlen relativ zur ersten Seite, wie bei FOP ab 1 gezählt.
	if len(out) > 0 && len(d.Pages) > 0 {
		first := d.Pages[0].Number
		for i := range out {
			out[i].Page = out[i].Page - first + 1
		}
	}
	return out
}

func lineText(l *areatree.Container) string {
	var b strings.Builder
	areatree.Walk(l.Children, func(a areatree.Area) bool {
		if g, ok := a.(*areatree.GlyphRun); ok {
			for _, gl := range g.Glyphs {
				b.WriteString(gl.Text)
			}
		}
		return true
	})
	return strings.Join(strings.Fields(b.String()), " ")
}

func compareBlocks(r *Report, pdx, fop []Block, tol Tolerance) {
	pairs := lcs(len(pdx), len(fop), func(i, j int) bool {
		return pdx[i].Page == fop[j].Page && pdx[i].FirstLine == fop[j].FirstLine
	})
	for _, pr := range pairs {
		a, b := pdx[pr[0]], fop[pr[1]]
		r.BlocksCompared++
		dt, db := a.Top-b.Top, a.Bottom-b.Bottom
		if math.Abs(dt) > tol.Position || math.Abs(db) > tol.Position || a.Lines != b.Lines {
			r.Blocks = append(r.Blocks, BlockDiff{Page: a.Page, FirstLine: a.FirstLine, DTop: dt, DBottom: db, LinesPDX: a.Lines, LinesFOP: b.Lines})
		}
	}
}

// PixelDiff liefert den Anteil der Pixel, die in keinem Nachbarpixel im
// Umkreis Shift einen Partner mit Grauwertabstand bis GrayDelta haben. Die
// Prüfung läuft in beide Richtungen.
func PixelDiff(a, b *image.Gray, tol Tolerance) float64 {
	w := min(a.Bounds().Dx(), b.Bounds().Dx())
	h := min(a.Bounds().Dy(), b.Bounds().Dy())
	if w == 0 || h == 0 {
		return 1
	}
	bad := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if !near(a, b, x, y, tol) || !near(b, a, x, y, tol) {
				bad++
			}
		}
	}
	return float64(bad) / float64(w*h)
}

func near(a, b *image.Gray, x, y int, tol Tolerance) bool {
	va := int(a.GrayAt(a.Bounds().Min.X+x, a.Bounds().Min.Y+y).Y)
	for dy := -tol.Shift; dy <= tol.Shift; dy++ {
		for dx := -tol.Shift; dx <= tol.Shift; dx++ {
			px, py := b.Bounds().Min.X+x+dx, b.Bounds().Min.Y+y+dy
			if !(image.Point{px, py}.In(b.Bounds())) {
				continue
			}
			vb := int(b.GrayAt(px, py).Y)
			if abs(va-vb) <= tol.GrayDelta {
				return true
			}
		}
	}
	return false
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
