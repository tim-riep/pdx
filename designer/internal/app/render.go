package app

import (
	"context"
	"maps"
	"net/http"
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

// SetView wählt Ansicht und Beispieldatensatz (Pfad unter samples/, leer für
// den ersten) und setzt neu.
func (s *RenderService) SetView(view, sample string) {
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	if view != ViewDesign && view != ViewPreview {
		view = ViewPreview
	}
	s.c.r.view, s.c.r.sample = view, sample
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
	sample := c.r.sample
	if sample == "" {
		if s := p.Samples(); len(s) > 0 {
			sample = s[0]
		}
	}
	data, ok := p.Files[sample]
	if c.r.view == ViewDesign || !ok {
		// Entwurf, oder Vorschau ohne Beispieldaten: Platzhalter aus dem XSD.
		if sc, err := c.schemaLocked(); err == nil && sc != nil {
			data, sample, ok = sc.Placeholder(), "", true
		}
	}
	if !ok {
		data, sample = []byte("<daten/>"), ""
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
