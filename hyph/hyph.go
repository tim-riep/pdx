// Package hyph trennt Wörter nach dem Verfahren von Liang (1983) mit
// TeX-Trennmustern aus hyph-utf8. Eingebettet sind Deutsch in reformierter
// Rechtschreibung (de-1996) und amerikanisches Englisch (en-us), Lizenzen in
// patterns/README.md.
package hyph

import (
	"bufio"
	"bytes"
	"embed"
	"fmt"
	"strings"
	"sync"
	"unicode"
)

//go:embed patterns/*.txt
var files embed.FS

// Patterns ist ein geladener Satz Trennmuster.
type Patterns struct {
	pats       map[string][]uint8 // Buchstabenfolge zu Werten zwischen den Buchstaben
	maxLen     int
	exceptions map[string][]int
	// LeftMin und RightMin sind die Mindestzahl Buchstaben vor und nach einer
	// Trennstelle, wie in den Mustern vorgesehen.
	LeftMin, RightMin int
}

// Parse liest Muster im Textformat von hyph-utf8 (ein Muster je Zeile, etwa
// "a1b2c") und optional Ausnahmen (ein Wort je Zeile mit Bindestrichen).
func Parse(pat, hyp []byte, left, right int) (*Patterns, error) {
	p := &Patterns{pats: map[string][]uint8{}, exceptions: map[string][]int{}, LeftMin: left, RightMin: right}
	sc := bufio.NewScanner(bytes.NewReader(pat))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "%") {
			continue
		}
		var letters []rune
		var vals []uint8
		pending := uint8(0)
		for _, r := range line {
			if r >= '0' && r <= '9' {
				pending = uint8(r - '0')
				continue
			}
			vals = append(vals, pending)
			pending = 0
			letters = append(letters, r)
		}
		vals = append(vals, pending)
		if len(letters) == 0 {
			return nil, fmt.Errorf("Trennmuster %q ohne Buchstaben", line)
		}
		key := string(letters)
		p.pats[key] = vals
		p.maxLen = max(p.maxLen, len(letters))
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	sc = bufio.NewScanner(bytes.NewReader(hyp))
	for sc.Scan() {
		w := strings.TrimSpace(sc.Text())
		if w == "" || strings.HasPrefix(w, "%") {
			continue
		}
		var pos []int
		n := 0
		for _, r := range w {
			if r == '-' {
				pos = append(pos, n)
				continue
			}
			n++
		}
		p.exceptions[strings.ReplaceAll(strings.ToLower(w), "-", "")] = pos
	}
	return p, sc.Err()
}

// Hyphenate liefert die Trennstellen eines Wortes als Anzahl der Zeichen vor
// der Stelle. Berücksichtigt werden nur Buchstabenfolgen, Satzzeichen am
// Anfang und Ende bleiben unberührt. left und right sind die Mindestzahl
// Zeichen vor und nach einer Trennstelle (hyphenation-remain- und
// -push-character-count), 0 übernimmt die Werte der Muster.
func (p *Patterns) Hyphenate(word string, left, right int) []int {
	runes := []rune(word)
	start, end := 0, len(runes)
	for start < end && !unicode.IsLetter(runes[start]) {
		start++
	}
	for end > start && !unicode.IsLetter(runes[end-1]) {
		end--
	}
	core := runes[start:end]
	for _, r := range core {
		if !unicode.IsLetter(r) {
			return nil // zusammengesetzte Ausdrücke mit Ziffern oder Zeichen
		}
	}
	left = max(left, p.LeftMin, 1)
	right = max(right, p.RightMin, 1)
	if len(core) < left+right {
		return nil
	}
	lower := []rune(strings.ToLower(string(core)))
	if len(lower) != len(core) {
		return nil
	}
	var cuts []int
	if ex, ok := p.exceptions[string(lower)]; ok {
		cuts = ex
	} else {
		w := append(append([]rune{'.'}, lower...), '.')
		vals := make([]uint8, len(w)+1)
		for i := range w {
			for j := i + 1; j <= len(w) && j-i <= p.maxLen; j++ {
				if pv, ok := p.pats[string(w[i:j])]; ok {
					for k, v := range pv {
						if v > vals[i+k] {
							vals[i+k] = v
						}
					}
				}
			}
		}
		// vals[k] liegt vor w[k], w[0] ist der Punkt. Eine Stelle nach n
		// Buchstaben des Wortes entspricht vals[n+1].
		for n := 1; n < len(lower); n++ {
			if vals[n+1]%2 == 1 {
				cuts = append(cuts, n)
			}
		}
	}
	var out []int
	for _, c := range cuts {
		if c >= left && len(core)-c >= right {
			out = append(out, start+c)
		}
	}
	return out
}

var (
	mu    sync.Mutex
	cache = map[string]*Patterns{}
)

// languages ordnet Sprachkennungen den eingebetteten Mustern zu. Englische
// Varianten nutzen die amerikanischen Muster, Deutsch in der Schweiz und in
// Österreich die deutschen.
var languages = map[string]struct {
	file        string
	left, right int
}{
	"de": {"de-1996", 2, 2},
	"en": {"en-us", 2, 3},
}

// ForLanguage liefert die Muster zu einer Sprache nach BCP 47 oder XSL
// (language und country), etwa "de", "de-DE" oder "en-GB". ok ist false, wenn
// keine Muster vorhanden sind.
func ForLanguage(lang string) (*Patterns, bool, error) {
	l := strings.ToLower(strings.TrimSpace(lang))
	if i := strings.IndexAny(l, "-_"); i > 0 {
		l = l[:i]
	}
	spec, ok := languages[l]
	if !ok {
		return nil, false, nil
	}
	mu.Lock()
	defer mu.Unlock()
	if p, ok := cache[spec.file]; ok {
		return p, true, nil
	}
	pat, err := files.ReadFile("patterns/" + spec.file + ".pat.txt")
	if err != nil {
		return nil, false, err
	}
	hyp, _ := files.ReadFile("patterns/" + spec.file + ".hyp.txt")
	p, err := Parse(pat, hyp, spec.left, spec.right)
	if err != nil {
		return nil, false, fmt.Errorf("Trennmuster %s: %w", spec.file, err)
	}
	cache[spec.file] = p
	return p, true, nil
}
