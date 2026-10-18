package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/tim-riep/pdx/template/pkg"
)

func do(t *testing.T, method, url, key string, body []byte) (*http.Response, []byte) {
	t.Helper()
	req, _ := http.NewRequest(method, url, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, b
}

// Lebenszyklus einer Vorlage über die Schnittstelle mit DirStore: hochladen,
// gleicher Inhalt ergibt dieselbe Version, aktivieren, rendern mit aktiver
// und bestimmter Version, Paket zurückholen.
func TestTemplateLifecycle(t *testing.T) {
	store := &DirStore{Dir: t.TempDir()}
	s, err := New(testConfig(), store, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(specHandler(t, s))
	defer srv.Close()
	p, data := muster(t)
	v1, _ := p.Bytes()
	f, _ := pkg.ReadDir("../testdata/templates/formular")
	v2, _ := f.Bytes()
	base := srv.URL + "/v1/templates/rechnung"

	resp, b := do(t, "POST", base+"?activate=true", keyAdmin, v1)
	var v Version
	json.Unmarshal(b, &v)
	if resp.StatusCode != 201 || v.Version != "1" || !v.Active || v.SHA256 != digest(v1) {
		t.Fatalf("erstes Hochladen: %d %s", resp.StatusCode, b)
	}
	resp, b = do(t, "POST", base, keyAdmin, v1)
	json.Unmarshal(b, &v)
	if resp.StatusCode != 200 || v.Version != "1" {
		t.Errorf("gleicher Inhalt: %d %s", resp.StatusCode, b)
	}
	resp, b = do(t, "POST", base, keyAdmin, v2)
	json.Unmarshal(b, &v)
	if resp.StatusCode != 201 || v.Version != "2" || v.Active {
		t.Errorf("zweite Version: %d %s", resp.StatusCode, b)
	}
	resp, _ = do(t, "POST", srv.URL+"/v1/render/rechnung", keyRender, data)
	if resp.StatusCode != 200 || resp.Header.Get("X-Pdx-Template-Version") != "1" {
		t.Errorf("Rendern mit aktiver Version: %d %s", resp.StatusCode, resp.Header.Get("X-Pdx-Template-Version"))
	}
	resp, b = do(t, "PUT", base+"/active", keyAdmin, []byte(`{"version":"2"}`))
	if resp.StatusCode != 200 {
		t.Fatalf("aktivieren: %d %s", resp.StatusCode, b)
	}
	fdata, _ := f.File("samples/antrag.xml")
	resp, _ = do(t, "POST", srv.URL+"/v1/render/rechnung", keyRender, fdata)
	if resp.StatusCode != 200 || resp.Header.Get("X-Pdx-Template-Version") != "2" {
		t.Errorf("nach Aktivieren: %d", resp.StatusCode)
	}
	resp, _ = do(t, "POST", srv.URL+"/v1/render/rechnung?version=1", keyRender, data)
	if resp.StatusCode != 200 || resp.Header.Get("X-Pdx-Template-Version") != "1" {
		t.Errorf("bestimmte Version: %d", resp.StatusCode)
	}
	resp, b = do(t, "GET", base+"/versions", keyAdmin, nil)
	var vl struct{ Versions []Version }
	json.Unmarshal(b, &vl)
	if resp.StatusCode != 200 || len(vl.Versions) != 2 || vl.Versions[0].Active || !vl.Versions[1].Active {
		t.Errorf("Versionen: %s", b)
	}
	resp, b = do(t, "GET", base+"/versions/1", keyAdmin, nil)
	if resp.StatusCode != 200 || !bytes.Equal(b, v1) || resp.Header.Get("Content-Type") != "application/zip" {
		t.Error("Paket nicht zurückgeholt")
	}
	resp, b = do(t, "GET", srv.URL+"/v1/templates", keyAdmin, nil)
	if resp.StatusCode != 200 || !bytes.Contains(b, []byte(`{"id":"rechnung","active":"2"}`)) {
		t.Errorf("Liste: %s", b)
	}

	for _, c := range []struct {
		name, method, url string
		body              []byte
		status            int
	}{
		{"ID", "POST", srv.URL + "/v1/templates/Böse..", v1, 400},
		{"kein ZIP", "POST", base, []byte("kein zip"), 422},
		{"Version fehlt", "PUT", base + "/active", []byte(`{"version":"7"}`), 404},
		{"Rumpf", "PUT", base + "/active", []byte(`7`), 400},
		{"unbekannt", "GET", srv.URL + "/v1/templates/nix/versions", nil, 404},
		{"Recht", "GET", srv.URL + "/v1/templates", nil, 403},
	} {
		t.Run(c.name, func(t *testing.T) {
			key := keyAdmin
			if c.name == "Recht" {
				key = keyRender
			}
			resp, b := do(t, c.method, c.url, key, c.body)
			problem(t, resp, b, c.status)
		})
	}
	resp, _ = do(t, "GET", srv.URL+"/readyz", "", nil)
	if resp.StatusCode != 200 {
		t.Error("readyz")
	}
}

// Gleichzeitiges Hochladen verschiedener Inhalte vergibt verschiedene
// Nummern, auch mit getrennten Ablagen auf demselben Verzeichnis.
func TestDirStoreConcurrent(t *testing.T) {
	dir := t.TempDir()
	var wg sync.WaitGroup
	versions := make(chan string, 20)
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			st := &DirStore{Dir: dir}
			v, created, err := st.Put(context.Background(), "x", []byte{byte(i)})
			if err != nil || !created {
				t.Error(err)
			}
			versions <- v.Version
		}()
	}
	wg.Wait()
	close(versions)
	seen := map[string]bool{}
	for v := range versions {
		if seen[v] {
			t.Errorf("Version %s doppelt vergeben", v)
		}
		seen[v] = true
	}
	if len(seen) != 20 {
		t.Errorf("%d Versionen", len(seen))
	}
}
