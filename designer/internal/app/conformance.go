package app

import (
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/tim-riep/pdx"
	"github.com/tim-riep/pdx/designer/internal/pipeline"
	"github.com/tim-riep/pdx/pdf/render"
	"github.com/tim-riep/pdx/template"
	"github.com/tim-riep/pdx/verapdf"
)

// EventConformance meldet den Stand der Konformitätsprüfung.
const EventConformance = "conformance:changed"

// Zustände der Konformitätsprüfung.
const (
	ConformanceIdle    = "idle"
	ConformanceRunning = "running"
	ConformanceDone    = "done"
	ConformanceError   = "error"
)

// ConformanceItem ist eine fehlgeschlagene Einzelprüfung von veraPDF.
type ConformanceItem struct {
	// Ref ist das Vorlagenelement, wenn sich der Kontext zuordnen lässt.
	Ref string `json:"ref"`
	// Page ist die Seite ab 1, 0 ohne Seitenbezug.
	Page    int    `json:"page"`
	Context string `json:"context"`
	Message string `json:"message"`
}

// ConformanceFinding ist eine verletzte Regel mit ihren Einzelprüfungen.
type ConformanceFinding struct {
	Clause      string            `json:"clause"`
	Test        string            `json:"test"`
	Description string            `json:"description"`
	Count       int               `json:"count"`
	Items       []ConformanceItem `json:"items"`
}

// Conformance ist der Stand der Prüfung.
type Conformance struct {
	State string `json:"state"`
	// Rev ist die geprüfte Revision. Weicht sie vom Dokument ab, ist das
	// Ergebnis veraltet.
	Rev     uint64 `json:"rev"`
	Sample  string `json:"sample"` // leer für Platzhalterdaten
	Runner  string `json:"runner"`
	Profile string `json:"profile"`
	// Compliant gilt nur im Zustand done.
	Compliant bool                 `json:"compliant"`
	Findings  []ConformanceFinding `json:"findings"`
	// Problems sind Fehler beim Erzeugen des PDF.
	Problems []pipeline.Problem `json:"problems"`
	Notes    []string           `json:"notes"`
	Error    string             `json:"error"`
	Millis   int64              `json:"millis"`
}

// maxItems begrenzt die Einzelprüfungen je Regel.
const maxItems = 50

type conformance struct {
	state  Conformance
	gen    int
	cancel context.CancelFunc
	// detect wählt den Runner, in Tests eine Attrappe.
	detect func(ctx context.Context, warn func(string)) (verapdf.Runner, error)
	// now liefert das Erstellungsdatum des geprüften PDF.
	now func() time.Time
}

// ConformanceService prüft das erzeugte PDF mit veraPDF gegen PDF/A-3b.
type ConformanceService struct{ c *Core }

// NewConformanceService liefert die Sicht auf den gemeinsamen Zustand.
func NewConformanceService(c *Core) *ConformanceService { return &ConformanceService{c} }

// State liefert den letzten Stand.
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

// Check startet die Prüfung des aktuellen Stands im Hintergrund. Eine
// laufende Prüfung wird abgebrochen. Das Ergebnis meldet EventConformance.
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
	c.conf.state = Conformance{State: ConformanceRunning, Rev: c.doc.Rev(), Sample: sample, Profile: pdx.DefaultProfile}
	st := c.conf.stateCopy()
	c.emit.Emit(EventConformance, st)
	go c.runConformance(ctx, gen, in, st)
	return st, nil
}

// Cancel bricht eine laufende Prüfung ab.
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

// previewDataLocked liefert die Daten wie die Vorschau: Beispieldaten, sonst
// Platzhalter aus dem XSD.
func (c *Core) previewDataLocked() ([]byte, string) {
	p := c.doc.Pkg
	sample := c.r.sample
	if sample == "" {
		if s := p.Samples(); len(s) > 0 {
			sample = s[0]
		}
	}
	if b, ok := p.Files[sample]; ok {
		return b, sample
	}
	if sc, err := c.schemaLocked(); err == nil && sc != nil {
		return sc.Placeholder(), ""
	}
	return []byte("<daten/>"), ""
}

func (c *Core) runConformance(ctx context.Context, gen int, in pipeline.Input, st Conformance) {
	start := time.Now()
	finish := func(st Conformance) {
		st.Millis = time.Since(start).Milliseconds()
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.conf.gen != gen {
			return // abgebrochen oder ersetzt
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
		detect = verapdf.Detect
	}
	runner, err := detect(ctx, func(s string) { st.Notes = append(st.Notes, s) })
	if err != nil {
		fail(err)
		return
	}
	st.Runner = runner.Name()
	flavour, err := verapdf.Flavour(st.Profile)
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
	rep, err := runner.Validate(ctx, file, flavour)
	if err != nil {
		fail(err)
		return
	}
	if rep.Exception != "" {
		fail(errors.New("veraPDF: " + rep.Exception))
		return
	}
	st.State, st.Compliant = ConformanceDone, rep.Compliant
	st.Findings = mapFindings(rep, &trace)
	finish(st)
}

var (
	rePage = regexp.MustCompile(`/pages\[(\d+)\]`)
	// Nur Operatoren im Inhaltsstrom der Seite, nicht in Form-XObjects.
	reOp = regexp.MustCompile(`/pages\[\d+\][^/]*/contentStream\[0\][^/]*/operators\[(\d+)\]`)
)

// mapFindings ordnet die Kontexte der Einzelprüfungen über Seite und
// Operator den Vorlagenelementen zu. Kontexte ohne Seite (etwa Metadaten)
// gehören zum Dokument.
func mapFindings(rep *verapdf.Report, trace *render.Trace) []ConformanceFinding {
	out := []ConformanceFinding{}
	for _, f := range rep.Findings {
		cf := ConformanceFinding{Clause: f.Clause, Test: f.TestNumber, Description: f.Description, Count: f.FailedChecks, Items: []ConformanceItem{}}
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
