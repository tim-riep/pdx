package fosuite

import (
	"fmt"
	"math"
	"path/filepath"
	"runtime/debug"
	"sort"
	"time"

	"github.com/tim-riep/pdx"
	"github.com/tim-riep/pdx/internal/fopcompare"
)

// Outcome ist das Ergebnis von pdx für einen Fall.
type Outcome struct {
	Pages int
	Lines []fopcompare.ATLine
	Err   error
}

// Render setzt einen Fall mit pdx. Abstürze und Zeitüberschreitungen werden
// zu Fehlern.
func Render(c Case, timeout time.Duration) Outcome {
	done := make(chan Outcome, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- Outcome{Err: fmt.Errorf("Absturz: %v\n%s", r, debug.Stack())}
			}
		}()
		res, err := pdx.RenderFile(c.File, pdx.Options{
			Profile:      "pdf-1.7",
			CreationDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			BaseDir:      filepath.Dir(c.File),
		})
		if err != nil {
			done <- Outcome{Err: err}
			return
		}
		done <- Outcome{Pages: len(res.Tree.Pages), Lines: fopcompare.PDXLines(res.Tree)}
	}()
	select {
	case o := <-done:
		return o
	case <-time.After(timeout):
		return Outcome{Err: fmt.Errorf("Zeitüberschreitung nach %s", timeout)}
	}
}

// Diff ist eine Abweichung von der Erwartung.
type Diff string

// Compare vergleicht das Ergebnis von pdx mit der Erwartung. Zeilen werden je
// Seite über Text und Vorkommen zugeordnet, die Lage muss innerhalb von tol
// Punkt liegen.
func Compare(c Case, o Outcome, e *Expect, tol float64) []Diff {
	if o.Err != nil {
		return []Diff{Diff("Fehler in pdx: " + o.Err.Error())}
	}
	var out []Diff
	if o.Pages != e.Pages {
		out = append(out, Diff(fmt.Sprintf("Seiten: pdx %d, FOP %d", o.Pages, e.Pages)))
	}
	type key struct {
		page int
		text string
	}
	rest := map[key][]Line{}
	var order []key
	for _, l := range e.Lines {
		k := key{l.Page, l.Text}
		if _, ok := rest[k]; !ok {
			order = append(order, k)
		}
		rest[k] = append(rest[k], l)
	}
	show := func(text, plain string) string {
		if c.Suite == SuiteW3C {
			return plain
		}
		return text
	}
	// Gleiche Texte auf einer Seite werden nach Lage zugeordnet, oben vor
	// unten und links vor rechts. Die Reihenfolge der Zeilen im Area Tree ist
	// Sache der Implementierung (etwa bei Tabellenzellen).
	byPos := func(a, b float64, ax, bx float64) bool {
		if math.Abs(a-b) > 0.5 {
			return a < b
		}
		return ax < bx
	}
	for k, q := range rest {
		sort.SliceStable(q, func(i, j int) bool { return byPos(q[i].Y, q[j].Y, q[i].X, q[j].X) })
		rest[k] = q
	}
	lines := append([]fopcompare.ATLine(nil), o.Lines...)
	sort.SliceStable(lines, func(i, j int) bool {
		if lines[i].Page != lines[j].Page {
			return lines[i].Page < lines[j].Page
		}
		return byPos(lines[i].Baseline, lines[j].Baseline, lines[i].X, lines[j].X)
	})
	for _, l := range lines {
		plain := fopcompare.Norm(l.Text)
		if plain == "" {
			continue
		}
		k := key{l.Page, textFor(c.Suite, l.Text)}
		q := rest[k]
		if len(q) == 0 {
			out = append(out, Diff(fmt.Sprintf("Seite %d: Zeile nur in pdx: %q", l.Page, plain)))
			continue
		}
		want := q[0]
		rest[k] = q[1:]
		dy := l.Baseline - want.Y
		dx := 0.0
		if l.HasX && want.HasX {
			dx = l.X - want.X
		}
		if math.Abs(dx) > tol || math.Abs(dy) > tol {
			out = append(out, Diff(fmt.Sprintf("Seite %d: %q verschoben um %.2f pt waagerecht, %.2f pt senkrecht",
				l.Page, show(want.Text, plain), dx, dy)))
		}
	}
	for _, k := range order {
		for _, l := range rest[k] {
			out = append(out, Diff(fmt.Sprintf("Seite %d: Zeile nur in FOP: %q", l.Page, l.Text)))
		}
	}
	return out
}
