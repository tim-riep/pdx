package app

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/tim-riep/pdx/designer/internal/model"
)

// ProjectState beschreibt das geöffnete Projekt für die Oberfläche.
type ProjectState struct {
	Open   bool   `json:"open"`
	Path   string `json:"path"`
	Name   string `json:"name"`
	Format string `json:"format"`
	Dirty  bool   `json:"dirty"`
	Rev    uint64 `json:"rev"`
	// Doc kennzeichnet das geöffnete Dokument. Wechselt es, ist jeder
	// Lesecache der Oberfläche veraltet.
	Doc string `json:"doc"`
}

// RecentFile ist ein zuletzt geöffnetes Paket.
type RecentFile struct {
	Path   string    `json:"path"`
	Name   string    `json:"name"`
	Opened time.Time `json:"opened"`
}

// Recovery ist eine automatische Sicherung, die nach einem Absturz übrig
// geblieben ist.
type Recovery struct {
	ID    string    `json:"id"`
	Path  string    `json:"path"`
	Name  string    `json:"name"`
	Saved time.Time `json:"saved"`
}

// ProjectService öffnet und speichert Vorlagenpakete.
type ProjectService struct{ c *Core }

// NewProjectService liefert die Sicht auf den gemeinsamen Zustand.
func NewProjectService(c *Core) *ProjectService { return &ProjectService{c} }

var filters = []Filter{{Name: "Vorlagenpaket (*.zip)", Pattern: "*.zip"}}

// State liefert den Zustand des Projekts.
func (s *ProjectService) State() ProjectState {
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	return s.c.stateLocked()
}

func (c *Core) stateLocked() ProjectState {
	if c.doc == nil {
		return ProjectState{}
	}
	return ProjectState{Open: true, Path: c.doc.Path, Name: c.doc.Name(), Format: string(c.doc.Format),
		Dirty: c.doc.Dirty(), Rev: c.doc.Rev(), Doc: c.session}
}

// New legt eine neue Vorlage an.
func (s *ProjectService) New() ProjectState {
	return s.c.setDoc(model.New())
}

// Open öffnet ein Paket (ZIP-Datei oder Verzeichnis).
func (s *ProjectService) Open(path string) (ProjectState, error) {
	d, err := model.Open(path)
	if err != nil {
		return ProjectState{}, err
	}
	st := s.c.setDoc(d)
	s.c.addRecent(d)
	return st, nil
}

// OpenDialog fragt nach einem Paket und öffnet es. dir wählt ein Verzeichnis
// statt einer ZIP-Datei. Ein abgebrochener Dialog ändert nichts.
func (s *ProjectService) OpenDialog(dir bool) (ProjectState, error) {
	if s.c.dialogs == nil {
		return s.State(), ErrNoDialogs
	}
	path, err := s.c.dialogs.OpenFile("Vorlage öffnen", filters, dir)
	if err != nil || path == "" {
		return s.State(), err
	}
	return s.Open(path)
}

// Save speichert an den bisherigen Ort.
func (s *ProjectService) Save() (ProjectState, error) {
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	if s.c.doc == nil {
		return ProjectState{}, errors.New("keine Vorlage geöffnet")
	}
	if err := s.c.doc.Save(); err != nil {
		return s.c.stateLocked(), err
	}
	return s.c.savedLocked(), nil
}

// SaveAs speichert an einen neuen Ort. Ein Pfad auf .zip ergibt eine
// ZIP-Datei, sonst ein Verzeichnis.
func (s *ProjectService) SaveAs(path string) (ProjectState, error) {
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	if s.c.doc == nil {
		return ProjectState{}, errors.New("keine Vorlage geöffnet")
	}
	if err := s.c.doc.SaveAs(path); err != nil {
		return s.c.stateLocked(), err
	}
	st := s.c.savedLocked()
	s.c.addRecentLocked(s.c.doc)
	return st, nil
}

// SaveAsDialog fragt nach dem Ort und speichert.
func (s *ProjectService) SaveAsDialog() (ProjectState, error) {
	if s.c.dialogs == nil {
		return s.State(), ErrNoDialogs
	}
	name := s.State().Name
	if name == "" {
		name = "vorlage"
	}
	path, err := s.c.dialogs.SaveFile("Vorlage speichern", name+".zip", filters)
	if err != nil || path == "" {
		return s.State(), err
	}
	return s.SaveAs(path)
}

// savedLocked räumt nach dem Speichern die automatische Sicherung weg.
func (c *Core) savedLocked() ProjectState {
	if c.saver != nil {
		c.saver.Stop()
		c.saver = nil
	}
	c.removeAutosave(c.session)
	st := c.stateLocked()
	c.emit.Emit(EventProject, st)
	return st
}

// setDoc macht d zum geöffneten Dokument.
func (c *Core) setDoc(d *model.Document) ProjectState {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.saver != nil {
		c.saver.Stop()
		c.saver = nil
	}
	if c.doc != nil && !c.doc.Dirty() {
		c.removeAutosave(c.session)
	}
	c.doc, c.session = d, newSession()
	c.view = model.Snapshot{}
	c.r.latest = Manifest{}
	c.scheduleLocked(0)
	st := c.stateLocked()
	c.emit.Emit(EventProject, st)
	return st
}

func newSession() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Changed vermerkt eine Änderung am Dokument und plant die automatische
// Sicherung. Aufruf mit gehaltener Sperre.
func (c *Core) changedLocked() {
	c.doc.Touch()
	c.afterChangeLocked()
}

// afterChangeLocked plant Sicherung und Setzen nach einer Änderung und
// meldet den neuen Zustand. Aufruf mit gehaltener Sperre.
func (c *Core) afterChangeLocked() {
	if c.saver == nil {
		c.saver = time.AfterFunc(c.AutosaveDelay, c.autosave)
	}
	c.scheduleLocked(renderDelay)
	c.emit.Emit(EventProject, c.stateLocked())
}

// autosave schreibt das Dokument als ZIP in das Datenverzeichnis.
func (c *Core) autosave() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.saver = nil
	if c.doc == nil || !c.doc.Dirty() {
		return
	}
	b, err := c.doc.Bytes()
	if err != nil {
		return
	}
	dir := filepath.Join(c.dataDir, "autosave")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	meta, _ := json.Marshal(Recovery{ID: c.session, Path: c.doc.Path, Name: c.doc.Name(), Saved: time.Now()})
	if os.WriteFile(filepath.Join(dir, c.session+".zip.tmp"), b, 0o600) != nil {
		return
	}
	os.Rename(filepath.Join(dir, c.session+".zip.tmp"), filepath.Join(dir, c.session+".zip"))
	os.WriteFile(filepath.Join(dir, c.session+".json"), meta, 0o600)
}

// Flush schreibt eine anstehende automatische Sicherung sofort, etwa vor dem
// Beenden.
func (c *Core) Flush() {
	c.mu.Lock()
	pending := c.saver != nil
	if pending {
		c.saver.Stop()
	}
	c.mu.Unlock()
	if pending {
		c.autosave()
	}
}

func (c *Core) removeAutosave(id string) {
	if id == "" {
		return
	}
	dir := filepath.Join(c.dataDir, "autosave")
	os.Remove(filepath.Join(dir, id+".zip"))
	os.Remove(filepath.Join(dir, id+".json"))
}

// Recoverable listet Sicherungen früherer Sitzungen, neueste zuerst.
func (s *ProjectService) Recoverable() []Recovery {
	s.c.mu.Lock()
	session := s.c.session
	s.c.mu.Unlock()
	files, _ := filepath.Glob(filepath.Join(s.c.dataDir, "autosave", "*.json"))
	out := []Recovery{}
	for _, f := range files {
		var r Recovery
		b, err := os.ReadFile(f)
		if err != nil || json.Unmarshal(b, &r) != nil || r.ID == session {
			continue
		}
		if _, err := os.Stat(filepath.Join(filepath.Dir(f), r.ID+".zip")); err != nil {
			continue
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Saved.After(out[j].Saved) })
	return out
}

// Recover öffnet eine Sicherung. Das Dokument behält seinen ursprünglichen
// Speicherort und gilt als geändert.
func (s *ProjectService) Recover(id string) (ProjectState, error) {
	if !validID(id) {
		return s.State(), fmt.Errorf("Sicherung %q unbekannt", id)
	}
	dir := filepath.Join(s.c.dataDir, "autosave")
	var r Recovery
	b, err := os.ReadFile(filepath.Join(dir, id+".json"))
	if err != nil || json.Unmarshal(b, &r) != nil {
		return s.State(), fmt.Errorf("Sicherung %q unbekannt", id)
	}
	d, err := model.Open(filepath.Join(dir, id+".zip"))
	if err != nil {
		return s.State(), err
	}
	d.Path = r.Path
	if r.Path != "" {
		d.Format = model.Zip
		if !strings.EqualFold(filepath.Ext(r.Path), ".zip") {
			d.Format = model.Dir
		}
	}
	d.Touch()
	st := s.c.setDoc(d)
	s.c.removeAutosave(id)
	return st, nil
}

// Discard verwirft eine Sicherung.
func (s *ProjectService) Discard(id string) error {
	if !validID(id) {
		return fmt.Errorf("Sicherung %q unbekannt", id)
	}
	s.c.removeAutosave(id)
	return nil
}

func validID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

const maxRecent = 10

// Recent liefert die zuletzt geöffneten Pakete, die es noch gibt.
func (s *ProjectService) Recent() []RecentFile {
	out := []RecentFile{}
	for _, r := range s.c.loadRecent() {
		if _, err := os.Stat(r.Path); err == nil {
			out = append(out, r)
		}
	}
	return out
}

func (c *Core) loadRecent() []RecentFile {
	var list []RecentFile
	b, err := os.ReadFile(filepath.Join(c.dataDir, "recent.json"))
	if err == nil {
		json.Unmarshal(b, &list)
	}
	return list
}

func (c *Core) addRecent(d *model.Document) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.addRecentLocked(d)
}

func (c *Core) addRecentLocked(d *model.Document) {
	if d.Path == "" {
		return
	}
	list := slices.DeleteFunc(c.loadRecent(), func(r RecentFile) bool { return r.Path == d.Path })
	list = append([]RecentFile{{Path: d.Path, Name: d.Name(), Opened: time.Now()}}, list...)
	if len(list) > maxRecent {
		list = list[:maxRecent]
	}
	b, _ := json.MarshalIndent(list, "", "  ")
	os.MkdirAll(c.dataDir, 0o755)
	os.WriteFile(filepath.Join(c.dataDir, "recent.json"), b, 0o600)
}
