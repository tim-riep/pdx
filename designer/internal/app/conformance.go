package app

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tim-riep/pdx"
	"github.com/tim-riep/pdx/designer/internal/pipeline"
	"github.com/tim-riep/pdx/pdf/render"
	"github.com/tim-riep/pdx/template"
	"github.com/tim-riep/pdx/verapdf"
)

// EventConformance reports the state of the conformance check.
const EventConformance = "conformance:changed"

// States of the conformance check.
const (
	ConformanceIdle    = "idle"
	ConformanceRunning = "running"
	ConformanceDone    = "done"
	ConformanceError   = "error"
)

// ConformanceItem is a failed single check of veraPDF.
type ConformanceItem struct {
	// Ref is the template element, if the context can be mapped.
	Ref string `json:"ref"`
	// Page is the page starting at 1, 0 without page reference.
	Page    int    `json:"page"`
	Context string `json:"context"`
	Message string `json:"message"`
}

// ConformanceFinding is a violated rule with its single checks.
type ConformanceFinding struct {
	// Specification is the standard of the rule, for example "ISO 14289-1:2014".
	Specification string            `json:"specification"`
	Clause        string            `json:"clause"`
	Test          string            `json:"test"`
	Description   string            `json:"description"`
	Count         int               `json:"count"`
	Items         []ConformanceItem `json:"items"`
}

// Conformance is the state of the check.
type Conformance struct {
	State string `json:"state"`
	// Rev is the checked revision. If it differs from the document, the
	// result is outdated.
	Rev     uint64 `json:"rev"`
	Sample  string `json:"sample"` // empty for placeholder data
	Runner  string `json:"runner"`
	Profile string `json:"profile"`
	// Compliant is only valid in the state done.
	Compliant bool                 `json:"compliant"`
	Findings  []ConformanceFinding `json:"findings"`
	// Problems are errors while generating the PDF.
	Problems []pipeline.Problem `json:"problems"`
	Notes    []string           `json:"notes"`
	Error    string             `json:"error"`
	Millis   int64              `json:"millis"`
}

// maxItems limits the single checks per rule.
const maxItems = 50

type conformance struct {
	state  Conformance
	gen    int
	cancel context.CancelFunc
	// detect chooses the runner, a fake in tests.
	detect func(ctx context.Context, warn func(string)) (verapdf.Runner, error)
	// now returns the creation date of the checked PDF.
	now func() time.Time
}

// ConformanceService checks the generated PDF with veraPDF against every
// standard that the profile of the template claims.
type ConformanceService struct{ c *Core }

// NewConformanceService returns the view on the shared state.
func NewConformanceService(c *Core) *ConformanceService { return &ConformanceService{c} }

// State returns the latest state.
func (s *ConformanceService) State() Conformance {
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	return s.c.conf.stateCopy()
}

func (c *conformance) stateCopy() Conformance {
	st := c.state
	if st.State == "" {
		st.State = ConformanceIdle
	}
	if st.Findings == nil {
		st.Findings = []ConformanceFinding{}
	}
	if st.Problems == nil {
		st.Problems = []pipeline.Problem{}
	}
	if st.Notes == nil {
		st.Notes = []string{}
	}
	return st
}

// Check starts the check of the current state in the background. A
// running check is canceled. EventConformance reports the result.
func (s *ConformanceService) Check() (Conformance, error) {
	c := s.c
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.doc == nil {
		return Conformance{}, errors.New("keine Vorlage geöffnet")
	}
	if c.conf.cancel != nil {
		c.conf.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.conf.cancel = cancel
	c.conf.gen++
	gen := c.conf.gen

	data, sample := c.previewDataLocked()
	in := pipeline.Input{Canon: template.Write(c.doc.Pkg.Template), Files: maps.Clone(c.doc.Pkg.Files), Data: data}
	prof := c.doc.Pkg.Template.Root.Get("profile", pdx.DefaultProfile)
	c.conf.state = Conformance{State: ConformanceRunning, Rev: c.doc.Rev(), Sample: sample, Profile: prof}
	st := c.conf.stateCopy()
	c.emit.Emit(EventConformance, st)
	go c.runConformance(ctx, gen, in, st)
	return st, nil
}

// VeraPDFSetup describes which veraPDF the designer uses.
type VeraPDFSetup struct {
	// Path is the chosen path, empty means automatic.
	Path string `json:"path"`
	// Runner describes the variant found, Error why none was found,
	// Notes hints such as a different version.
	Runner string   `json:"runner"`
	Error  string   `json:"error"`
	Notes  []string `json:"notes"`
	// Version is the required version.
	Version string `json:"version"`
}

type verapdfSettings struct {
	Path string `json:"path"`
}

func (c *Core) verapdfSettingsPath() string { return filepath.Join(c.dataDir, "verapdf.json") }

// verapdfPath reads the chosen path from the settings.
func (c *Core) verapdfPath() string {
	var st verapdfSettings
	if b, err := os.ReadFile(c.verapdfSettingsPath()); err == nil {
		json.Unmarshal(b, &st)
	}
	return st.Path
}

// VeraPDF shows which veraPDF is found: first the chosen path,
// then PDX_VERAPDF, otherwise the search in PATH and the usual folders,
// finally Docker.
func (s *ConformanceService) VeraPDF() VeraPDFSetup {
	path := s.c.verapdfPath()
	setup := VeraPDFSetup{Path: path, Version: verapdf.Version, Notes: []string{}}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	r, err := verapdf.DetectWith(ctx, verapdf.Options{Path: path}, func(n string) { setup.Notes = append(setup.Notes, n) })
	if err != nil {
		setup.Error = err.Error()
	} else {
		setup.Runner = r.Name()
	}
	return setup
}

// SetVeraPDF stores a path to the veraPDF CLI, empty switches back to
// the automatic search. A path must point to a working CLI.
func (s *ConformanceService) SetVeraPDF(path string) (VeraPDFSetup, error) {
	path = strings.TrimSpace(path)
	if path != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if _, err := verapdf.DetectWith(ctx, verapdf.Options{Path: path}, nil); err != nil {
			return s.VeraPDF(), err
		}
	}
	b, _ := json.MarshalIndent(verapdfSettings{Path: path}, "", "  ")
	os.MkdirAll(s.c.dataDir, 0o755)
	if err := os.WriteFile(s.c.verapdfSettingsPath(), b, 0o600); err != nil {
		return s.VeraPDF(), err
	}
	return s.VeraPDF(), nil
}

// Cancel cancels a running check.
func (s *ConformanceService) Cancel() {
	c := s.c
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conf.cancel != nil {
		c.conf.cancel()
		c.conf.cancel = nil
	}
	if c.conf.state.State == ConformanceRunning {
		c.conf.gen++
		c.conf.state = Conformance{State: ConformanceIdle}
		c.emit.Emit(EventConformance, c.conf.stateCopy())
	}
}

// previewDataLocked returns the data like the preview: sample data,
// otherwise placeholders from the XSD.
func (c *Core) previewDataLocked() ([]byte, string) {
	p := c.doc.Pkg
	// The selection applies as long as the file exists, otherwise the first.
	if b, ok := p.Files[c.r.sample]; ok && c.r.sample != "" {
		return b, c.r.sample
	}
	if s := p.Samples(); len(s) > 0 {
		return p.Files[s[0]], s[0]
	}
	return c.placeholderLocked(), ""
}

// placeholderLocked returns placeholder data: from the data XSD, otherwise an
// empty root element after template/@dataRoot with its namespace, so that
// the transformation finds the expected root.
func (c *Core) placeholderLocked() []byte {
	if sc, err := c.schemaLocked(); err == nil && sc != nil {
		return sc.Placeholder()
	}
	root := c.doc.Pkg.Template.Root
	name := root.Get("dataRoot", "")
	if name == "" {
		return []byte("<daten/>")
	}
	prefix, _, ok := strings.Cut(name, ":")
	if !ok {
		return []byte("<" + name + "/>")
	}
	uri := ""
	if nss := root.First("namespaces"); nss != nil {
		for _, ns := range nss.Elements("ns") {
			if ns.Get("prefix", "") == prefix {
				uri = ns.Get("uri", "")
			}
		}
	}
	var b strings.Builder
	b.WriteString("<" + name + " xmlns:" + prefix + `="`)
	xml.EscapeText(&b, []byte(uri))
	b.WriteString(`"/>`)
	return []byte(b.String())
}

func (c *Core) runConformance(ctx context.Context, gen int, in pipeline.Input, st Conformance) {
	start := time.Now()
	finish := func(st Conformance) {
		st.Millis = time.Since(start).Milliseconds()
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.conf.gen != gen {
			return // canceled or replaced
		}
		c.conf.state, c.conf.cancel = st, nil
		c.emit.Emit(EventConformance, c.conf.stateCopy())
	}
	fail := func(err error) {
		if ctx.Err() != nil {
			return
		}
		st.State, st.Error = ConformanceError, err.Error()
		finish(st)
	}

	now := c.conf.now
	if now == nil {
		now = time.Now
	}
	var trace render.Trace
	pdf, probs, err := c.r.pipe.PDF(ctx, in, now().UTC().Truncate(time.Second), &trace)
	if err != nil {
		fail(err)
		return
	}
	if len(probs) > 0 {
		st.State, st.Problems = ConformanceError, probs
		st.Error = "Das PDF ließ sich nicht erzeugen"
		finish(st)
		return
	}

	detect := c.conf.detect
	if detect == nil {
		opts := verapdf.Options{Path: c.verapdfPath()}
		detect = func(ctx context.Context, warn func(string)) (verapdf.Runner, error) {
			return verapdf.DetectWith(ctx, opts, warn)
		}
	}
	runner, err := detect(ctx, func(s string) { st.Notes = append(st.Notes, s) })
	if err != nil {
		fail(err)
		return
	}
	st.Runner = runner.Name()
	flavours, err := verapdf.Flavours(st.Profile)
	if err != nil {
		fail(err)
		return
	}
	dir, err := os.MkdirTemp("", "pdx-konformitaet-")
	if err != nil {
		fail(err)
		return
	}
	defer os.RemoveAll(dir)
	file := filepath.Join(dir, "dokument.pdf")
	if err := os.WriteFile(file, pdf, 0o644); err != nil {
		fail(err)
		return
	}
	// veraPDF checks one flavour per run, a combination needs several.
	compliant := true
	var findings []ConformanceFinding
	for _, flavour := range flavours {
		rep, err := runner.Validate(ctx, file, flavour)
		if err != nil {
			fail(err)
			return
		}
		if rep.Exception != "" {
			fail(errors.New("veraPDF: " + rep.Exception))
			return
		}
		compliant = compliant && rep.Compliant
		findings = append(findings, mapFindings(rep, &trace)...)
	}
	if findings == nil {
		findings = []ConformanceFinding{}
	}
	st.State, st.Compliant = ConformanceDone, compliant
	st.Findings = findings
	finish(st)
}

var (
	rePage = regexp.MustCompile(`/pages\[(\d+)\]`)
	// Only operators in the content stream of the page, not in form XObjects.
	reOp = regexp.MustCompile(`/pages\[\d+\][^/]*/contentStream\[0\][^/]*/operators\[(\d+)\]`)
)

// mapFindings maps the contexts of the single checks to the template
// elements via page and operator. Contexts without a page (such as metadata)
// belong to the document.
func mapFindings(rep *verapdf.Report, trace *render.Trace) []ConformanceFinding {
	out := []ConformanceFinding{}
	for _, f := range rep.Findings {
		cf := ConformanceFinding{Specification: f.Specification, Clause: f.Clause, Test: f.TestNumber, Description: f.Description, Count: f.FailedChecks, Items: []ConformanceItem{}}
		if cf.Count == 0 {
			cf.Count = len(f.Checks)
		}
		for _, ch := range f.Checks {
			if len(cf.Items) >= maxItems {
				break
			}
			it := ConformanceItem{Context: ch.Context, Message: ch.Message}
			if m := rePage.FindStringSubmatch(ch.Context); m != nil {
				page, _ := strconv.Atoi(m[1])
				it.Page = page + 1
				if o := reOp.FindStringSubmatch(ch.Context); o != nil {
					op, _ := strconv.Atoi(o[1])
					it.Ref = trace.IDAt(page, op)
				}
			}
			cf.Items = append(cf.Items, it)
		}
		out = append(out, cf)
	}
	return out
}
