package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrNotFound meldet eine fehlende Vorlage oder Version.
var ErrNotFound = errors.New("nicht gefunden")

// Version beschreibt eine abgelegte Version einer Vorlage. Versionen sind
// unveränderlich und fortlaufend nummeriert.
type Version struct {
	Version string    `json:"version"`
	SHA256  string    `json:"sha256"`
	Size    int       `json:"size"`
	Created time.Time `json:"created"`
	Active  bool      `json:"active"`
}

// TemplateInfo ist eine Vorlage mit ihrer aktiven Version.
type TemplateInfo struct {
	ID     string `json:"id"`
	Active string `json:"active,omitempty"`
}

// Store ist die Vorlagenablage.
type Store interface {
	// Get liefert eine Version, leer bedeutet die aktive, mit Rohdaten
	// des Pakets und Versionsnummer.
	Get(ctx context.Context, id, version string) ([]byte, string, error)
	// Put legt ein Paket als neue Version ab. Gleicher Inhalt wie eine
	// vorhandene Version liefert diese, ohne neue anzulegen.
	Put(ctx context.Context, id string, raw []byte) (Version, bool, error)
	// Activate macht eine Version zur aktiven.
	Activate(ctx context.Context, id, version string) error
	// List liefert alle Vorlagen, Versions alle Versionen einer Vorlage.
	List(ctx context.Context) ([]TemplateInfo, error)
	Versions(ctx context.Context, id string) ([]Version, error)
}

var reID = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// ValidID prüft eine Vorlagen-ID: Kleinbuchstaben, Ziffern, - und _, bis
// 64 Zeichen. So bleibt sie als Verzeichnisname sicher.
func ValidID(id string) bool { return reID.MatchString(id) }

func digest(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// DirStore legt Vorlagen in einem Verzeichnis ab, auch auf einem
// gemeinsamen Volume für mehrere Instanzen: {id}/{n}.zip und {id}/active.
// Neue Nummern werden exklusiv angelegt, active wird atomar ersetzt.
type DirStore struct {
	Dir string
}

func (d *DirStore) path(id string, parts ...string) (string, error) {
	if !ValidID(id) {
		return "", ErrNotFound
	}
	return filepath.Join(append([]string{d.Dir, id}, parts...)...), nil
}

func (d *DirStore) numbers(id string) ([]int, error) {
	dir, err := d.path(id)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ns []int
	for _, e := range entries {
		if n, err := strconv.Atoi(strings.TrimSuffix(e.Name(), ".zip")); err == nil && strings.HasSuffix(e.Name(), ".zip") && n > 0 {
			ns = append(ns, n)
		}
	}
	sort.Ints(ns)
	return ns, nil
}

func (d *DirStore) active(id string) string {
	p, err := d.path(id, "active")
	if err != nil {
		return ""
	}
	b, _ := os.ReadFile(p)
	return strings.TrimSpace(string(b))
}

func (d *DirStore) Get(_ context.Context, id, version string) ([]byte, string, error) {
	if version == "" {
		version = d.active(id)
	}
	if _, err := strconv.Atoi(version); err != nil {
		return nil, "", ErrNotFound
	}
	p, err := d.path(id, version+".zip")
	if err != nil {
		return nil, "", err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, "", ErrNotFound
	}
	return b, version, err
}

func (d *DirStore) Put(ctx context.Context, id string, raw []byte) (Version, bool, error) {
	dir, err := d.path(id)
	if err != nil {
		return Version{}, false, fmt.Errorf("Vorlagen-ID %q ist ungültig", id)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Version{}, false, err
	}
	sum := digest(raw)
	vs, err := d.Versions(ctx, id)
	if err != nil {
		return Version{}, false, err
	}
	for _, v := range vs {
		if v.SHA256 == sum {
			return v, false, nil
		}
	}
	n := 1
	if len(vs) > 0 {
		n, _ = strconv.Atoi(vs[len(vs)-1].Version)
		n++
	}
	for ; ; n++ {
		// Erst unter temporärem Namen schreiben, dann exklusiv verlinken,
		// damit nie eine halbe Version sichtbar ist.
		tmp, err := os.CreateTemp(dir, ".upload-*")
		if err != nil {
			return Version{}, false, err
		}
		_, werr := tmp.Write(raw)
		cerr := tmp.Close()
		if werr != nil || cerr != nil {
			os.Remove(tmp.Name())
			return Version{}, false, errors.Join(werr, cerr)
		}
		target := filepath.Join(dir, strconv.Itoa(n)+".zip")
		err = os.Link(tmp.Name(), target)
		os.Remove(tmp.Name())
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return Version{}, false, err
		}
		st, _ := os.Stat(target)
		return Version{Version: strconv.Itoa(n), SHA256: sum, Size: len(raw), Created: st.ModTime().UTC(), Active: false}, true, nil
	}
}

func (d *DirStore) Activate(ctx context.Context, id, version string) error {
	if _, _, err := d.Get(ctx, id, version); err != nil {
		return err
	}
	dir, _ := d.path(id)
	tmp, err := os.CreateTemp(dir, ".active-*")
	if err != nil {
		return err
	}
	_, werr := tmp.WriteString(version + "\n")
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		os.Remove(tmp.Name())
		return errors.Join(werr, cerr)
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, "active"))
}

func (d *DirStore) List(_ context.Context) ([]TemplateInfo, error) {
	entries, err := os.ReadDir(d.Dir)
	if err != nil {
		return nil, err
	}
	var out []TemplateInfo
	for _, e := range entries {
		if e.IsDir() && ValidID(e.Name()) {
			out = append(out, TemplateInfo{ID: e.Name(), Active: d.active(e.Name())})
		}
	}
	return out, nil
}

func (d *DirStore) Versions(_ context.Context, id string) ([]Version, error) {
	ns, err := d.numbers(id)
	if err != nil {
		return nil, err
	}
	act := d.active(id)
	var out []Version
	for _, n := range ns {
		p, _ := d.path(id, strconv.Itoa(n)+".zip")
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		st, _ := os.Stat(p)
		v := strconv.Itoa(n)
		out = append(out, Version{Version: v, SHA256: digest(b), Size: len(b), Created: st.ModTime().UTC(), Active: v == act})
	}
	return out, nil
}

// MemStore hält Vorlagen im Speicher, für Tests.
type MemStore struct {
	mu   sync.Mutex
	data map[string][][]byte
	act  map[string]string
}

func (m *MemStore) init() {
	if m.data == nil {
		m.data, m.act = map[string][][]byte{}, map[string]string{}
	}
}

func (m *MemStore) Get(_ context.Context, id, version string) ([]byte, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.init()
	if version == "" {
		version = m.act[id]
	}
	n, err := strconv.Atoi(version)
	if err != nil || n < 1 || n > len(m.data[id]) {
		return nil, "", ErrNotFound
	}
	return m.data[id][n-1], version, nil
}

func (m *MemStore) Put(_ context.Context, id string, raw []byte) (Version, bool, error) {
	if !ValidID(id) {
		return Version{}, false, fmt.Errorf("Vorlagen-ID %q ist ungültig", id)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.init()
	sum := digest(raw)
	for i, b := range m.data[id] {
		if digest(b) == sum {
			v := strconv.Itoa(i + 1)
			return Version{Version: v, SHA256: sum, Size: len(b), Active: m.act[id] == v}, false, nil
		}
	}
	m.data[id] = append(m.data[id], append([]byte(nil), raw...))
	return Version{Version: strconv.Itoa(len(m.data[id])), SHA256: sum, Size: len(raw)}, true, nil
}

func (m *MemStore) Activate(ctx context.Context, id, version string) error {
	if _, _, err := m.Get(ctx, id, version); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.act[id] = version
	return nil
}

func (m *MemStore) List(context.Context) ([]TemplateInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []TemplateInfo
	for id := range m.data {
		out = append(out, TemplateInfo{ID: id, Active: m.act[id]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (m *MemStore) Versions(_ context.Context, id string) ([]Version, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Version
	for i, b := range m.data[id] {
		v := strconv.Itoa(i + 1)
		out = append(out, Version{Version: v, SHA256: digest(b), Size: len(b), Active: m.act[id] == v})
	}
	return out, nil
}
