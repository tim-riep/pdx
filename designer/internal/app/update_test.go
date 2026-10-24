package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestIsNewer(t *testing.T) {
	for _, c := range []struct {
		latest, current string
		want            bool
	}{
		{"0.3.3", "0.3.2", true},
		{"v0.4.0", "0.3.9", true},
		{"1.0", "0.9.9", true},
		{"0.3.10", "0.3.9", true},
		{"0.3.2", "0.3.2", false},
		{"0.3.1", "0.3.2", false},
		{"0.3.2.0", "0.3.2", false},
		{"0.3.3", "0.3.2-4-gabc123-dirty", true},
		{"0.3.2", "0.3.2+build", false},
		{"", "0.3.2", false},
		{"neu", "0.3.2", false},
		{"0.3.3", "dev", false},
		{"0.-1", "0.0", false},
	} {
		if got := isNewer(c.latest, c.current); got != c.want {
			t.Errorf("isNewer(%q, %q) = %v, erwartet %v", c.latest, c.current, got, c.want)
		}
	}
}

// release starts a stand-in for GitHub that answers with tag and counts
// the requests.
func release(t *testing.T, status int, tag string) (*httptest.Server, *atomic.Int32) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		if r.Header.Get("Accept") != "application/vnd.github.html+json" || r.Header.Get("User-Agent") != "pdx-designer/"+Version {
			t.Errorf("Kopfzeilen: %v", r.Header)
		}
		w.WriteHeader(status)
		w.Write([]byte(`{"tag_name":"` + tag + `","html_url":"https://github.com/tim-riep/pdx/releases/tag/` + tag +
			`","body_html":"<p>Neu</p>","published_at":"2026-10-24T10:30:00Z"}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &n
}

func updateService(t *testing.T, url string) (*UpdateService, *fakeDialogs) {
	fd := &fakeDialogs{}
	s := NewUpdateService(NewCore(nil, fd, t.TempDir()))
	s.url = url
	return s, fd
}

func TestCheckNewer(t *testing.T) {
	srv, _ := release(t, http.StatusOK, "v99.0.0")
	s, _ := updateService(t, srv.URL)
	info := s.Check()
	if info.Error != "" || !info.Newer || info.Latest != "99.0.0" || info.Current != Version ||
		info.URL != "https://github.com/tim-riep/pdx/releases/tag/v99.0.0" || info.NotesHTML != "<p>Neu</p>" || info.PublishedAt == "" {
		t.Fatalf("Ergebnis: %+v", info)
	}
}

func TestCheckSameVersion(t *testing.T) {
	srv, _ := release(t, http.StatusOK, "v"+Version)
	s, _ := updateService(t, srv.URL)
	if info := s.Check(); info.Error != "" || info.Newer {
		t.Fatalf("Ergebnis: %+v", info)
	}
}

// A failure (rate limit, broken answer, offline) lands in Error.
func TestCheckFailures(t *testing.T) {
	srv, _ := release(t, http.StatusForbidden, "v99.0.0")
	s, _ := updateService(t, srv.URL)
	if info := s.Check(); info.Newer || !strings.Contains(info.Error, "403") {
		t.Errorf("Ratenlimit: %+v", info)
	}
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("{")) }))
	defer broken.Close()
	s.url = broken.URL
	if info := s.Check(); info.Newer || info.Error == "" {
		t.Errorf("kaputte Antwort: %+v", info)
	}
	broken.Close()
	if info := s.Check(); info.Newer || info.Error == "" {
		t.Errorf("offline: %+v", info)
	}
}

// Switched off, Check makes no request, and the choice survives a restart.
func TestCheckSwitchedOff(t *testing.T) {
	srv, n := release(t, http.StatusOK, "v99.0.0")
	s, _ := updateService(t, srv.URL)
	if !s.Settings().Check {
		t.Fatal("ohne Einstellung muss die Suche an sein")
	}
	if _, err := s.SetCheck(false); err != nil {
		t.Fatal(err)
	}
	again := NewUpdateService(s.c)
	again.url = srv.URL
	if again.Settings().Check {
		t.Fatal("Einstellung nicht gespeichert")
	}
	if info := again.Check(); info.Newer || info.Error == "" || n.Load() != 0 {
		t.Fatalf("ausgeschaltet: %+v, %d Anfragen", info, n.Load())
	}
	// An empty URL (PDX_UPDATE_URL="") switches the check off for good.
	if st, _ := (&UpdateService{c: s.c, url: ""}).SetCheck(true); st.Check {
		t.Error("leere URL muss die Suche ausschalten")
	}
}

func TestSkip(t *testing.T) {
	s, _ := updateService(t, "http://127.0.0.1:1")
	st, err := s.Skip("v99.0.0")
	if err != nil || st.Skipped != "99.0.0" || !st.Check {
		t.Fatalf("%+v %v", st, err)
	}
	if st, _ := s.SetCheck(false); st.Skipped != "99.0.0" {
		t.Errorf("Ausschalten vergisst die übersprungene Version: %+v", st)
	}
}

// Open shows only release pages of pdx, without a browser the UI takes over.
func TestOpen(t *testing.T) {
	s, fd := updateService(t, "")
	page := "https://github.com/tim-riep/pdx/releases/tag/v99.0.0"
	if err := s.Open(page); err != nil || len(fd.shown) != 1 || fd.shown[0] != page {
		t.Fatalf("%v %v", err, fd.shown)
	}
	for _, u := range []string{
		"https://example.org/",
		"https://github.com/tim-riep/pdx/releases",
		"https://github.com/tim-riep/pdx/releases/",
		"http://github.com/tim-riep/pdx/releases/tag/v1.0.0",
		"https://github.com.evil.org/tim-riep/pdx/releases/tag/v1.0.0",
		"https://user@github.com/tim-riep/pdx/releases/tag/v1.0.0",
		"https://github.com/tim-riep/pdx/releases/../../evil",
		"file:///etc/passwd",
		"javascript:alert(1)",
	} {
		if err := s.Open(u); err != ErrNotReleasePage {
			t.Errorf("%s: %v", u, err)
		}
	}
	if len(fd.shown) != 1 {
		t.Errorf("geöffnet: %v", fd.shown)
	}
	if err := NewUpdateService(NewCore(nil, nil, t.TempDir())).Open(page); err != ErrNoDialogs {
		t.Errorf("ohne Browser: %v", err)
	}
}
