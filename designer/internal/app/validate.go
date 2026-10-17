package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"sort"
	"strings"
	"time"

	"github.com/tim-riep/pdx/check"
	"github.com/tim-riep/pdx/dataschema"
	"github.com/tim-riep/pdx/designer/internal/schema"
	"github.com/tim-riep/pdx/designer/internal/xpath"
	"github.com/tim-riep/pdx/template"
	"github.com/tim-riep/pdx/template/pkg"
)

// Problem ist ein Befund der laufenden Prüfung.
type Problem struct {
	// Level ist Fehler oder Hinweis.
	Level string `json:"level"`
	// Source ist vorlage (Vorlage, Verweise, Übersetzung, Bindungen) oder
	// beispiel (Beispieldaten gegen das Daten-XSD).
	Source string `json:"source"`
	// Ref ist das Vorlagenelement, leer bei Befunden ohne Bezug.
	Ref string `json:"ref"`
	// File ist bei Beispieldaten die Datei im Paket.
	File string `json:"file"`
	Msg  string `json:"msg"`
}

// Report sind die Befunde zu einer Revision.
type Report struct {
	Rev      uint64    `json:"rev"`
	Problems []Problem `json:"problems"`
	Millis   int64     `json:"millis"`
}

// validator hält die laufende Prüfung.
type validator struct {
	timer  *time.Timer
	gen    int
	key    string
	latest Report
}

// validateDelay ist die Pause nach einer Änderung bis zur Prüfung. Sie ist
// länger als beim Setzen, die Prüfung liest auch Schema und Beispieldaten.
const validateDelay = 250 * time.Millisecond

// ValidationService liefert die Befunde der laufenden Prüfung.
type ValidationService struct{ c *Core }

// NewValidationService liefert die Sicht auf den gemeinsamen Zustand.
func NewValidationService(c *Core) *ValidationService { return &ValidationService{c} }

// Problems liefert die zuletzt gemeldeten Befunde.
func (s *ValidationService) Problems() Report {
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	r := s.c.v.latest
	if r.Problems == nil {
		r.Problems = []Problem{}
	}
	return r
}

// validateLocked plant die Prüfung. Aufruf mit gehaltener Sperre.
func (c *Core) validateLocked(delay time.Duration) {
	if c.doc == nil {
		return
	}
	if c.v.timer != nil {
		c.v.timer.Stop()
	}
	c.v.timer = time.AfterFunc(delay, c.validate)
}

func (c *Core) validate() {
	c.mu.Lock()
	if c.doc == nil {
		c.mu.Unlock()
		return
	}
	canon := template.Write(c.doc.Pkg.Template)
	files := maps.Clone(c.doc.Pkg.Files)
	rev := c.doc.Rev()
	key := validationKey(canon, files)
	if key == c.v.key {
		// Inhalt unverändert, etwa nach einem Wechsel der Ansicht: nur die
		// Revision folgt.
		if c.v.latest.Rev != rev {
			c.v.latest.Rev = rev
			c.emit.Emit(EventProblems, c.v.latest)
		}
		c.mu.Unlock()
		return
	}
	c.v.gen++
	gen := c.v.gen
	c.mu.Unlock()

	start := time.Now()
	problems := runChecks(canon, files)
	r := Report{Rev: rev, Problems: problems, Millis: time.Since(start).Milliseconds()}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.v.gen != gen || c.doc == nil {
		return // eine neuere Prüfung läuft
	}
	c.v.key, c.v.latest = key, r
	c.emit.Emit(EventProblems, r)
}

func validationKey(canon []byte, files map[string][]byte) string {
	h := sha256.New()
	h.Write(canon)
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Fprintf(h, "\x00%s\x00%d\x00", n, len(files[n]))
		h.Write(files[n])
	}
	return hex.EncodeToString(h.Sum(nil))
}

// runChecks prüft einen Stand mit den Prüfungen von pdx check.
func runChecks(canon []byte, files map[string][]byte) []Problem {
	out := []Problem{}
	t, err := template.Parse(canon)
	if err != nil {
		return append(out, problemsOf(check.FromError(err))...)
	}
	p, err := pkg.New(t, files)
	if err != nil {
		return append(out, problemsOf(check.FromError(err))...)
	}
	out = append(out, problemsOf(check.Package(p))...)
	return append(out, presenceProblems(p)...)
}

// presenceProblems prüft die Pfade in Präsenzbedingungen gegen das
// Daten-XSD. Bindungen prüft schon check.Package.
func presenceProblems(p *pkg.Package) []Problem {
	sc, err := schema.Load(p)
	if err != nil || sc == nil {
		return nil
	}
	var out []Problem
	var walk func(n *template.Node)
	walk = func(n *template.Node) {
		if expr, ok := n.Attr("presence"); ok {
			if ctx, err := schema.Context(p.Template, n); err == nil {
				probs, _ := xpath.Paths(sc, ctx, expr)
				for _, pr := range probs {
					out = append(out, Problem{Level: string(dataschema.Warning), Source: "vorlage", Ref: n.NearestID(),
						Msg: "Präsenz: " + pr.Message})
				}
			}
		}
		for _, c := range n.Elements("") {
			walk(c)
		}
	}
	walk(p.Template.Root)
	return out
}

func problemsOf(fs []dataschema.Finding) []Problem {
	out := make([]Problem, 0, len(fs))
	for _, f := range fs {
		p := Problem{Level: string(f.Level), Source: "vorlage", Ref: f.ID, Msg: f.Msg}
		// Befunde zu Beispieldaten beginnen mit dem Dateinamen.
		if name, rest, ok := strings.Cut(f.Msg, " "); ok && f.ID == "" && strings.HasPrefix(name, "samples/") {
			p.Source, p.File, p.Msg = "beispiel", name, rest
		}
		out = append(out, p)
	}
	return out
}
