package fosuite

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/tim-riep/pdx/internal/fopcompare"
)

// TestAnalyze teilt die abweichenden Fälle anhand der lokalen FOP-Ausgaben
// (out/suites/ref) in Klassen ein und schreibt out/fo-analyse.txt. Nur zur
// Analyse: PDX_SUITE_ANALYZE=1.
func TestAnalyze(t *testing.T) {
	if os.Getenv("PDX_SUITE_ANALYZE") != "1" {
		t.Skip("PDX_SUITE_ANALYZE=1 setzen")
	}
	root, _ := fopcompare.RepoRoot()
	deviating, _ := ReadList(filepath.Join(DataDir(root), "deviating.txt"))
	cases, _ := Cases(root)
	classes := map[string][]string{}
	for _, c := range cases {
		why, ok := deviating[c.ID()]
		if !ok || !strings.HasPrefix(why, ReasonOpen) {
			continue
		}
		at, err := os.ReadFile(filepath.Join(root, "out", "suites", "ref", c.Suite, c.Name+".at.xml"))
		if err != nil {
			classes["ohne FOP-Ausgabe"] = append(classes["ohne FOP-Ausgabe"], c.ID())
			continue
		}
		_, fl, fpages, _ := fopcompare.ParseAreaTreeLines(at)
		o := Render(c, 30*time.Second)
		if o.Err != nil {
			classes["Fehler"] = append(classes["Fehler"], c.ID()+" "+oneLine(o.Err.Error()))
			continue
		}
		class := classify(o, fl, fpages)
		classes[class] = append(classes[class], c.ID())
	}
	var b strings.Builder
	keys := make([]string, 0, len(classes))
	for k := range classes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, "## %s (%d)\n", k, len(classes[k]))
		for _, id := range classes[k] {
			fmt.Fprintf(&b, "%s\n", id)
		}
	}
	os.WriteFile(filepath.Join(root, "out", "fo-analyse.txt"), []byte(b.String()), 0o644)
	for _, k := range keys {
		t.Logf("%s: %d", k, len(classes[k]))
	}
}

// stream verbindet die Zeilentexte ohne Trennstriche und Leerraum.
func stream(ls []fopcompare.ATLine) string {
	var b strings.Builder
	for _, l := range ls {
		s := fopcompare.Norm(l.Text)
		s = strings.TrimSuffix(s, "‐")
		s = strings.TrimSuffix(s, "-")
		b.WriteString(strings.ReplaceAll(s, " ", ""))
	}
	return b.String()
}

func classify(o Outcome, fl []fopcompare.ATLine, fpages int) string {
	var pl []fopcompare.ATLine
	for _, l := range o.Lines {
		if fopcompare.Norm(l.Text) != "" {
			pl = append(pl, l)
		}
	}
	var fl2 []fopcompare.ATLine
	for _, l := range fl {
		if fopcompare.Norm(l.Text) != "" {
			fl2 = append(fl2, l)
		}
	}
	sameText := stream(pl) == stream(fl2)
	sameLines := len(pl) == len(fl2)
	if sameLines {
		for i := range pl {
			if fopcompare.Norm(pl[i].Text) != fopcompare.Norm(fl2[i].Text) || pl[i].Page != fl2[i].Page {
				sameLines = false
				break
			}
		}
	}
	switch {
	case sameLines:
		// Gleiche Zeilen: nur die Lage weicht ab.
		dy := map[string]int{}
		for i := range pl {
			d := math.Round((pl[i].Baseline-fl2[i].Baseline)*100) / 100
			if math.Abs(d) > 1 {
				dy[fmt.Sprintf("%.2f", d)]++
			}
		}
		if len(dy) == 0 {
			return "Lage waagerecht"
		}
		return "Lage senkrecht"
	case sameText && o.Pages == fpages:
		return "Zeilenumbruch anders, gleiche Seitenzahl"
	case sameText:
		return "Zeilenumbruch anders, andere Seitenzahl"
	case o.Pages != fpages:
		return "Text und Seitenzahl anders"
	}
	return "Text anders"
}
