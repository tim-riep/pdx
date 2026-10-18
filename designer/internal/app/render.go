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

	"github.com/tim-riep/pdx/designer/internal/pipeline"
	"github.com/tim-riep/pdx/template"
)

// Ansichten des Canvas.
const (
	ViewDesign  = "design"
	ViewPreview = "preview"
)

// Manifest beschreibt die gesetzten Seiten eines Stands. Die Seiten selbst
// holt die Oberfläche über /pdx/page/<hash>, nur geänderte Hashes sind neu.
type Manifest struct {
	View   string `json:"view"`
	Rev    uint64 `json:"rev"`
	Sample string `json:"sample"`
	pipeline.Result
}

// RenderService setzt die Vorlage im Hintergrund.
type RenderService struct{ c *Core }

// NewRenderService liefert die Sicht auf den gemeinsamen Zustand.
func NewRenderService(c *Core) *RenderService { return &RenderService{c} }

// renderDelay ist die Wartezeit nach der letzten Änderung bis zum Setzen.
const renderDelay = 80 * time.Millisecond

type renderer struct {
	pipe   *pipeline.Pipeline
	view   string
	sample string
	timer  *time.Timer
	cancel context.CancelFunc
	latest Manifest
	gen    uint64 // Zähler der Läufe, nur der neueste wird gemeldet
}

// SetView wählt Ansicht und Beispieldatensatz (Pfad unter samples/, leer
// behält die Auswahl) und setzt neu.
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

// Latest liefert das zuletzt gesetzte Manifest.
func (s *RenderService) Latest() Manifest {
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	return s.c.r.latest
}

// scheduleLocked plant das Setzen nach delay. Ein laufendes Setzen wird
// abgebrochen, sobald das neue beginnt.
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

// render setzt den aktuellen Stand und meldet das Ergebnis.
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
	// Die Dateiliste kopieren: Befehle ändern sie, während gesetzt wird.
	in := pipeline.Input{Canon: template.Write(p.Template), Files: maps.Clone(p.Files), Data: data}
	rev, view := c.doc.Rev(), c.r.view
	c.mu.Unlock()

	res, err := c.r.pipe.Run(ctx, in)
	if err != nil {
		return // abgebrochen, ein neuerer Lauf folgt
	}
	m := Manifest{View: view, Rev: rev, Sample: sample, Result: *res}
	c.mu.Lock()
	if c.doc != nil && c.doc.Rev() == rev && c.r.gen == gen {
		c.r.latest = m
		c.emit.Emit(EventRenderPages, m)
	}
	c.mu.Unlock()
}

// Assets liefert Seiten, Schriften und Bilder des Canvas über HTTP unter
// /pdx/. Die Inhalte sind über ihren Hash adressiert und ändern sich nie.
type Assets struct{ c *Core }

// NewAssets liefert den Handler.
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
		// Das PDF des aktuellen Stands für die Ansicht PDF. key ist die
		// Revision und dient nur dazu, alte Antworten nicht zu cachen.
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

// currentPDF erzeugt das PDF/A-3b des aktuellen Stands mit den Daten der
// Vorschau. Befunde stehen in probs, err meldet einen Abbruch.
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

// pdfError fasst Befunde beim Erzeugen zu einem Fehler zusammen.
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

// servePDF liefert das PDF des aktuellen Stands. Fehler beim Erzeugen kommen
// als Text mit Status 422.
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

// ExportPDF schreibt das PDF des aktuellen Stands in eine Datei.
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

// ExportPDFDialog fragt nach dem Ziel und schreibt das PDF. Ein
// abgebrochener Dialog liefert einen leeren Pfad.
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

// OpenPDF schreibt das PDF in das Datenverzeichnis und öffnet es mit dem
// PDF-Programm des Systems.
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
