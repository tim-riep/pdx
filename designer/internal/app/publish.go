package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tim-riep/pdx/server"
	"github.com/tim-riep/pdx/template"
	"github.com/tim-riep/pdx/template/pkg"
)

// PublishService publishes the template on a pdx-server: comparison
// with the active version, upload as a new version, activation. The
// API key stays only in the memory of this session, only the address of
// the server is stored.
type PublishService struct {
	c      *Core
	mu     sync.Mutex
	client *server.Client
}

// NewPublishService returns the service.
func NewPublishService(c *Core) *PublishService { return &PublishService{c: c} }

// PublishState is the state of the connection for the UI.
type PublishState struct {
	URL       string `json:"url"`
	Connected bool   `json:"connected"`
	// ID is the suggestion for the template ID on the server.
	ID string `json:"id"`
}

// FileChange is a file that changes compared to the active version.
type FileChange struct {
	Name string `json:"name"`
	// Kind is added, removed or changed.
	Kind string `json:"kind"`
}

// DiffLine is a line in the comparison of template.xml.
type DiffLine struct {
	// Op is " " (equal), "+" (new) or "-" (removed).
	Op   string `json:"op"`
	Text string `json:"text"`
}

// PublishDiff compares the template with the active version on the server.
type PublishDiff struct {
	ID string `json:"id"`
	// Active is the active version, empty if the template does not exist
	// yet or no version is active.
	Active   string       `json:"active"`
	Versions int          `json:"versions"`
	Same     bool         `json:"same"`
	Files    []FileChange `json:"files"`
	Template []DiffLine   `json:"template"`
}

// PublishResult is the result of the upload.
type PublishResult struct {
	Version string `json:"version"`
	Created bool   `json:"created"`
	Active  bool   `json:"active"`
}

type publishSettings struct {
	URL string `json:"url"`
}

func (s *PublishService) settingsPath() string { return filepath.Join(s.c.dataDir, "publish.json") }

// State returns the address, the connection and the suggestion for the ID.
func (s *PublishService) State() PublishState {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := PublishState{Connected: s.client != nil, ID: s.suggestID()}
	if s.client != nil {
		st.URL = s.client.URL
	} else {
		var ps publishSettings
		if b, err := os.ReadFile(s.settingsPath()); err == nil {
			json.Unmarshal(b, &ps)
		}
		st.URL = ps.URL
	}
	return st
}

var reNonID = regexp.MustCompile(`[^a-z0-9_-]+`)

// suggestID derives the ID from the ID of the template or the file name.
func (s *PublishService) suggestID() string {
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	if s.c.doc == nil {
		return ""
	}
	id := s.c.doc.Pkg.Template.Root.ID()
	if s.c.doc.Path != "" {
		id = strings.TrimSuffix(filepath.Base(s.c.doc.Path), ".zip")
	}
	id = strings.Trim(reNonID.ReplaceAllString(strings.ToLower(id), "-"), "-_")
	if len(id) > 64 {
		id = id[:64]
	}
	return id
}

// Connect checks address and key with a query of the templates. The
// key stays only in memory.
func (s *PublishService) Connect(url, key string) (PublishState, error) {
	c, err := server.NewClient(url, key)
	if err != nil {
		return PublishState{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := c.Templates(ctx); err != nil {
		return PublishState{}, err
	}
	s.mu.Lock()
	s.client = c
	s.mu.Unlock()
	b, _ := json.MarshalIndent(publishSettings{URL: c.URL}, "", "  ")
	os.MkdirAll(s.c.dataDir, 0o755)
	os.WriteFile(s.settingsPath(), b, 0o600)
	return s.State(), nil
}

// Disconnect forgets the key.
func (s *PublishService) Disconnect() PublishState {
	s.mu.Lock()
	s.client = nil
	s.mu.Unlock()
	return s.State()
}

func (s *PublishService) connected() (*server.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client == nil {
		return nil, errors.New("nicht mit einem pdx-server verbunden")
	}
	return s.client, nil
}

// local returns the package of the open template as a ZIP.
func (s *PublishService) local() ([]byte, *pkg.Package, error) {
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	if s.c.doc == nil {
		return nil, nil, errors.New("keine Vorlage geöffnet")
	}
	b, err := s.c.doc.Bytes()
	if err != nil {
		return nil, nil, err
	}
	p, err := pkg.Read(b)
	return b, p, err
}

// Compare compares the open template with the active version on the
// server, file by file and template.xml line by line.
func (s *PublishService) Compare(id string) (PublishDiff, error) {
	c, err := s.connected()
	if err != nil {
		return PublishDiff{}, err
	}
	if !server.ValidID(id) {
		return PublishDiff{}, errors.New("die ID darf nur Kleinbuchstaben, Ziffern, - und _ enthalten, bis 64 Zeichen")
	}
	_, lp, err := s.local()
	if err != nil {
		return PublishDiff{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	d := PublishDiff{ID: id}
	vs, err := c.Versions(ctx, id)
	if err != nil && !errors.Is(err, server.ErrNotFound) {
		return d, err
	}
	d.Versions = len(vs)
	for _, v := range vs {
		if v.Active {
			d.Active = v.Version
		}
	}
	localFiles := packageFiles(lp)
	remoteFiles := map[string][]byte{}
	if d.Active != "" {
		raw, err := c.Download(ctx, id, d.Active)
		if err != nil {
			return d, err
		}
		rp, err := pkg.Read(raw)
		if err != nil {
			return d, err
		}
		remoteFiles = packageFiles(rp)
	}
	names := map[string]bool{}
	for n := range localFiles {
		names[n] = true
	}
	for n := range remoteFiles {
		names[n] = true
	}
	var sorted []string
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)
	for _, n := range sorted {
		l, inL := localFiles[n]
		r, inR := remoteFiles[n]
		switch {
		case inL && !inR:
			d.Files = append(d.Files, FileChange{n, "added"})
		case !inL && inR:
			d.Files = append(d.Files, FileChange{n, "removed"})
		case string(l) != string(r):
			d.Files = append(d.Files, FileChange{n, "changed"})
		}
	}
	d.Same = len(d.Files) == 0
	d.Template = lineDiff(string(remoteFiles["template.xml"]), string(localFiles["template.xml"]))
	return d, nil
}

// packageFiles returns all files of a package including template.xml in the
// canonical form of pdx fmt, so that equal templates compare as equal.
func packageFiles(p *pkg.Package) map[string][]byte {
	out := map[string][]byte{"template.xml": template.Write(p.Template)}
	for n, b := range p.Files {
		out[n] = b
	}
	return out
}

// Publish uploads the template as a new version, content equal to an
// existing version creates no new one. activate activates it immediately.
// If the server rejects the template, the error contains all findings.
func (s *PublishService) Publish(id string, activate bool) (PublishResult, error) {
	c, err := s.connected()
	if err != nil {
		return PublishResult{}, err
	}
	raw, _, err := s.local()
	if err != nil {
		return PublishResult{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	before, _ := c.Versions(ctx, id)
	v, err := c.Upload(ctx, id, raw, activate)
	if err != nil {
		return PublishResult{}, err
	}
	created := true
	for _, b := range before {
		if b.Version == v.Version {
			created = false
		}
	}
	return PublishResult{Version: v.Version, Created: created, Active: v.Active}, nil
}

// Activate makes a version on the server active.
func (s *PublishService) Activate(id, version string) error {
	c, err := s.connected()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return c.Activate(ctx, id, version)
}

// lineDiff compares two texts line by line (longest common
// subsequence). Very large texts return only removed and new lines.
func lineDiff(a, b string) []DiffLine {
	if a == b {
		return nil
	}
	al, bl := splitLines(a), splitLines(b)
	if len(al)*len(bl) > 4_000_000 {
		var out []DiffLine
		for _, l := range al {
			out = append(out, DiffLine{"-", l})
		}
		for _, l := range bl {
			out = append(out, DiffLine{"+", l})
		}
		return out
	}
	n, m := len(al), len(bl)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if al[i] == bl[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var out []DiffLine
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case al[i] == bl[j]:
			out = append(out, DiffLine{" ", al[i]})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			out = append(out, DiffLine{"-", al[i]})
			i++
		default:
			out = append(out, DiffLine{"+", bl[j]})
			j++
		}
	}
	for ; i < n; i++ {
		out = append(out, DiffLine{"-", al[i]})
	}
	for ; j < m; j++ {
		out = append(out, DiffLine{"+", bl[j]})
	}
	return out
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}
