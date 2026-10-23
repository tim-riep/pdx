package fosuite

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tim-riep/pdx"
	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/fo/props"
	"github.com/tim-riep/pdx/internal/fopcompare"
)

// TestAnalyze sorts the deviating cases into classes based on the local FOP
// outputs (out/suites/ref) and writes out/fo-analyse.txt. For analysis only:
// PDX_SUITE_ANALYZE=1.
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
		if strings.HasPrefix(class, "Zeilenumbruch") {
			if over := overflow(c); over != "" {
				class += ", Überlauf"
				classes["Überlauf: "+c.ID()] = append(classes["Überlauf: "+c.ID()], over)
			} else if sameWidths(c, at) {
				class += ", gleiche Zeilenbreiten"
			} else {
				class += ", andere Zeilenbreiten"
			}
		}
		// Deviations only due to the pixel size: with 1 px = 1 pt as in FOP
		// identical to the expectation.
		if src, err := os.ReadFile(c.File); err == nil && strings.Contains(string(src), "px") {
			if e, err := ReadExpect(ExpectFile(root, c)); err == nil {
				props.SetPixelSize(1)
				d := Compare(c, Render(c, 30*time.Second), e, fopcompare.Default.Position)
				props.SetPixelSize(0.75)
				if len(d) == 0 {
					class = "nur px"
				}
			}
		}
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

// stream joins the line texts without hyphens and whitespace.
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
	// Same text per page: only the line breaks differ.
	pageStream := func(ls []fopcompare.ATLine) map[int]string {
		m := map[int][]fopcompare.ATLine{}
		for _, l := range ls {
			m[l.Page] = append(m[l.Page], l)
		}
		out := map[int]string{}
		for p, v := range m {
			out[p] = stream(v)
		}
		return out
	}
	ps, fs := pageStream(pl), pageStream(fl2)
	sameText := len(ps) == len(fs)
	for p, v := range ps {
		if fs[p] != v {
			sameText = false
		}
	}
	if !sameText && stream(pl) == stream(fl2) {
		return "Seitenverteilung anders"
	}
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
		// Same lines: only the position differs.
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

// overflow reports the first line of pdx whose glyphs extend beyond the
// line.
func overflow(c Case) string {
	res, err := pdx.RenderFile(c.File, pdx.Options{Profile: "pdf-1.7", CreationDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		return ""
	}
	for _, p := range res.Tree.Pages {
		var found string
		areatree.Walk(p.Areas, func(a areatree.Area) bool {
			l, ok := a.(*areatree.Container)
			if !ok || l.Kind != areatree.KindLine || found != "" {
				return true
			}
			areatree.Walk(l.Children, func(x areatree.Area) bool {
				if g, ok := x.(*areatree.GlyphRun); ok {
					end := g.Origin.X
					for _, gl := range g.Glyphs {
						end += gl.Advance
					}
					if end > l.X+l.W+500 {
						found = fmt.Sprintf("Zeile endet bei %.1f pt, Breite %.1f pt", end.Pt(), (l.X + l.W).Pt())
					}
				}
				return true
			})
			return false
		})
		if found != "" {
			return found
		}
	}
	return ""
}

var lineIPD = regexp.MustCompile(`<lineArea ipd="(\d+)"`)

// sameWidths compares the available line widths of pdx (width of the line
// containers) and FOP (ipd of the lineArea) as sets, accurate to 0.1 pt.
func sameWidths(c Case, at []byte) bool {
	fw := map[int]bool{}
	for _, m := range lineIPD.FindAllSubmatch(at, -1) {
		v, _ := strconv.Atoi(string(m[1]))
		fw[(v+50)/100] = true
	}
	res, err := pdx.RenderFile(c.File, pdx.Options{Profile: "pdf-1.7", CreationDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		return false
	}
	pw := map[int]bool{}
	for _, p := range res.Tree.Pages {
		areatree.Walk(p.Areas, func(a areatree.Area) bool {
			if l, ok := a.(*areatree.Container); ok && l.Kind == areatree.KindLine {
				pw[(int(l.W)+50)/100] = true
				return false
			}
			return true
		})
	}
	if len(fw) != len(pw) {
		return false
	}
	for k := range fw {
		if !pw[k] {
			return false
		}
	}
	return true
}
