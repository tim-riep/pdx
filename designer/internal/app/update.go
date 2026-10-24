package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// releasesURL is GitHub's "latest release" endpoint for pdx. It excludes
// drafts and pre-releases.
const releasesURL = "https://api.github.com/repos/tim-riep/pdx/releases/latest"

// releasePages is the path below which Open shows pages on github.com.
const releasePages = "/tim-riep/pdx/releases/"

// UpdateService asks GitHub whether a newer release exists. The request
// runs only while the check is switched on (the default) and contains
// nothing but the version in the User-Agent. The UI decides when to call
// Check.
type UpdateService struct {
	c      *Core
	url    string
	client *http.Client
}

// NewUpdateService returns the service. PDX_UPDATE_URL replaces the GitHub
// endpoint, for example with a stand-in in the end-to-end tests. Set but
// empty, it switches the check off entirely.
func NewUpdateService(c *Core) *UpdateService {
	endpoint := releasesURL
	if v, ok := os.LookupEnv("PDX_UPDATE_URL"); ok {
		endpoint = v
	}
	return &UpdateService{c: c, url: endpoint, client: &http.Client{Timeout: 10 * time.Second}}
}

// UpdateInfo is the outcome of Check.
type UpdateInfo struct {
	// Current is the version of this build.
	Current string `json:"current"`
	// Latest is the version of the newest release, without a leading "v".
	Latest string `json:"latest"`
	// URL is the page of the newest release.
	URL string `json:"url"`
	// Newer reports that Latest is newer than Current.
	Newer bool `json:"newer"`
	// NotesHTML holds the notes of the newest release, rendered and
	// sanitized by GitHub. The UI cleans them once more before showing them.
	NotesHTML string `json:"notesHtml"`
	// PublishedAt is when the newest release was published (RFC 3339).
	PublishedAt string `json:"publishedAt"`
	// Error says why the check failed, empty on success.
	Error string `json:"error,omitempty"`
}

// UpdateSettings are the user's choices, stored in update.json in the data
// directory.
type UpdateSettings struct {
	// Check switches the check at start and every few hours on or off.
	Check bool `json:"check"`
	// Skipped is the release whose dialog no longer opens by itself.
	Skipped string `json:"skipped,omitempty"`
}

func (s *UpdateService) settingsPath() string { return filepath.Join(s.c.dataDir, "update.json") }

// Settings returns the stored choices. Without a file the check is on.
func (s *UpdateService) Settings() UpdateSettings {
	st := UpdateSettings{Check: true}
	if b, err := os.ReadFile(s.settingsPath()); err == nil {
		json.Unmarshal(b, &st)
	}
	if s.url == "" {
		st.Check = false
	}
	return st
}

func (s *UpdateService) save(st UpdateSettings) (UpdateSettings, error) {
	b, _ := json.MarshalIndent(st, "", "  ")
	os.MkdirAll(s.c.dataDir, 0o755)
	if err := os.WriteFile(s.settingsPath(), b, 0o600); err != nil {
		return s.Settings(), err
	}
	return s.Settings(), nil
}

// SetCheck switches the check on or off.
func (s *UpdateService) SetCheck(on bool) (UpdateSettings, error) {
	st := s.Settings()
	st.Check = on
	return s.save(st)
}

// Skip remembers a release whose dialog should no longer open by itself.
// The button in the toolbar still shows it.
func (s *UpdateService) Skip(version string) (UpdateSettings, error) {
	st := s.Settings()
	st.Skipped = strings.TrimPrefix(strings.TrimSpace(version), "v")
	return s.save(st)
}

// Check fetches the newest release from GitHub and compares it with this
// build. Failures (offline, rate limit) end up in Error and never as a Go
// error, so the UI can ignore them quietly. With the check switched off no
// request is made.
func (s *UpdateService) Check() UpdateInfo {
	info := UpdateInfo{Current: Version}
	if !s.Settings().Check {
		info.Error = "Suche nach neuen Versionen ist ausgeschaltet"
		return info
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
	if err != nil {
		info.Error = err.Error()
		return info
	}
	// The html media type adds body_html: the notes rendered and sanitized
	// by GitHub, so the app needs no Markdown renderer of its own.
	req.Header.Set("Accept", "application/vnd.github.html+json")
	req.Header.Set("User-Agent", "pdx-designer/"+Version)
	resp, err := s.client.Do(req)
	if err != nil {
		info.Error = err.Error()
		return info
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		info.Error = fmt.Sprintf("GitHub antwortete mit %s", resp.Status)
		return info
	}
	var rel struct {
		TagName     string `json:"tag_name"`
		HTMLURL     string `json:"html_url"`
		BodyHTML    string `json:"body_html"`
		PublishedAt string `json:"published_at"`
	}
	// A release answer is a few kilobytes, more is not one.
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rel); err != nil {
		info.Error = err.Error()
		return info
	}
	info.Latest = strings.TrimPrefix(rel.TagName, "v")
	info.URL = rel.HTMLURL
	info.Newer = isNewer(info.Latest, Version)
	info.NotesHTML = rel.BodyHTML
	info.PublishedAt = rel.PublishedAt
	return info
}

// ErrNotReleasePage reports a URL outside the release pages of pdx.
var ErrNotReleasePage = errors.New("nur Release-Seiten von pdx lassen sich öffnen")

// Open shows a release page of pdx in the system's browser. Other addresses
// are refused, so the UI cannot be used to open arbitrary links. Without
// a browser (server build) it returns ErrNoDialogs and the UI opens the page
// itself.
func (s *UpdateService) Open(page string) error {
	u, err := url.Parse(page)
	if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil ||
		!strings.HasPrefix(path.Clean(u.Path)+"/", releasePages) || path.Clean(u.Path)+"/" == releasePages {
		return ErrNotReleasePage
	}
	if s.c.dialogs == nil {
		return ErrNoDialogs
	}
	return s.c.dialogs.OpenURL(u.String())
}

// isNewer reports whether the release version latest is newer than current.
// Both are dotted numbers, optionally with a "v" prefix and a suffix after
// "-" or "+", which counts as its base version. A version that does not
// parse is never newer.
func isNewer(latest, current string) bool {
	l, ok1 := parseVersion(latest)
	c, ok2 := parseVersion(current)
	if !ok1 || !ok2 {
		return false
	}
	for i := 0; i < len(l) || i < len(c); i++ {
		var a, b int
		if i < len(l) {
			a = l[i]
		}
		if i < len(c) {
			b = c[i]
		}
		if a != b {
			return a > b
		}
	}
	return false
}

func parseVersion(v string) ([]int, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	if v == "" {
		return nil, false
	}
	var out []int
	for _, part := range strings.Split(v, ".") {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}
