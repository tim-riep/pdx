package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func fuzzServer(f *testing.F) (*Server, http.Handler, []byte, []byte) {
	p, data := muster(f)
	raw, _ := p.Bytes()
	store := &MemStore{}
	store.Put(context.Background(), "rechnung", raw)
	store.Activate(context.Background(), "rechnung", "1")
	cfg := allRightsConfig()
	cfg.MaxBody = 1 << 20
	cfg.MaxTemplate = 1 << 20
	s, err := New(cfg, store, nil)
	if err != nil {
		f.Fatal(err)
	}
	return s, s.Handler(), raw, data
}

func fuzzCall(t *testing.T, s *Server, h http.Handler, target string, body []byte) {
	req := httptest.NewRequest(http.MethodPost, target, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+keyRender)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code >= 500 && rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("Status %d: %s", rec.Code, rec.Body.Bytes())
	}
	_, pattern := s.mux.Handler(req)
	for _, e := range checkResponse(pattern, rec.Code, rec.Header(), rec.Body.Bytes()) {
		t.Error(e)
	}
}

// FuzzUpload uploads arbitrary bytes as a template package.
func FuzzUpload(f *testing.F) {
	s, h, raw, _ := fuzzServer(f)
	f.Add(raw)
	f.Add([]byte("PK\x03\x04"))
	f.Fuzz(func(t *testing.T, body []byte) {
		fuzzCall(t, s, h, "/v1/templates/fuzz", body)
	})
}

// FuzzRender renders arbitrary data with the sample invoice.
func FuzzRender(f *testing.F) {
	s, h, _, data := fuzzServer(f)
	f.Add(data, "pdfa-3b")
	f.Add([]byte("<rechnung/>"), "pdfa-4f")
	f.Add([]byte("<!DOCTYPE a [<!ENTITY x SYSTEM \"file:///etc/passwd\">]><rechnung>&x;</rechnung>"), "")
	f.Fuzz(func(t *testing.T, body []byte, prof string) {
		fuzzCall(t, s, h, "/v1/render/rechnung?strict=true&profile="+url.QueryEscape(prof), body)
	})
}
