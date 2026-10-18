package app

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tim-riep/pdx/server"
)

const publishKey = "designer-test-schluessel"

func publishServer(t *testing.T) *httptest.Server {
	cfg := &server.Config{Keys: []server.APIKey{{Name: "designer", SHA256: server.HashKey(publishKey), Rights: []string{server.RightTemplates}}}}
	s, err := server.New(cfg, &server.MemStore{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return srv
}

// Veröffentlichen: verbinden, vergleichen gegen eine neue Vorlage, hochladen
// mit Aktivieren, gleicher Inhalt bleibt dieselbe Version, eine Änderung
// zeigt sich im Vergleich und wird Version 2. Der Schlüssel landet nicht
// auf der Platte.
func TestPublish(t *testing.T) {
	srv := publishServer(t)
	dir := t.TempDir()
	c := NewCore(nil, nil, dir)
	if _, err := NewProjectService(c).Open(muster); err != nil {
		t.Fatal(err)
	}
	ps := NewPublishService(c)
	if st := ps.State(); st.Connected || st.ID != "musterrechnung" {
		t.Errorf("Anfang: %+v", st)
	}
	if _, err := ps.Compare("musterrechnung"); err == nil {
		t.Error("Vergleich ohne Verbindung")
	}
	if _, err := ps.Connect(srv.URL, "falsch"); err == nil {
		t.Error("falscher Schlüssel angenommen")
	}
	st, err := ps.Connect(srv.URL, publishKey)
	if err != nil || !st.Connected || st.URL != srv.URL {
		t.Fatalf("Connect: %+v %v", st, err)
	}
	d, err := ps.Compare("musterrechnung")
	if err != nil || d.Active != "" || d.Same || len(d.Files) == 0 || d.Files[0].Kind != "added" {
		t.Fatalf("Vergleich neu: %+v %v", d, err)
	}
	r, err := ps.Publish("musterrechnung", true)
	if err != nil || r.Version != "1" || !r.Created || !r.Active {
		t.Fatalf("Publish: %+v %v", r, err)
	}
	if d, _ := ps.Compare("musterrechnung"); !d.Same || d.Active != "1" || len(d.Template) != 0 {
		t.Errorf("nach Publish nicht gleich: %+v", d)
	}
	if r, _ := ps.Publish("musterrechnung", false); r.Created || r.Version != "1" {
		t.Errorf("gleicher Inhalt: %+v", r)
	}
	c.mu.Lock()
	c.doc.Pkg.Template.ByID("erste").SetAttr("margin", "20mm")
	c.mu.Unlock()
	d, err = ps.Compare("musterrechnung")
	if err != nil || d.Same || len(d.Files) != 1 || d.Files[0] != (FileChange{"template.xml", "changed"}) {
		t.Fatalf("Vergleich geändert: %+v %v", d.Files, err)
	}
	var plus, minus int
	for _, l := range d.Template {
		switch l.Op {
		case "+":
			plus++
			if !strings.Contains(l.Text, `margin="20mm"`) {
				t.Errorf("neue Zeile %q", l.Text)
			}
		case "-":
			minus++
		}
	}
	if plus != 1 || minus != 1 {
		t.Errorf("Diff mit %d neuen und %d entfernten Zeilen", plus, minus)
	}
	r, err = ps.Publish("musterrechnung", false)
	if err != nil || r.Version != "2" || r.Active {
		t.Fatalf("zweite Version: %+v %v", r, err)
	}
	if err := ps.Activate("musterrechnung", "2"); err != nil {
		t.Fatal(err)
	}
	if d, _ := ps.Compare("musterrechnung"); !d.Same || d.Active != "2" || d.Versions != 2 {
		t.Errorf("nach Aktivieren: %+v", d)
	}
	if _, err := ps.Compare("Böse ID"); err == nil {
		t.Error("ungültige ID angenommen")
	}
	// Nur die Adresse ist gespeichert, nie der Schlüssel.
	b, _ := os.ReadFile(filepath.Join(dir, "publish.json"))
	if !strings.Contains(string(b), srv.URL) {
		t.Error("Adresse nicht gespeichert")
	}
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if info != nil && !info.IsDir() {
			if b, _ := os.ReadFile(p); strings.Contains(string(b), publishKey) {
				t.Errorf("Schlüssel in %s", p)
			}
		}
		return nil
	})
	if st := ps.Disconnect(); st.Connected || st.URL != srv.URL {
		t.Errorf("Disconnect: %+v", st)
	}
	if _, err := ps.Publish("musterrechnung", false); err == nil {
		t.Error("Publish nach Disconnect")
	}
}

// Eine fehlerhafte Vorlage wird mit Vorlagen-ID abgelehnt, noch bevor sie
// den Server erreicht.
func TestPublishRejected(t *testing.T) {
	srv := publishServer(t)
	c := NewCore(nil, nil, t.TempDir())
	NewProjectService(c).Open(muster)
	ps := NewPublishService(c)
	ps.Connect(srv.URL, publishKey)
	c.mu.Lock()
	c.doc.Pkg.Template.ByID("erste").SetAttr("margin", "viel")
	c.mu.Unlock()
	_, err := ps.Publish("musterrechnung", true)
	if err == nil || !strings.Contains(err.Error(), "[erste]") {
		t.Errorf("Ablehnung: %v", err)
	}
}

func TestLineDiff(t *testing.T) {
	d := lineDiff("a\nb\nc\n", "a\nx\nc\nd\n")
	var got []string
	for _, l := range d {
		got = append(got, l.Op+l.Text)
	}
	if strings.Join(got, ",") != " a,-b,+x, c,+d" {
		t.Errorf("Diff %v", got)
	}
	if lineDiff("gleich", "gleich") != nil {
		t.Error("gleiche Texte")
	}
}
