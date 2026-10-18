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

// PublishService veröffentlicht die Vorlage auf einem pdx-server: Vergleich
// mit der aktiven Version, Hochladen als neue Version, Aktivieren. Der
// API-Schlüssel bleibt nur im Speicher dieser Sitzung, gespeichert wird nur
// die Adresse des Servers.
type PublishService struct {
	c      *Core
	mu     sync.Mutex
	client *server.Client
}

// NewPublishService liefert den Service.
func NewPublishService(c *Core) *PublishService { return &PublishService{c: c} }

// PublishState ist der Stand der Verbindung für die Oberfläche.
type PublishState struct {
	URL       string `json:"url"`
	Connected bool   `json:"connected"`
	// ID ist der Vorschlag für die Vorlagen-ID auf dem Server.
	ID string `json:"id"`
}

// FileChange ist eine Datei, die sich gegenüber der aktiven Version ändert.
type FileChange struct {
	Name string `json:"name"`
	// Kind ist added, removed oder changed.
	Kind string `json:"kind"`
}

// DiffLine ist eine Zeile im Vergleich von template.xml.
type DiffLine struct {
	// Op ist " " (gleich), "+" (neu) oder "-" (entfernt).
	Op   string `json:"op"`
	Text string `json:"text"`
}

// PublishDiff vergleicht die Vorlage mit der aktiven Version auf dem Server.
type PublishDiff struct {
	ID string `json:"id"`
	// Active ist die aktive Version, leer, wenn es die Vorlage noch nicht
	// gibt oder keine Version aktiv ist.
	Active   string       `json:"active"`
	Versions int          `json:"versions"`
	Same     bool         `json:"same"`
	Files    []FileChange `json:"files"`
	Template []DiffLine   `json:"template"`
}

// PublishResult ist das Ergebnis des Hochladens.
type PublishResult struct {
	Version string `json:"version"`
	Created bool   `json:"created"`
	Active  bool   `json:"active"`
}

type publishSettings struct {
	URL string `json:"url"`
}

func (s *PublishService) settingsPath() string { return filepath.Join(s.c.dataDir, "publish.json") }

// State liefert Adresse, Verbindung und den Vorschlag für die ID.
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

// suggestID leitet die ID aus der ID der Vorlage oder dem Dateinamen ab.
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

// Connect prüft Adresse und Schlüssel mit einer Abfrage der Vorlagen. Der
// Schlüssel bleibt nur im Speicher.
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

// Disconnect vergisst den Schlüssel.
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

// local liefert das Paket der geöffneten Vorlage als ZIP.
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

// Compare vergleicht die geöffnete Vorlage mit der aktiven Version auf dem
// Server, Datei für Datei und template.xml Zeile für Zeile.
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

// packageFiles liefert alle Dateien eines Pakets samt template.xml in der
// kanonischen Form von pdx fmt, so dass gleiche Vorlagen gleich vergleichen.
func packageFiles(p *pkg.Package) map[string][]byte {
	out := map[string][]byte{"template.xml": template.Write(p.Template)}
	for n, b := range p.Files {
		out[n] = b
	}
	return out
}

// Publish lädt die Vorlage als neue Version hoch, gleicher Inhalt wie eine
// vorhandene Version legt keine neue an. activate aktiviert sie sofort.
// Lehnt der Server die Vorlage ab, enthält der Fehler alle Befunde.
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

// Activate macht eine Version auf dem Server aktiv.
func (s *PublishService) Activate(id, version string) error {
	c, err := s.connected()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return c.Activate(ctx, id, version)
}

// lineDiff vergleicht zwei Texte zeilenweise (längste gemeinsame
// Teilfolge). Sehr große Texte liefern nur entfernte und neue Zeilen.
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
