package server

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tim-riep/pdx/internal/testpki"
	"github.com/tim-riep/pdx/internal/testutil"
	"github.com/tim-riep/pdx/sign"
	"github.com/tim-riep/pdx/verapdf"
)

func allRightsConfig() *Config {
	return &Config{Keys: []APIKey{{Name: "alles", SHA256: HashKey(keyRender), Rights: allRights}}}
}

func zipOf(t *testing.T, files map[string][]byte, order []string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, n := range order {
		fw, _ := zw.Create(n)
		fw.Write(files[n])
	}
	zw.Close()
	return buf.Bytes()
}

// Ein Stapel liefert die PDFs in Eingabereihenfolge, fehlerhafte
// Datensätze mit Befunden in ergebnis.json.
func TestBatch(t *testing.T) {
	cfg := allRightsConfig()
	cfg.MaxConcurrent = 2
	srv, data := newTest(t, cfg, nil)
	order := []string{"a.xml", "daten/b.xml", "kaputt.xml", "c.xml", "liesmich.txt"}
	in := zipOf(t, map[string][]byte{"a.xml": data, "daten/b.xml": data, "kaputt.xml": []byte("<rechnung"), "c.xml": data, "liesmich.txt": []byte("x")}, order)
	resp, b := post(t, srv.URL+"/v1/render/rechnung/batch?profile=pdfa-3b", keyRender, in)
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "application/zip" {
		t.Fatalf("Status %d: %.300s", resp.StatusCode, b)
	}
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	if strings.Join(names, ",") != "a.pdf,b.pdf,c.pdf,ergebnis.json" {
		t.Errorf("Einträge %v", names)
	}
	rc, _ := zr.File[len(zr.File)-1].Open()
	raw, _ := io.ReadAll(rc)
	var res struct {
		Version   string
		Dokumente []batchResult
	}
	json.Unmarshal(raw, &res)
	if len(res.Dokumente) != 4 || res.Dokumente[2].Data != "kaputt.xml" || len(res.Dokumente[2].Findings) == 0 || res.Dokumente[0].Pages != 3 || res.Version != "1" {
		t.Errorf("ergebnis.json: %s", raw)
	}
	pdf, _ := zr.File[0].Open()
	head := make([]byte, 5)
	pdf.Read(head)
	if string(head) != "%PDF-" {
		t.Error("kein PDF im Stapel")
	}
}

func TestBatchLimits(t *testing.T) {
	cfg := allRightsConfig()
	cfg.MaxBatchEntries = 2
	cfg.MaxBody = 1000
	srv, _ := newTest(t, cfg, nil)
	u := srv.URL + "/v1/render/rechnung/batch"
	resp, b := post(t, u, keyRender, []byte("kein zip"))
	problem(t, resp, b, 400)
	big := bytes.Repeat([]byte("<a/>"), 400)
	in := zipOf(t, map[string][]byte{"a.xml": big, "b.xml": []byte("<a/>"), "c.xml": []byte("<a/>")}, []string{"a.xml", "b.xml", "c.xml"})
	resp, b = post(t, u, keyRender, in)
	p := problem(t, resp, b, 422)
	if len(p.Findings) != 2 {
		t.Errorf("Befunde %v, verlangt Größe und Anzahl auf einmal", p.Findings)
	}
	resp, b = post(t, u, keyRender, zipOf(t, map[string][]byte{"x.txt": nil}, []string{"x.txt"}))
	problem(t, resp, b, 422)
}

type fakeVerifier struct{ fail bool }

func (fakeVerifier) Name() string { return "Test" }
func (f fakeVerifier) Validate(_ context.Context, file, flavour string) (*verapdf.Report, error) {
	if _, err := os.Stat(file); err != nil {
		return nil, err
	}
	r := &verapdf.Report{Profile: "PDF/A-" + flavour, Compliant: !f.fail}
	if f.fail {
		r.Findings = []verapdf.Finding{{Specification: "ISO 19005-3:2012", Clause: "6.2.11.4.1", TestNumber: "1", Description: "Schrift nicht eingebettet", FailedChecks: 2}}
		r.FailedRules = 1
	}
	return r, nil
}

func TestVerify(t *testing.T) {
	p, data := muster(t)
	store := &MemStore{}
	raw, _ := p.Bytes()
	store.Put(context.Background(), "rechnung", raw)
	store.Activate(context.Background(), "rechnung", "1")
	s, _ := New(allRightsConfig(), store, nil)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	_ = data
	resp, b := post(t, srv.URL+"/v1/verify", keyRender, []byte("%PDF-1.7"))
	problem(t, resp, b, 501)
	s.Verifier = fakeVerifier{}
	resp, b = post(t, srv.URL+"/v1/verify?profile=pdfa-3a,pdfua-1", keyRender, []byte("%PDF-1.7"))
	var out struct {
		Konform  bool
		Berichte []verifyReport
	}
	json.Unmarshal(b, &out)
	if resp.StatusCode != 200 || !out.Konform || len(out.Berichte) != 2 {
		t.Errorf("verify: %d %s", resp.StatusCode, b)
	}
	s.Verifier = fakeVerifier{fail: true}
	_, b = post(t, srv.URL+"/v1/verify?profile=pdfa-3b", keyRender, []byte("%PDF-1.7"))
	json.Unmarshal(b, &out)
	if out.Konform || len(out.Berichte[0].Findings) != 1 || out.Berichte[0].Findings[0].Clause != "6.2.11.4.1" {
		t.Errorf("Befund fehlt: %s", b)
	}
	resp, b = post(t, srv.URL+"/v1/verify?profile=quatsch", keyRender, nil)
	problem(t, resp, b, 400)
}

// /v1/sign signiert mit dem Signer des Dienstes, Fehler wie ein
// abgelaufenes Zertifikat kommen als Befund.
func TestSignEndpoint(t *testing.T) {
	pki := testpki.New(t)
	p, data := muster(t)
	store := &MemStore{}
	raw, _ := p.Bytes()
	store.Put(context.Background(), "rechnung", raw)
	store.Activate(context.Background(), "rechnung", "1")
	s, _ := New(allRightsConfig(), store, nil)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	_, pdf := post(t, srv.URL+"/v1/render/rechnung", keyRender, data)
	resp, b := post(t, srv.URL+"/v1/sign", keyRender, pdf)
	problem(t, resp, b, 501)
	s.Signer = pki.KeySigner(t, pki.Signer)
	s.TSA = &sign.TSA{URL: pki.TSAURL()}
	resp, b = post(t, srv.URL+"/v1/sign?level=B-T&reason=Freigabe", keyRender, pdf)
	if resp.StatusCode != 200 || !bytes.HasPrefix(b, pdf) || !bytes.Contains(b[len(pdf):], []byte("/SubFilter /ETSI.CAdES.detached")) {
		t.Fatalf("Signieren: %d %.300s", resp.StatusCode, b)
	}
	s.Signer = pki.KeySigner(t, pki.Expired)
	resp, b = post(t, srv.URL+"/v1/sign", keyRender, pdf)
	pr := problem(t, resp, b, 422)
	if !strings.Contains(pr.Findings[0].Message, "abgelaufen") {
		t.Errorf("Befund %v", pr.Findings)
	}
}

// /metrics zählt je Route und Status, ohne IDs oder Nutzdaten.
func TestMetrics(t *testing.T) {
	srv, data := newTest(t, testConfig(), nil)
	post(t, srv.URL+"/v1/render/rechnung", keyRender, data)
	post(t, srv.URL+"/v1/render/gibtsnicht", keyRender, data)
	time.Sleep(10 * time.Millisecond)
	resp, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	m := string(b)
	for _, want := range []string{
		`pdx_requests_total{route="POST /v1/render/{id}",status="200"} 1`,
		`pdx_requests_total{route="POST /v1/render/{id}",status="404"} 1`,
		`pdx_request_duration_seconds_count{route="POST /v1/render/{id}"} 2`,
		"pdx_render_slots 8", "pdx_template_cache_entries 1", "# TYPE pdx_in_flight gauge",
	} {
		if !strings.Contains(m, want) {
			t.Errorf("%q fehlt in\n%s", want, m)
		}
	}
	if strings.Contains(m, "gibtsnicht") || strings.Contains(m, "rechnung") {
		t.Error("IDs in den Metriken")
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/plain; version=0.0.4") {
		t.Error("Content-Type")
	}
}

// Mit PDX_CONFORMANCE=1 prüft /v1/verify ein gerendertes PDF mit dem
// echten veraPDF.
func TestVerifyVeraPDF(t *testing.T) {
	if !testutil.ConformanceEnabled() {
		t.Skip("nur mit PDX_CONFORMANCE=1")
	}
	r, err := verapdf.Detect(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	cfg := allRightsConfig()
	cfg.Timeout = Duration{2 * time.Minute}
	p, data := muster(t)
	store := &MemStore{}
	raw, _ := p.Bytes()
	store.Put(context.Background(), "rechnung", raw)
	store.Activate(context.Background(), "rechnung", "1")
	s, _ := New(cfg, store, nil)
	s.Verifier = r
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	_, pdf := post(t, srv.URL+"/v1/render/rechnung?profile=pdfa-3b", keyRender, data)
	resp, b := post(t, srv.URL+"/v1/verify?profile=pdfa-3b", keyRender, pdf)
	if resp.StatusCode != 200 || !bytes.Contains(b, []byte(`"konform":true`)) {
		t.Errorf("veraPDF: %d %s", resp.StatusCode, b)
	}
}
