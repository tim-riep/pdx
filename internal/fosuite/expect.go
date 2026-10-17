package fosuite

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/tim-riep/pdx/internal/fopcompare"
)

// Expect ist die Erwartung an einen Fall, abgeleitet aus FOP.
type Expect struct {
	Pages int
	Lines []Line
}

// Line ist eine Zeile mit Grundlinie und, wenn bekannt, Anfang in Punkt.
type Line struct {
	Page int
	X    float64
	HasX bool
	Y    float64
	// Text ist der normierte Zeilentext, bei der W3C-Suite ein Hashwert.
	Text string
}

// HashText ersetzt einen Zeilentext durch einen Hashwert. Erwartungen der
// W3C-Suite enthalten so keine Inhalte der Testdateien.
func HashText(s string) string {
	h := sha256.Sum256([]byte(fopcompare.Norm(s)))
	return "#" + hex.EncodeToString(h[:8])
}

// textFor liefert den Text einer Zeile, wie er in der Erwartung steht.
func textFor(suite, s string) string {
	if suite == SuiteW3C {
		return HashText(s)
	}
	return fopcompare.Norm(s)
}

// ReadExpect liest eine Erwartung.
//
//	pages 2
//	1 56.693 85.123 Text der Zeile
//	1 - 99.120 Zeile ohne bekannten Anfang
func ReadExpect(path string) (*Expect, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	e := &Expect{Pages: -1}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	n := 0
	for sc.Scan() {
		n++
		l := sc.Text()
		// Kommentare beginnen am Zeilenanfang mit #, Hashwerte der W3C-Suite
		// stehen erst nach Seite und Lage.
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		if v, ok := strings.CutPrefix(l, "pages "); ok {
			e.Pages, err = strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("%s:%d: %v", path, n, err)
			}
			continue
		}
		parts := strings.SplitN(l, " ", 4)
		if len(parts) < 3 {
			return nil, fmt.Errorf("%s:%d: Zeile unvollständig", path, n)
		}
		var ln Line
		if ln.Page, err = strconv.Atoi(parts[0]); err != nil {
			return nil, fmt.Errorf("%s:%d: %v", path, n, err)
		}
		if parts[1] != "-" {
			if ln.X, err = strconv.ParseFloat(parts[1], 64); err != nil {
				return nil, fmt.Errorf("%s:%d: %v", path, n, err)
			}
			ln.HasX = true
		}
		if ln.Y, err = strconv.ParseFloat(parts[2], 64); err != nil {
			return nil, fmt.Errorf("%s:%d: %v", path, n, err)
		}
		if len(parts) == 4 {
			ln.Text = parts[3]
		}
		e.Lines = append(e.Lines, ln)
	}
	if e.Pages < 0 {
		return nil, fmt.Errorf("%s: pages fehlt", path)
	}
	return e, sc.Err()
}

// WriteExpect schreibt eine Erwartung mit Kopfkommentar.
func WriteExpect(path, header string, e *Expect) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n", header)
	fmt.Fprintf(&b, "pages %d\n", e.Pages)
	for _, l := range e.Lines {
		x := "-"
		if l.HasX {
			x = strconv.FormatFloat(l.X, 'f', 3, 64)
		}
		fmt.Fprintf(&b, "%d %s %s %s\n", l.Page, x, strconv.FormatFloat(l.Y, 'f', 3, 64), l.Text)
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
