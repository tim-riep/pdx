// Package hyph hyphenates words with Liang's algorithm (1983) using
// TeX hyphenation patterns from hyph-utf8. Embedded are German in reformed
// spelling (de-1996) and American English (en-us), licenses in
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

// Patterns is a loaded set of hyphenation patterns.
type Patterns struct {
	pats       map[string][]uint8 // letter sequence to values between the letters
	maxLen     int
	exceptions map[string][]int
	// LeftMin and RightMin are the minimum number of letters before and after a
	// hyphenation point, as specified by the patterns.
	LeftMin, RightMin int
}

// Parse reads patterns in the hyph-utf8 text format (one pattern per line,
// such as "a1b2c") and optional exceptions (one word per line with hyphens).
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

// Hyphenate returns the hyphenation points of a word as the number of
// characters before the point. Only letter sequences are considered,
// punctuation at the start and end is left untouched. left and right are the
// minimum number of characters before and after a hyphenation point
// (hyphenation-remain- and -push-character-count), 0 uses the pattern values.
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
			return nil // compound expressions with digits or symbols
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
		// vals[k] lies before w[k], w[0] is the dot. A point after n
		// letters of the word corresponds to vals[n+1].
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

// languages maps language codes to the embedded patterns. English
// variants use the American patterns, German in Switzerland and in
// Austria uses the German ones.
var languages = map[string]struct {
	file        string
	left, right int
}{
	"de": {"de-1996", 2, 2},
	"en": {"en-us", 2, 3},
}

// ForLanguage returns the patterns for a language given as BCP 47 or XSL
// (language and country), such as "de", "de-DE" or "en-GB". ok is false if
// no patterns are available.
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
