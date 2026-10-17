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

// Registry ordnet Familiennamen, Strichstärke und Stil einer Schrift zu.
//
// Eine neue Registry kennt die eingebetteten Liberation-Schriften und
// bildet die PDF-Standardschriften sowie die generischen CSS-Familien darauf
// ab. Eigene Schriften kommen über Add oder AddFile hinzu und haben Vorrang.
type Registry struct {
	mu       sync.Mutex
	families map[string][]*Face
	aliases  map[string]string
	lazy     map[string][]string // Familie zu eingebetteten Dateien, erst bei Bedarf geladen
}

// builtinAliases bildet Standardschriften und generische Familien ab.
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

// fallbackFamily ist die Ersatzfamilie, wenn keine Familie der Liste
// vorhanden ist (XSL 1.1 7.9.3). Die generischen Familien cursive und fantasy
// haben keine eigene Schrift, die Liste wird dann weiter durchsucht.
const fallbackFamily = "liberation sans"

// NewRegistry legt eine Registry mit den eingebetteten Schriften an.
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

// Add registriert eine Schrift unter ihrem Familiennamen und optional weiteren
// Namen.
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

// AddFile lädt eine Schriftdatei und registriert sie.
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

// Lookup sucht eine Schrift. family darf eine kommagetrennte Liste wie in
// CSS sein, die erste gefundene Familie gewinnt. weight ist 100 bis 900.
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
	// XSL 1.1 7.9.3: Passt keine Schrift, wird systemabhängig ersetzt.
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

// pick wählt den Schnitt mit passendem Stil und nächster Strichstärke. Bei
// Gleichstand gewinnt die kräftigere Schrift ab 500, sonst die leichtere.
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
