package fosuite

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tim-riep/pdx/internal/fopcompare"
)

const expectHeader = "Erwartung aus Apache FOP 2.11 (fop -r), erzeugt mit PDX_FOP=1 PDX_SUITE_UPDATE=1"

// TestReference erzeugt die Erwartungen aller aktiven Fälle mit FOP.
// PDX_SUITE=fop oder w3c beschränkt auf eine Suite.
func TestReference(t *testing.T) {
	if !fopcompare.Enabled() || os.Getenv("PDX_SUITE_UPDATE") != "1" {
		t.Skip("PDX_FOP=1 und PDX_SUITE_UPDATE=1 setzen, um Erwartungen mit FOP zu erzeugen")
	}
	ctx := context.Background()
	root, err := fopcompare.RepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	listPath := filepath.Join(DataDir(root), "disabled.txt")
	disabled, err := ReadList(listPath)
	if err != nil {
		t.Fatal(err)
	}
	cases, err := Cases(root)
	if err != nil {
		t.Fatal(err)
	}
	only := os.Getenv("PDX_SUITE")
	bySuite := map[string][]Case{}
	for _, c := range cases {
		if only != "" && c.Suite != only {
			continue
		}
		if why, ok := disabled[c.ID()]; ok {
			if !strings.HasPrefix(why, ReasonFOPFails) {
				continue
			}
			delete(disabled, c.ID()) // neu versuchen
		}
		bySuite[c.Suite] = append(bySuite[c.Suite], c)
	}
	for suite, cs := range bySuite {
		out := filepath.Join(root, "out", "suites", "ref", suite)
		// PDX_SUITE_REUSE=1 nutzt die Ausgaben von FOP und Poppler aus dem
		// letzten Lauf, etwa nach einer Änderung an der Auswertung.
		if os.Getenv("PDX_SUITE_REUSE") != "1" {
			os.RemoveAll(out)
			var files []string
			for _, c := range cs {
				files = append(files, c.File)
			}
			if err := fopcompare.RunFOPBatch(ctx, files, out, max(runtime.NumCPU()-2, 1)); err != nil {
				t.Fatal(err)
			}
			if err := fopcompare.PopplerBatch(ctx, out); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.MkdirAll(filepath.Join(DataDir(root), suite, "expect"), 0o755); err != nil {
			t.Fatal(err)
		}
		written := 0
		for _, c := range cs {
			e, why := expectFromFOP(out, c)
			if e == nil {
				disabled[c.ID()] = ReasonFOPFails + why
				os.Remove(ExpectFile(root, c))
				continue
			}
			if err := WriteExpect(ExpectFile(root, c), expectHeader, e); err != nil {
				t.Fatal(err)
			}
			written++
		}
		t.Logf("%s: %d Erwartungen geschrieben", suite, written)
	}
	if err := WriteList(listPath, disabledHeader, disabled); err != nil {
		t.Fatal(err)
	}
}

// expectFromFOP baut die Erwartung aus Area Tree und pdftotext-Ausgabe.
func expectFromFOP(out string, c Case) (*Expect, string) {
	at, err := os.ReadFile(filepath.Join(out, c.Name+".at.xml"))
	if err != nil {
		return nil, lastLogLine(filepath.Join(out, c.Name+".log"))
	}
	_, lines, pages, err := fopcompare.ParseAreaTreeLines(at)
	if err != nil {
		return nil, "Area Tree unlesbar: " + oneLine(err.Error())
	}
	type key struct {
		page int
		text string
	}
	xs := map[key][]float64{}
	if bb, err := os.ReadFile(filepath.Join(out, c.Name+".bbox.html")); err == nil {
		if pl, _, err := fopcompare.ParseBBox(bb); err == nil {
			for _, l := range pl {
				k := key{l.Page, fopcompare.Norm(l.Text)}
				xs[k] = append(xs[k], l.X)
			}
		}
	}
	e := &Expect{Pages: pages}
	for _, l := range lines {
		text := fopcompare.Norm(l.Text)
		if text == "" {
			continue
		}
		ln := Line{Page: l.Page, Y: l.Baseline, Text: textFor(c.Suite, l.Text)}
		k := key{l.Page, text}
		if q := xs[k]; len(q) > 0 {
			ln.X, ln.HasX = q[0], true
			xs[k] = q[1:]
		}
		e.Lines = append(e.Lines, ln)
	}
	return e, ""
}

func lastLogLine(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "keine Ausgabe"
	}
	for _, l := range strings.Split(string(data), "\n") {
		if strings.Contains(l, "SEVERE") || strings.Contains(l, "Exception:") {
			return oneLine(l)
		}
	}
	return "kein Area Tree"
}
