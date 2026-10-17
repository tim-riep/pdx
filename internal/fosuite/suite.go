// Package fosuite führt die FO-Testsuiten aus: ausgewählte Testfälle von
// Apache FOP 2.11 (test/layoutengine/standard-testcases) und die W3C XSL Test
// Suite. Erwartet wird, was FOP setzt: Seitenzahl, Zeilentexte, Grundlinien
// und Zeilenanfänge. Die Erwartungen werden einmal mit FOP erzeugt und liegen
// als kleine Textdateien im Repository, der Lauf selbst braucht kein Docker.
//
// Fälle außerhalb des Teilumfangs stehen mit Begründung in
// testdata/suites/disabled.txt, bekannte Abweichungen mit Begründung in
// testdata/suites/deviating.txt.
package fosuite

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Suiten.
const (
	SuiteFOP = "fop"
	SuiteW3C = "w3c"
)

// Case ist ein Testfall.
type Case struct {
	Suite, Name string
	// File ist die FO-Datei.
	File     string
	Category string
}

// ID ist "suite/name", so stehen Fälle in disabled.txt und deviating.txt.
func (c Case) ID() string { return c.Suite + "/" + c.Name }

// DataDir ist testdata/suites.
func DataDir(root string) string { return filepath.Join(root, "testdata", "suites") }

// W3CDir ist das Verzeichnis der heruntergeladenen W3C-Suite. Ihre Dateien
// stehen unter keiner genannten Lizenz und liegen daher nicht im Repository.
func W3CDir(root string) string { return filepath.Join(root, "out", "suites", "w3c") }

// caseDir liefert das Verzeichnis mit den FO-Dateien einer Suite.
func caseDir(root, suite string) string {
	if suite == SuiteW3C {
		return filepath.Join(W3CDir(root), "fo")
	}
	return filepath.Join(DataDir(root), suite, "cases")
}

// ExpectFile liefert den Pfad der Erwartung eines Falls.
func ExpectFile(root string, c Case) string {
	return filepath.Join(DataDir(root), c.Suite, "expect", c.Name+".txt")
}

// Cases liefert alle vorhandenen Fälle, sortiert nach ID.
func Cases(root string) ([]Case, error) {
	var out []Case
	for _, s := range []string{SuiteFOP, SuiteW3C} {
		files, err := filepath.Glob(filepath.Join(caseDir(root, s), "*.fo"))
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			name := strings.TrimSuffix(filepath.Base(f), ".fo")
			out = append(out, Case{Suite: s, Name: name, File: f, Category: Category(s, name)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out, nil
}

// ReadList liest eine Liste "suite/name: Begründung". Leerzeilen und Zeilen
// mit # am Anfang werden übersprungen. Ein Fall ohne Begründung ist ein Fehler.
func ReadList(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	n := 0
	for sc.Scan() {
		n++
		l := strings.TrimSpace(sc.Text())
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		id, why, ok := strings.Cut(l, ":")
		if !ok || strings.TrimSpace(why) == "" {
			return nil, fmt.Errorf("%s:%d: Fall ohne Begründung", path, n)
		}
		out[strings.TrimSpace(id)] = strings.TrimSpace(why)
	}
	return out, sc.Err()
}

// WriteList schreibt eine Liste sortiert mit Kopfkommentar.
func WriteList(path, header string, m map[string]string) error {
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var b strings.Builder
	for _, l := range strings.Split(strings.TrimSpace(header), "\n") {
		b.WriteString("# " + l + "\n")
	}
	b.WriteString("\n")
	for _, id := range ids {
		fmt.Fprintf(&b, "%s: %s\n", id, m[id])
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// Kategorien in der Reihenfolge des Berichts.
var Categories = []string{"Seitenfolgen", "Tabellen", "Listen", "Keeps", "Marker", "Verweise",
	"Zeilenumbruch", "Silbentrennung", "Leader und Links", "Sonstiges"}

// Category ordnet einen Fall nach seinem Namen einer Kategorie zu.
func Category(suite, name string) string {
	n := strings.ToLower(name)
	has := func(subs ...string) bool {
		for _, s := range subs {
			if strings.Contains(n, s) {
				return true
			}
		}
		return false
	}
	pre := func(ps ...string) bool {
		for _, p := range ps {
			if strings.HasPrefix(n, p) {
				return true
			}
		}
		return false
	}
	switch {
	case has("hyphenat"):
		return "Silbentrennung"
	case has("marker"):
		return "Marker"
	case has("keep", "widow", "orphan", "break-before", "break-after", "page-breaking", "breakbefore", "breakafter"):
		return "Keeps"
	case pre("table", "tcfo", "tfo", "tbfo", "trfo", "thfo", "tffo", "tcolfo"):
		return "Tabellen"
	case pre("list", "lbfo", "lifo", "libfo", "lilfo"):
		return "Listen"
	case has("citation", "page-number", "pagenumber", "pnfo"):
		return "Verweise"
	case has("leader", "basic-link", "bookmark", "link"):
		return "Leader und Links"
	case has("page-sequence", "page-master", "page-position", "region", "page-height", "page-width",
		"page-production", "force-page-count", "flow", "static-content", "pagelevel", "page-seq"):
		return "Seitenfolgen"
	case has("line-break", "linebreak", "text-align", "justif", "word", "white-space", "whitespace",
		"linefeed", "line-height", "text-indent", "letter-spacing", "inline", "block_", "space"):
		return "Zeilenumbruch"
	}
	return "Sonstiges"
}
