package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tim-riep/pdx/designer/internal/model"
	"github.com/tim-riep/pdx/designer/internal/pipeline"
)

func TestRender(t *testing.T) {
	rec := &recorder{}
	c := NewCore(rec, nil, t.TempDir())
	if _, err := NewProjectService(c).Open(muster); err != nil {
		t.Fatal(err)
	}
	rs := NewRenderService(c)
	rs.SetView(ViewPreview, "")
	m := rec.wait(t, EventRenderPages, func(d any) bool { return len(d.(Manifest).Pages) > 0 }).(Manifest)
	if len(m.Pages) != 3 || m.Sample != "samples/rechnung.xml" || len(m.Problems) > 0 {
		t.Fatalf("Manifest: %+v", m)
	}
	if got := rs.Latest(); got.Rev != m.Rev || len(got.Pages) != 3 {
		t.Errorf("Latest: %+v", got)
	}

	h := NewAssets(c)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/pdx/page/"+m.Pages[0].Hash, nil))
	var page pipeline.Page
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &page) != nil || page.N != 1 {
		t.Fatalf("Seite 1: %d %s", w.Code, w.Body.String()[:min(200, w.Body.Len())])
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/pdx/font/"+m.Fonts[0].Key, nil))
	if w.Code != http.StatusOK || w.Body.Len() < 1000 {
		t.Errorf("Schrift: %d", w.Code)
	}
	for _, bad := range []string{"/pdx/page/gibtsnicht", "/pdx/../etc/passwd", "/pdx/x/y"} {
		w = httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, bad, nil))
		if w.Code != http.StatusNotFound {
			t.Errorf("%s: %d", bad, w.Code)
		}
	}

	// A change typesets again with the new revision.
	change(c)
	m2 := rec.wait(t, EventRenderPages, func(d any) bool { return d.(Manifest).Rev > m.Rev }).(Manifest)
	if len(m2.Pages) != 3 || m2.Compiled {
		t.Errorf("nach Änderung ohne neuen Inhalt: %+v", m2)
	}
}

func TestSnapshot(t *testing.T) {
	c := NewCore(nil, nil, t.TempDir())
	ts := NewTemplateService(c)
	if _, err := ts.Snapshot(); err == nil {
		t.Error("Snapshot ohne Vorlage gelingt")
	}
	if _, err := NewProjectService(c).Open(muster); err != nil {
		t.Fatal(err)
	}
	s, err := ts.Snapshot()
	if err != nil || s.Nodes["positionen"].Type != "table" {
		t.Fatalf("Snapshot: %v", err)
	}
	cat, err := ts.Catalog()
	if err != nil || cat["table"] == nil {
		t.Fatalf("Catalog: %v", err)
	}
}

func TestServePDF(t *testing.T) {
	c := NewCore(nil, nil, t.TempDir())
	h := NewAssets(c)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/pdx/pdf/0", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("ohne Vorlage: %d", w.Code)
	}
	if _, err := NewProjectService(c).Open(muster); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/pdx/pdf/1", nil))
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "application/pdf" || !strings.HasPrefix(w.Body.String(), "%PDF-") {
		t.Fatalf("PDF: %d %s", w.Code, w.Header().Get("Content-Type"))
	}
	// Errors during generation come as text with template ID.
	if _, err := NewTemplateService(c).Apply("Stil", model.Command{Op: "setAttr", Ref: "titel", Name: "style", Value: "st-basis"}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewTemplateService(c).Apply("Farbe", model.Command{Op: "setAttr", Ref: "titel", Name: "color", Value: "$fehlt"}); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/pdx/pdf/3", nil))
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "[titel]") {
		t.Fatalf("Fehler: %d %q", w.Code, w.Body.String())
	}
}

// fakeDialogs answers with fixed paths and remembers what was opened.
type fakeDialogs struct {
	save  string
	shown []string
}

func (f *fakeDialogs) OpenFile(string, []Filter, bool) (string, error) { return "", nil }
func (f *fakeDialogs) SaveFile(string, string, []Filter) (string, error) {
	return f.save, nil
}
func (f *fakeDialogs) Show(path string) error {
	f.shown = append(f.shown, path)
	return nil
}

func TestExportAndOpenPDF(t *testing.T) {
	dir := t.TempDir()
	fd := &fakeDialogs{save: filepath.Join(dir, "rechnung")}
	c := NewCore(nil, fd, dir)
	if _, err := NewProjectService(c).Open(muster); err != nil {
		t.Fatal(err)
	}
	rs := NewRenderService(c)
	path, err := rs.ExportPDFDialog()
	if err != nil || path != filepath.Join(dir, "rechnung.pdf") {
		t.Fatalf("Export: %q %v", path, err)
	}
	if b, _ := os.ReadFile(path); !strings.HasPrefix(string(b), "%PDF-") {
		t.Fatal("kein PDF geschrieben")
	}
	if err := rs.OpenPDF(); err != nil || len(fd.shown) != 1 || !strings.HasSuffix(fd.shown[0], "vorschau.pdf") {
		t.Fatalf("Öffnen: %v %v", err, fd.shown)
	}
	// Without dialogs (server mode) there is an understandable message.
	if err := NewRenderService(NewCore(nil, nil, dir)).OpenPDF(); !errors.Is(err, ErrNoDialogs) {
		t.Fatalf("ohne Dialoge: %v", err)
	}
}
