package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

	// Eine Änderung setzt neu mit der neuen Revision.
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
	// Fehler beim Erzeugen kommen als Text mit Vorlagen-ID.
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
