package fosuite

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/tim-riep/pdx/internal/fopcompare"
)

// TestFOConformance setzt alle aktiven Fälle mit pdx, vergleicht sie mit den
// Erwartungen aus FOP und schreibt den Bericht nach out/fo-conformance.md.
// Er schlägt fehl, wenn ein Fall abweicht, der nicht in deviating.txt steht,
// oder ein aktiver Fall keine Erwartung hat.
func TestFOConformance(t *testing.T) {
	if os.Getenv("PDX_FO_CONFORMANCE") != "1" {
		t.Skip("PDX_FO_CONFORMANCE=1 setzen oder make fo-conformance aufrufen")
	}
	root, err := fopcompare.RepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	disabled, err := ReadList(filepath.Join(DataDir(root), "disabled.txt"))
	if err != nil {
		t.Fatal(err)
	}
	deviating, err := ReadList(filepath.Join(DataDir(root), "deviating.txt"))
	if err != nil {
		t.Fatal(err)
	}
	cases, err := Cases(root)
	if err != nil {
		t.Fatal(err)
	}
	type row struct{ total, pass, known, open, off, fail int }
	rows := map[string]*row{}
	for _, c := range Categories {
		rows[c] = &row{}
	}
	suites := map[string]*row{SuiteFOP: {}, SuiteW3C: {}}
	var failures, fixed []string
	details := map[string][]Diff{}
	count := func(c Case, f func(r *row)) {
		f(rows[c.Category])
		f(suites[c.Suite])
	}
	seen := map[string]bool{}
	for _, c := range cases {
		seen[c.ID()] = true
		count(c, func(r *row) { r.total++ })
		if _, ok := disabled[c.ID()]; ok {
			count(c, func(r *row) { r.off++ })
			continue
		}
		e, err := ReadExpect(ExpectFile(root, c))
		if err != nil {
			count(c, func(r *row) { r.fail++ })
			failures = append(failures, fmt.Sprintf("%s: keine Erwartung (%v)", c.ID(), err))
			continue
		}
		diffs := Compare(c, Render(c, 30*time.Second), e, fopcompare.Default.Position)
		_, known := deviating[c.ID()]
		switch {
		case len(diffs) == 0:
			count(c, func(r *row) { r.pass++ })
			if known {
				fixed = append(fixed, c.ID())
			}
		case known && strings.HasPrefix(deviating[c.ID()], ReasonOpen):
			count(c, func(r *row) { r.open++ })
			details[c.ID()] = diffs
		case known:
			count(c, func(r *row) { r.known++ })
			details[c.ID()] = diffs
		default:
			count(c, func(r *row) { r.fail++ })
			details[c.ID()] = diffs
			failures = append(failures, fmt.Sprintf("%s: %s", c.ID(), diffs[0]))
		}
	}
	// Deaktivierte Fälle außerhalb des Teilumfangs liegen nicht als Datei vor.
	for id := range disabled {
		if seen[id] {
			continue
		}
		suite, name, _ := strings.Cut(id, "/")
		if suite == SuiteW3C && !dirExists(caseDir(root, SuiteW3C)) {
			continue
		}
		c := Case{Suite: suite, Name: name, Category: Category(suite, name)}
		count(c, func(r *row) { r.total++; r.off++ })
	}

	var b strings.Builder
	b.WriteString("# FO-Konformität\n\n")
	b.WriteString("Vergleich mit den Erwartungen aus Apache FOP 2.11: Seitenzahl, Zeilentexte, Grundlinien und Zeilenanfänge (Toleranz 1 pt).\n\n")
	b.WriteString("| Kategorie | Fälle | bestanden | begründet abweichend | offen | deaktiviert | neu abweichend |\n|---|---|---|---|---|---|---|\n")
	var sum row
	for _, cat := range Categories {
		r := rows[cat]
		fmt.Fprintf(&b, "| %s | %d | %d | %d | %d | %d | %d |\n", cat, r.total, r.pass, r.known, r.open, r.off, r.fail)
		sum.open += r.open
		sum.total += r.total
		sum.pass += r.pass
		sum.known += r.known
		sum.off += r.off
		sum.fail += r.fail
	}
	fmt.Fprintf(&b, "| **Summe** | %d | %d | %d | %d | %d | %d |\n\n", sum.total, sum.pass, sum.known, sum.open, sum.off, sum.fail)
	b.WriteString("| Suite | Fälle | bestanden | begründet abweichend | offen | deaktiviert | neu abweichend |\n|---|---|---|---|---|---|---|\n")
	for _, s := range []string{SuiteFOP, SuiteW3C} {
		r := suites[s]
		fmt.Fprintf(&b, "| %s | %d | %d | %d | %d | %d | %d |\n", s, r.total, r.pass, r.known, r.open, r.off, r.fail)
	}
	if !dirExists(caseDir(root, SuiteW3C)) {
		b.WriteString("\nDie W3C-Suite ist nicht heruntergeladen (make fo-conformance lädt sie).\n")
	}
	if len(fixed) > 0 {
		sort.Strings(fixed)
		b.WriteString("\nIn deviating.txt, aber jetzt gleich mit FOP: " + strings.Join(fixed, ", ") + "\n")
	}
	if len(details) > 0 {
		b.WriteString("\n## Abweichungen\n\n")
		for _, id := range sortedDiffKeys(details) {
			why := deviating[id]
			if why == "" {
				why = "NEU"
			}
			fmt.Fprintf(&b, "* %s (%s)\n", id, why)
			for i, d := range details[id] {
				if i == 3 {
					fmt.Fprintf(&b, "  * und %d weitere\n", len(details[id])-3)
					break
				}
				fmt.Fprintf(&b, "  * %s\n", d)
			}
		}
	}
	// Alle Abweichungen je Fall für die Analyse.
	var det strings.Builder
	for _, id := range sortedDiffKeys(details) {
		for _, d := range details[id] {
			fmt.Fprintf(&det, "%s\t%s\n", id, stripPaths(string(d), root))
		}
	}
	os.MkdirAll(filepath.Join(root, "out"), 0o755)
	os.WriteFile(filepath.Join(root, "out", "fo-details.txt"), []byte(det.String()), 0o644)
	report := b.String()
	os.MkdirAll(filepath.Join(root, "out"), 0o755)
	os.WriteFile(filepath.Join(root, "out", "fo-conformance.md"), []byte(report), 0o644)
	t.Log("\n" + report[:strings.Index(report+"\n## ", "\n## ")])
	if os.Getenv("PDX_SUITE_RECORD") == "1" {
		// Neue Abweichungen als offen vermerken, behobene austragen.
		// Offene Einträge zeigen die aktuelle erste Abweichung, begründete
		// bleiben unverändert.
		for id, diffs := range details {
			if why, ok := deviating[id]; !ok || strings.HasPrefix(why, ReasonOpen) {
				deviating[id] = ReasonOpen + oneLine(stripPaths(string(diffs[0]), root))
			}
		}
		for _, id := range fixed {
			delete(deviating, id)
		}
		for id := range deviating {
			if !seen[id] {
				delete(deviating, id)
			} else if _, off := disabled[id]; off {
				delete(deviating, id)
			}
		}
		if err := WriteList(filepath.Join(DataDir(root), "deviating.txt"), deviatingHeader, deviating); err != nil {
			t.Fatal(err)
		}
		return
	}
	for _, f := range failures {
		t.Error(f)
	}
}

// ReasonOpen kennzeichnet Abweichungen, die noch nicht untersucht sind. Sie
// sind Arbeit für die Engine, keine begründeten Abweichungen.
const ReasonOpen = "offen, erste Abweichung: "

const deviatingHeader = `Bekannte Abweichungen von FOP, eine Zeile je Fall. Ein Fall hier zählt im
Bericht als "abweichend (bekannt)" und lässt make fo-conformance nicht
scheitern. Begründete Abweichungen verweisen auf DEVIATIONS.md. Zeilen mit
"offen" sind noch nicht untersucht und Arbeit für die Engine.
PDX_SUITE_RECORD=1 trägt neue Abweichungen als offen ein und behobene aus.`

// stripPaths entfernt den Pfad des Repositorys aus Meldungen.
func stripPaths(s, root string) string {
	return strings.ReplaceAll(s, root+string(filepath.Separator), "")
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func sortedDiffKeys(m map[string][]Diff) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
