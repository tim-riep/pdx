package font

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/tim-riep/pdx/font/fonts"
)

// Registry maps family names, weight and style to a font.
//
// A new Registry knows the embedded Liberation fonts and
// maps the PDF standard fonts and the generic CSS families to them.
// Custom fonts are added via Add or AddFile and take precedence.
type Registry struct {
	mu       sync.Mutex
	families map[string][]*Face
	aliases  map[string]string
	lazy     map[string][]string // family to embedded files, loaded only when needed
}

// builtinAliases maps standard fonts and generic families.
var builtinAliases = map[string]string{
	"helvetica":       "liberation sans",
	"arial":           "liberation sans",
	"sans-serif":      "liberation sans",
	"times":           "liberation serif",
	"times-roman":     "liberation serif",
	"times new roman": "liberation serif",
	"serif":           "liberation serif",
	"courier":         "liberation mono",
	"courier new":     "liberation mono",
	"monospace":       "liberation mono",
}

// fallbackFamily is the fallback family if no family of the list
// is available (XSL 1.1 7.9.3). The generic families cursive and fantasy
// have no font of their own, the list is then searched further.
const fallbackFamily = "liberation sans"

// NewRegistry creates a Registry with the embedded fonts.
func NewRegistry() *Registry {
	r := &Registry{
		families: map[string][]*Face{},
		aliases:  map[string]string{},
		lazy:     map[string][]string{},
	}
	for k, v := range builtinAliases {
		r.aliases[k] = v
	}
	entries, _ := fs.ReadDir(fonts.FS, "liberation")
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".ttf") {
			continue
		}
		fam := "liberation " + strings.ToLower(strings.TrimPrefix(strings.SplitN(name, "-", 2)[0], "Liberation"))
		r.lazy[fam] = append(r.lazy[fam], path.Join("liberation", name))
	}
	return r
}

// Add registers a font under its family name and optionally further
// names.
func (r *Registry) Add(f *Face, names ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, n := range append([]string{f.Family}, names...) {
		k := normalize(n)
		if k == "" || slices.Contains(r.families[k], f) {
			continue
		}
		r.families[k] = append(r.families[k], f)
		delete(r.aliases, k)
	}
}

// AddFile loads a font file and registers it.
func (r *Registry) AddFile(file string, names ...string) (*Face, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	f, err := Load(data, 0)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	r.Add(f, names...)
	return f, nil
}

// Lookup looks up a font. family may be a comma-separated list as in
// CSS, the first family found wins. weight is 100 to 900.
func (r *Registry) Lookup(family string, weight int, italic bool) (*Face, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, part := range strings.Split(family, ",") {
		k := normalize(part)
		if a, ok := r.aliases[k]; ok {
			k = a
		}
		if err := r.loadLazy(k); err != nil {
			return nil, err
		}
		if faces := r.families[k]; len(faces) > 0 {
			return pick(faces, weight, italic), nil
		}
	}
	// XSL 1.1 7.9.3: If no font matches, a system-dependent substitution is made.
	if err := r.loadLazy(fallbackFamily); err != nil {
		return nil, err
	}
	if faces := r.families[fallbackFamily]; len(faces) > 0 {
		return pick(faces, weight, italic), nil
	}
	return nil, fmt.Errorf("Schriftfamilie %q nicht gefunden", strings.TrimSpace(family))
}

func (r *Registry) loadLazy(fam string) error {
	files, ok := r.lazy[fam]
	if !ok {
		return nil
	}
	delete(r.lazy, fam)
	sort.Strings(files)
	for _, file := range files {
		data, err := fs.ReadFile(fonts.FS, file)
		if err != nil {
			return err
		}
		f, err := Load(data, 0)
		if err != nil {
			return fmt.Errorf("eingebettete Schrift %s: %w", file, err)
		}
		r.families[fam] = append(r.families[fam], f)
	}
	return nil
}

// pick selects the face with matching style and nearest weight. On a
// tie the bolder font wins from 500 upward, else the lighter one.
func pick(faces []*Face, weight int, italic bool) *Face {
	var best *Face
	bestScore := 1 << 30
	for _, f := range faces {
		score := abs(f.Weight - weight)
		if f.Italic != italic {
			score += 10000
		}
		if score < bestScore || (score == bestScore && best != nil && ((weight >= 500) == (f.Weight > best.Weight))) {
			best, bestScore = f, score
		}
	}
	return best
}

func normalize(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	return strings.Trim(s, `"'`)
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
