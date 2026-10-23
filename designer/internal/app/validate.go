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

// Problem is a finding of the running check.
type Problem struct {
	// Level is error or hint.
	Level string `json:"level"`
	// Source is vorlage (template, references, compilation, bindings) or
	// beispiel (sample data against the data XSD).
	Source string `json:"source"`
	// Ref is the template element, empty for findings without reference.
	Ref string `json:"ref"`
	// File is the file in the package for sample data.
	File string `json:"file"`
	Msg  string `json:"msg"`
}

// Report is the findings for a revision.
type Report struct {
	Rev      uint64    `json:"rev"`
	Problems []Problem `json:"problems"`
	Millis   int64     `json:"millis"`
}

// validator holds the running check.
type validator struct {
	timer  *time.Timer
	gen    int
	key    string
	latest Report
}

// validateDelay is the pause after a change until the check. It is
// longer than for typesetting, the check also reads schema and sample data.
const validateDelay = 250 * time.Millisecond

// ValidationService returns the findings of the running check.
type ValidationService struct{ c *Core }

// NewValidationService returns the view on the shared state.
func NewValidationService(c *Core) *ValidationService { return &ValidationService{c} }

// Problems returns the most recently reported findings.
func (s *ValidationService) Problems() Report {
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	r := s.c.v.latest
	if r.Problems == nil {
		r.Problems = []Problem{}
	}
	return r
}

// validateLocked schedules the check. Call with the lock held.
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
		// Content unchanged, for example after switching the view: only the
		// revision follows.
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
		return // a newer check is running
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

// runChecks checks a state with the checks of pdx check.
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

// presenceProblems checks the paths in presence conditions against the
// data XSD. check.Package already checks bindings.
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
		// Findings for sample data start with the file name.
		if name, rest, ok := strings.Cut(f.Msg, " "); ok && f.ID == "" && strings.HasPrefix(name, "samples/") {
			p.Source, p.File, p.Msg = "beispiel", name, rest
		}
		out = append(out, p)
	}
	return out
}
