package fosuite

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tim-riep/pdx/fo/props"
	"github.com/tim-riep/pdx/internal/fopcompare"
)

// TestShow stellt für einen Fall die Zeilen von pdx und die Erwartung
// gegenüber: PDX_SUITE_SHOW=fop/table_basic.
func TestShow(t *testing.T) {
	id := os.Getenv("PDX_SUITE_SHOW")
	if id == "" {
		t.Skip("PDX_SUITE_SHOW=suite/name setzen")
	}
	root, _ := fopcompare.RepoRoot()
	cases, _ := Cases(root)
	for _, c := range cases {
		if c.ID() != id {
			continue
		}
		if os.Getenv("PDX_SUITE_PX") == "1" {
			props.SetPixelSize(1)
			defer props.SetPixelSize(0.75)
		}
		o := Render(c, 30*time.Second)
		if o.Err != nil {
			t.Fatal(o.Err)
		}
		e, err := ReadExpect(ExpectFile(root, c))
		if err != nil {
			t.Fatal(err)
		}
		var b strings.Builder
		fmt.Fprintf(&b, "Seiten pdx %d, FOP %d\n%-6s %8s %8s | %8s %8s  %s\n", o.Pages, e.Pages, "Seite", "x pdx", "y pdx", "x FOP", "y FOP", "Text")
		used := make([]bool, len(e.Lines))
		for _, l := range o.Lines {
			text := fopcompare.Norm(l.Text)
			if text == "" {
				continue
			}
			key := textFor(c.Suite, l.Text)
			fx, fy := "-", "-"
			for i, w := range e.Lines {
				if !used[i] && w.Page == l.Page && w.Text == key {
					used[i] = true
					fy = fmt.Sprintf("%.2f", w.Y)
					if w.HasX {
						fx = fmt.Sprintf("%.2f", w.X)
					}
					break
				}
			}
			fmt.Fprintf(&b, "%-6d %8.2f %8.2f | %8s %8s  %s\n", l.Page, l.X, l.Baseline, fx, fy, text)
		}
		for i, w := range e.Lines {
			if !used[i] {
				fmt.Fprintf(&b, "%-6d %8s %8s | %8.2f %8.2f  %s (nur FOP)\n", w.Page, "-", "-", w.X, w.Y, w.Text)
			}
		}
		t.Log("\n" + b.String())
		return
	}
	t.Fatalf("Fall %s nicht gefunden", id)
}
