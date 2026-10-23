package app

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tim-riep/pdx"
	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/designer/internal/pipeline"
	"github.com/tim-riep/pdx/einvoice"
	"github.com/tim-riep/pdx/pdf/profile"
	"github.com/tim-riep/pdx/template"
)

// Views of the canvas.
const (
	ViewDesign  = "design"
	ViewPreview = "preview"
)

// Manifest describes the typeset pages of a state. The UI fetches the pages
// themselves via /pdx/page/<hash>, only changed hashes are new.
type Manifest struct {
	View   string `json:"view"`
	Rev    uint64 `json:"rev"`
	Sample string `json:"sample"`
	pipeline.Result
}

// RenderService typesets the template in the background.
type RenderService struct{ c *Core }

// NewRenderService returns the view on the shared state.
func NewRenderService(c *Core) *RenderService { return &RenderService{c} }

// renderDelay is the wait time after the last change until typesetting.
const renderDelay = 80 * time.Millisecond

type renderer struct {
	pipe   *pipeline.Pipeline
	view   string
	sample string
	timer  *time.Timer
	cancel context.CancelFunc
	latest Manifest
	gen    uint64 // counter of the runs, only the newest is reported
}

// SetView chooses the view and the sample data set (path under samples/,
// empty keeps the selection) and typesets again.
func (s *RenderService) SetView(view, sample string) {
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	if view != ViewDesign && view != ViewPreview {
		view = ViewPreview
	}
	s.c.r.view = view
	if sample != "" {
		s.c.r.sample = sample
	}
	s.c.scheduleLocked(0)
}

// Access analyzes the structure tree and the accessibility pre-check for
// the preview data, without writing a PDF.
func (s *RenderService) Access() (*pipeline.Access, error) {
	c := s.c
	c.mu.Lock()
	if c.doc == nil {
		c.mu.Unlock()
		return nil, errors.New("keine Vorlage geöffnet")
	}
	data, _ := c.previewDataLocked()
	in := pipeline.Input{Canon: template.Write(c.doc.Pkg.Template), Files: maps.Clone(c.doc.Pkg.Files), Data: data}
	c.mu.Unlock()
	return c.r.pipe.Access(context.Background(), in)
}

// EInvoice describes the e-invoice of a template with the preview
// data: detected level, file name, relationship and whether the profile fits.
type EInvoice struct {
	Present      bool   `json:"present"`
	Level        string `json:"level"`
	Guideline    string `json:"guideline"`
	FileName     string `json:"fileName"`
	Relationship string `json:"relationship"`
	// Overridden reports that the template sets the relationship.
	Overridden bool   `json:"overridden"`
	Sample     string `json:"sample"`
	Error      string `json:"error"`
	// ProfileError is set if the output profile is not PDF/A-3.
	ProfileError string `json:"profileError"`
}

// EInvoice detects the e-invoice in the preview data.
func (s *RenderService) EInvoice() (EInvoice, error) {
	c := s.c
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.doc == nil {
		return EInvoice{}, errors.New("keine Vorlage geöffnet")
	}
	root := c.doc.Pkg.Template.Root
	e := root.First("einvoice")
	if e == nil {
		return EInvoice{}, nil
	}
	data, sample := c.previewDataLocked()
	out := EInvoice{Present: true, Sample: sample}
	spec := root.Get("profile", pdx.DefaultProfile)
	if p, err := profile.Lookup(spec); err != nil {
		out.ProfileError = err.Error()
	} else if profile.ClaimOf(p).PDFA != 3 {
		out.ProfileError = "E-Rechnungen verlangen PDF/A-3, das Ausgabeprofil ist " + spec
	}
	info, err := einvoice.Detect(data)
	if err != nil {
		out.Error = err.Error()
		return out, nil
	}
	rel := areatree.Relationship(e.Get("afRelationship", ""))
	a := info.Attachment(nil, rel)
	out.Level, out.Guideline, out.FileName = info.Level.Name, info.Guideline, a.Name
	out.Relationship, out.Overridden = string(a.Relationship), rel != ""
	return out, nil
}

// Latest returns the most recently typeset manifest.
func (s *RenderService) Latest() Manifest {
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	return s.c.r.latest
}

// scheduleLocked schedules typesetting after delay. A running typesetting
// run is canceled as soon as the new one starts.
func (c *Core) scheduleLocked(delay time.Duration) {
	if c.doc == nil {
		return
	}
	if c.r.timer != nil {
		c.r.timer.Stop()
	}
	c.r.timer = time.AfterFunc(delay, c.render)
	c.validateLocked(max(delay, validateDelay))
}

// render typesets the current state and reports the result.
func (c *Core) render() {
	c.mu.Lock()
	if c.doc == nil {
		c.mu.Unlock()
		return
	}
	if c.r.cancel != nil {
		c.r.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.r.cancel = cancel
	c.r.gen++
	gen := c.r.gen
	p := c.doc.Pkg
	var data []byte
	var sample string
	if c.r.view == ViewDesign {
		data = c.placeholderLocked()
	} else {
		data, sample = c.previewDataLocked()
	}
	// Copy the file list: commands change it while typesetting runs.
	in := pipeline.Input{Canon: template.Write(p.Template), Files: maps.Clone(p.Files), Data: data}
	rev, view := c.doc.Rev(), c.r.view
	c.mu.Unlock()

	res, err := c.r.pipe.Run(ctx, in)
	if err != nil {
		return // canceled, a newer run follows
	}
	m := Manifest{View: view, Rev: rev, Sample: sample, Result: *res}
	c.mu.Lock()
	if c.doc != nil && c.doc.Rev() == rev && c.r.gen == gen {
		c.r.latest = m
		c.emit.Emit(EventRenderPages, m)
	}
	c.mu.Unlock()
}

// Assets serves pages, fonts and images of the canvas over HTTP under
// /pdx/. The contents are addressed by their hash and never change.
type Assets struct{ c *Core }

// NewAssets returns the handler.
func NewAssets(c *Core) *Assets { return &Assets{c} }

func (a *Assets) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/pdx")
	kind, key, ok := strings.Cut(strings.TrimPrefix(path, "/"), "/")
	if !ok || key == "" || strings.Contains(key, "/") {
		http.NotFound(w, r)
		return
	}
	pipe := a.c.r.pipe
	if kind == "pdf" {
		// The PDF of the current state for the PDF view. key is the
		// revision and only serves to avoid caching old responses.
		a.servePDF(w, r)
		return
	}
	var body []byte
	var mime string
	switch kind {
	case "page":
		body, ok = pipe.PageJSON(key)
		mime = "application/json"
	case "font":
		body, ok = pipe.Font(key)
		mime = "font/ttf"
	case "image":
		body, mime, ok = pipe.Image(key)
	default:
		ok = false
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Write(body)
}

// currentPDF generates the PDF/A-3b of the current state with the preview
// data. Findings are in probs, err reports an abort.
func (c *Core) currentPDF(ctx context.Context) (pdf []byte, probs []pipeline.Problem, err error) {
	c.mu.Lock()
	if c.doc == nil {
		c.mu.Unlock()
		return nil, nil, errors.New("keine Vorlage geöffnet")
	}
	data, _ := c.previewDataLocked()
	in := pipeline.Input{Canon: template.Write(c.doc.Pkg.Template), Files: maps.Clone(c.doc.Pkg.Files), Data: data}
	c.mu.Unlock()
	return c.r.pipe.PDF(ctx, in, time.Now().UTC().Truncate(time.Second), nil)
}

// pdfError combines findings during generation into one error.
func pdfError(probs []pipeline.Problem) error {
	var b strings.Builder
	b.WriteString("Das PDF ließ sich nicht erzeugen:")
	for _, p := range probs {
		b.WriteString(" ")
		if p.ID != "" {
			b.WriteString("[" + p.ID + "] ")
		}
		b.WriteString(p.Msg)
	}
	return errors.New(b.String())
}

// servePDF serves the PDF of the current state. Errors during generation
// come as text with status 422.
func (a *Assets) servePDF(w http.ResponseWriter, r *http.Request) {
	pdf, probs, err := a.c.currentPDF(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if len(probs) > 0 {
		var b strings.Builder
		for _, p := range probs {
			if p.ID != "" {
				b.WriteString("[" + p.ID + "] ")
			}
			b.WriteString(p.Msg + "\n")
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(b.String()))
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(pdf)
}

// ExportPDF writes the PDF of the current state to a file.
func (s *RenderService) ExportPDF(path string) error {
	pdf, probs, err := s.c.currentPDF(context.Background())
	if err != nil {
		return err
	}
	if len(probs) > 0 {
		return pdfError(probs)
	}
	return os.WriteFile(path, pdf, 0o644)
}

// ExportPDFDialog asks for the target and writes the PDF. A
// canceled dialog returns an empty path.
func (s *RenderService) ExportPDFDialog() (string, error) {
	if s.c.dialogs == nil {
		return "", ErrNoDialogs
	}
	s.c.mu.Lock()
	name := "dokument"
	if s.c.doc != nil && s.c.doc.Path != "" {
		name = strings.TrimSuffix(filepath.Base(s.c.doc.Path), filepath.Ext(s.c.doc.Path))
	}
	s.c.mu.Unlock()
	path, err := s.c.dialogs.SaveFile("PDF speichern", name+".pdf", []Filter{{Name: "PDF (*.pdf)", Pattern: "*.pdf"}})
	if err != nil || path == "" {
		return "", err
	}
	if !strings.HasSuffix(strings.ToLower(path), ".pdf") {
		path += ".pdf"
	}
	return path, s.ExportPDF(path)
}

// OpenPDF writes the PDF into the data directory and opens it with the
// system's PDF program.
func (s *RenderService) OpenPDF() error {
	if s.c.dialogs == nil {
		return ErrNoDialogs
	}
	dir := filepath.Join(s.c.dataDir, "pdf")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "vorschau.pdf")
	if err := s.ExportPDF(path); err != nil {
		return err
	}
	return s.c.dialogs.Show(path)
}
