package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tim-riep/pdx"
	"github.com/tim-riep/pdx/internal/testutil"
	"github.com/tim-riep/pdx/pdf/profile"
	"github.com/tim-riep/pdx/template/pkg"
	"github.com/tim-riep/pdx/verapdf"
)

const (
	keyRender = "render-schluessel-1"
	keyAdmin  = "admin-schluessel-2"
)

func testConfig() *Config {
	return &Config{Keys: []APIKey{
		{Name: "anwendung", SHA256: HashKey(keyRender), Rights: []string{RightRender}},
		{Name: "admin", SHA256: HashKey(keyAdmin), Rights: []string{RightTemplates}},
	}}
}

func muster(t testing.TB) (*pkg.Package, []byte) {
	t.Helper()
	p, err := pkg.ReadDir("../testdata/templates/musterrechnung")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := p.File("samples/rechnung.xml")
	return p, data
}

// newTest startet den Dienst mit der Musterrechnung als Vorlage rechnung.
func newTest(t *testing.T, cfg *Config, logw io.Writer) (*httptest.Server, []byte) {
	t.Helper()
	p, data := muster(t)
	store := &MemStore{}
	raw, _ := p.Bytes()
	v, _, err := store.Put(context.Background(), "rechnung", raw)
	if err != nil {
		t.Fatal(err)
	}
	store.Activate(context.Background(), "rechnung", v.Version)
	var log *slog.Logger
	if logw != nil {
		log = slog.New(slog.NewJSONHandler(logw, nil))
	}
	s, err := New(cfg, store, log)
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC) }
	srv := httptest.NewServer(specHandler(t, s))
	t.Cleanup(srv.Close)
	return srv, data
}

func post(t *testing.T, url, key string, body []byte) (*http.Response, []byte) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	req.Header.Set("Content-Type", "application/xml")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, b
}

func problem(t *testing.T, resp *http.Response, b []byte, status int) Problem {
	t.Helper()
	if resp.StatusCode != status {
		t.Fatalf("Status %d statt %d: %s", resp.StatusCode, status, b)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/problem+json" {
		t.Fatalf("Content-Type %q", ct)
	}
	var p Problem
	if err := json.Unmarshal(b, &p); err != nil || p.Status != status || p.Title == "" {
		t.Fatalf("kein Problem nach RFC 9457: %s", b)
	}
	return p
}

// Die Musterrechnung wird in mehreren Profilen gerendert, gleiche
// Eingabe mit gleichem creationDate ergibt gleiche Bytes.
func TestRender(t *testing.T) {
	var logs bytes.Buffer
	srv, data := newTest(t, testConfig(), &logs)
	for _, prof := range []string{"", "pdfa-3b", "pdfa-4f", "pdf-2.0"} {
		resp, b := post(t, srv.URL+"/v1/render/rechnung?profile="+prof, keyRender, data)
		if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "application/pdf" || !bytes.HasPrefix(b, []byte("%PDF-")) {
			t.Fatalf("%s: Status %d: %.300s", prof, resp.StatusCode, b)
		}
		if resp.Header.Get("X-Pdx-Template-Version") != "1" || resp.Header.Get("Cache-Control") != "no-store" {
			t.Error("Kopfzeilen fehlen")
		}
	}
	u := srv.URL + "/v1/render/rechnung?creationDate=2026-01-01T00:00:00Z"
	_, a := post(t, u, keyRender, data)
	_, b := post(t, u, keyRender, data)
	if !bytes.Equal(a, b) {
		t.Error("gleiche Eingabe ergibt andere Bytes")
	}
	// Keine Nutzdaten im Log: weder Daten noch Schlüssel noch Abfrage.
	for _, secret := range []string{"Musterstraße", keyRender, "creationDate", "rechnung?"} {
		if strings.Contains(logs.String(), secret) {
			t.Errorf("Log enthält %q", secret)
		}
	}
	if !strings.Contains(logs.String(), `"route":"POST /v1/render/{id}"`) || !strings.Contains(logs.String(), `"schluessel":"anwendung"`) {
		t.Errorf("Log ohne Route oder Schlüsselname: %s", logs.String())
	}
}

func TestAuth(t *testing.T) {
	srv, data := newTest(t, testConfig(), nil)
	u := srv.URL + "/v1/render/rechnung"
	resp, b := post(t, u, "", data)
	problem(t, resp, b, http.StatusUnauthorized)
	if resp.Header.Get("WWW-Authenticate") == "" {
		t.Error("WWW-Authenticate fehlt")
	}
	resp, b = post(t, u, "falsch", data)
	problem(t, resp, b, http.StatusUnauthorized)
	resp, b = post(t, u, keyAdmin, data)
	problem(t, resp, b, http.StatusForbidden)
	req, _ := http.NewRequest(http.MethodPost, u, bytes.NewReader(data))
	req.Header.Set("X-API-Key", keyRender)
	r2, _ := http.DefaultClient.Do(req)
	r2.Body.Close()
	if r2.StatusCode != http.StatusOK {
		t.Errorf("X-API-Key: Status %d", r2.StatusCode)
	}
	r3, _ := http.Get(srv.URL + "/healthz")
	r3.Body.Close()
	if r3.StatusCode != http.StatusOK {
		t.Error("healthz verlangt keine Anmeldung")
	}
}

// Fehler kommen als Problem mit allen Befunden.
func TestRenderErrors(t *testing.T) {
	cfg := testConfig()
	cfg.MaxBody = 64 << 10
	srv, data := newTest(t, cfg, nil)
	cases := []struct {
		name, path string
		body       []byte
		status     int
		finding    string
	}{
		{"unbekannte Vorlage", "/v1/render/gibtsnicht", data, 404, ""},
		{"unbekannte Version", "/v1/render/rechnung?version=9", data, 404, ""},
		{"Parameter", "/v1/render/rechnung?profile=pdfa-9z&creationDate=gestern", data, 400, "creationDate"},
		{"zu groß", "/v1/render/rechnung", bytes.Repeat([]byte("x"), 65<<10), 413, ""},
		{"kaputte Daten", "/v1/render/rechnung", []byte("<rechnung"), 422, ""},
		{"strenge Prüfung", "/v1/render/rechnung?strict=true", []byte("<rechnung/>"), 422, "rechnung"},
		{"Pfad", "/v2/irgendwas", nil, 404, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp, b := post(t, srv.URL+c.path, keyRender, c.body)
			p := problem(t, resp, b, c.status)
			if c.finding != "" {
				all, _ := json.Marshal(p.Findings)
				if !strings.Contains(string(all), c.finding) {
					t.Errorf("Befunde ohne %q: %s", c.finding, all)
				}
			}
			if c.name == "Parameter" && len(p.Findings) != 2 {
				t.Errorf("%d Befunde, verlangt alle auf einmal", len(p.Findings))
			}
		})
	}
}

// Ist jeder Platz belegt, antwortet der Dienst nach QueueWait mit 503.
func TestOverload(t *testing.T) {
	cfg := testConfig()
	cfg.MaxConcurrent = 1
	cfg.QueueWait = Duration{10 * time.Millisecond}
	srv, data := newTest(t, cfg, nil)
	var wg sync.WaitGroup
	codes := make(chan int, 6)
	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, _ := post(t, srv.URL+"/v1/render/rechnung", keyRender, data)
			codes <- resp.StatusCode
			if resp.StatusCode == 503 && resp.Header.Get("Retry-After") == "" {
				t.Error("Retry-After fehlt")
			}
		}()
	}
	wg.Wait()
	close(codes)
	seen := map[int]int{}
	for c := range codes {
		seen[c]++
	}
	if seen[200] == 0 || seen[503] == 0 {
		t.Errorf("Statuscodes %v, erwartet 200 und 503", seen)
	}
}

func TestConfigCheck(t *testing.T) {
	c := &Config{Keys: []APIKey{
		{Name: "a", SHA256: "kein-hash", Rights: []string{"render"}},
		{Name: "a", SHA256: HashKey("x"), Rights: []string{"alles"}},
		{Name: "b", SHA256: HashKey("y")},
		{Name: "c", SHA256: HashKey("y"), Rights: []string{"render"}},
	}}
	err := c.Check()
	if err == nil {
		t.Fatal("Fehler nicht erkannt")
	}
	for _, want := range []string{"kein SHA-256", "doppelt", "unbekannt", "keine Rechte", "denselben Hash"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Befund %q fehlt in %v", want, err)
		}
	}
}

// Der Dienst rendert jedes Profil mit denselben Bytes wie die Bibliothek.
// Profile, die die Musterrechnung nicht erfüllen kann (Datenanhang unter
// PDF/A-2), lehnen beide mit Befund ab. Mit PDX_CONFORMANCE=1 prüft
// veraPDF jedes Ergebnis.
func TestRenderAllProfiles(t *testing.T) {
	srv, data := newTest(t, testConfig(), nil)
	p, _ := muster(t)
	date := time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC)
	for _, prof := range append(profile.Names(), "pdfa-3a,pdfua-1", "pdfa-4f,pdfua-2") {
		t.Run(prof, func(t *testing.T) {
			want, wantErr := pdx.RenderTemplate(p, data, pdx.TemplateOptions{Options: pdx.Options{Profile: prof, CreationDate: date}})
			resp, b := post(t, srv.URL+"/v1/render/rechnung?creationDate=2026-01-15T09:00:00Z&profile="+url.QueryEscape(prof), keyRender, data)
			if wantErr != nil {
				pr := problem(t, resp, b, http.StatusUnprocessableEntity)
				if len(pr.Findings) == 0 {
					t.Error("Ablehnung ohne Befund")
				}
				return
			}
			if resp.StatusCode != http.StatusOK || !bytes.Equal(b, want.PDF) {
				t.Fatalf("Status %d, gleiche Bytes: %v", resp.StatusCode, bytes.Equal(b, want.PDF))
			}
			if _, err := verapdf.Flavours(prof); err == nil {
				testutil.Conform(t, "dienst-"+prof+".pdf", b, prof)
			}
		})
	}
}
