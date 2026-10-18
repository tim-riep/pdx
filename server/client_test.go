package server

import (
	"context"
	"errors"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestClient(t *testing.T) {
	s, _ := New(allRightsConfig(), &MemStore{}, nil)
	srv := httptest.NewServer(specHandler(t, s))
	defer srv.Close()
	ctx := context.Background()
	c, err := NewClient(srv.URL, keyRender)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Versions(ctx, "rechnung"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Versions ohne Vorlage: %v", err)
	}
	p, data := muster(t)
	raw, _ := p.Bytes()
	v, err := c.Upload(ctx, "rechnung", raw, false)
	if err != nil || v.Version != "1" || v.Active {
		t.Fatalf("Upload: %+v %v", v, err)
	}
	if err := c.Activate(ctx, "rechnung", "1"); err != nil {
		t.Fatal(err)
	}
	vs, _ := c.Versions(ctx, "rechnung")
	if len(vs) != 1 || !vs[0].Active {
		t.Errorf("Versionen %+v", vs)
	}
	got, _ := c.Download(ctx, "rechnung", "1")
	if string(got) != string(raw) {
		t.Error("Download weicht ab")
	}
	pdf, err := c.Render(ctx, "rechnung", data, url.Values{"profile": {"pdfa-3b"}})
	if err != nil || !strings.HasPrefix(string(pdf), "%PDF-") {
		t.Errorf("Render: %v", err)
	}
	_, err = c.Upload(ctx, "rechnung", []byte("kein zip"), false)
	var pe *ProblemError
	if !errors.As(err, &pe) || pe.Problem.Status != 422 || len(pe.Problem.Findings) == 0 {
		t.Errorf("Problem: %v", err)
	}
	bad, _ := NewClient(srv.URL, "falsch")
	if _, err := bad.Templates(ctx); err == nil || strings.Contains(err.Error(), "falsch") {
		t.Errorf("falscher Schlüssel: %v", err)
	}
	if _, err := NewClient("http://example.org", "k"); err == nil {
		t.Error("HTTP ohne TLS erlaubt")
	}
	if c.String() != "Client("+srv.URL+")" {
		t.Error("String")
	}
}
