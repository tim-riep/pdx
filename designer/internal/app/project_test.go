package app

import (
	"path/filepath"
	"sync"
	"testing"
	"time"
)

const muster = "../../../testdata/templates/musterrechnung"

// recorder remembers the events sent to the UI.
type recorder struct {
	mu     sync.Mutex
	events []string
	last   map[string]any
}

func (r *recorder) Emit(name string, data any) {
	r.mu.Lock()
	r.events = append(r.events, name)
	if r.last == nil {
		r.last = map[string]any{}
	}
	r.last[name] = data
	r.mu.Unlock()
}

// wait waits for an event that satisfies cond and returns its data.
func (r *recorder) wait(t *testing.T, name string, cond func(any) bool) any {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		d, ok := r.last[name]
		r.mu.Unlock()
		if ok && cond(d) {
			return d
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("Ereignis %s nicht eingetroffen", name)
	return nil
}

func (r *recorder) count(name string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, e := range r.events {
		if e == name {
			n++
		}
	}
	return n
}

// change simulates a change to the document until the command model exists.
func change(c *Core) {
	c.mu.Lock()
	c.changedLocked()
	c.mu.Unlock()
}

func TestOpenSave(t *testing.T) {
	rec := &recorder{}
	c := NewCore(rec, nil, t.TempDir())
	ps := NewProjectService(c)
	if ps.State().Open {
		t.Fatal("Projekt offen ohne Öffnen")
	}
	st, err := ps.Open(muster)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Open || st.Dirty || st.Name != "musterrechnung" || st.Format != "dir" {
		t.Errorf("nach Open: %+v", st)
	}
	change(c)
	if !ps.State().Dirty {
		t.Error("nach Änderung nicht geändert")
	}
	target := filepath.Join(t.TempDir(), "rechnung.zip")
	st, err = ps.SaveAs(target)
	if err != nil {
		t.Fatal(err)
	}
	if st.Dirty || st.Path != target || st.Format != "zip" {
		t.Errorf("nach SaveAs: %+v", st)
	}
	recent := ps.Recent()
	if len(recent) != 2 || recent[0].Path != target {
		t.Errorf("Recent: %+v", recent)
	}
	if rec.count(EventProject) < 3 {
		t.Errorf("zu wenige Ereignisse %s: %v", EventProject, rec.events)
	}
	if _, err := ps.OpenDialog(false); err == nil {
		t.Error("OpenDialog ohne Dialoge gelingt")
	}
}

// TestAutosave checks backup and recovery in a new session.
func TestAutosave(t *testing.T) {
	data := t.TempDir()
	c := NewCore(nil, nil, data)
	c.AutosaveDelay = 10 * time.Millisecond
	ps := NewProjectService(c)
	if _, err := ps.Open(muster); err != nil {
		t.Fatal(err)
	}
	change(c)
	deadline := time.Now().Add(2 * time.Second)
	for len(NewProjectService(NewCore(nil, nil, data)).Recoverable()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("keine automatische Sicherung geschrieben")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := ps.Recoverable(); len(got) != 0 {
		t.Errorf("eigene Sitzung als wiederherstellbar gelistet: %+v", got)
	}

	// New session after a crash.
	c2 := NewCore(nil, nil, data)
	ps2 := NewProjectService(c2)
	list := ps2.Recoverable()
	if len(list) != 1 || list[0].Name != "musterrechnung" {
		t.Fatalf("Recoverable: %+v", list)
	}
	st, err := ps2.Recover(list[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Dirty || st.Name != "musterrechnung" {
		t.Errorf("nach Recover: %+v", st)
	}
	if got := ps2.Recoverable(); len(got) != 0 {
		t.Errorf("Sicherung nach Recover noch vorhanden: %+v", got)
	}
	if _, err := ps2.Recover("../../etc"); err == nil {
		t.Error("Recover mit unzulässiger Kennung gelingt")
	}
}

// TestSaveRemovesAutosave checks that saving removes the backup.
func TestSaveRemovesAutosave(t *testing.T) {
	data := t.TempDir()
	c := NewCore(nil, nil, data)
	c.AutosaveDelay = time.Hour
	ps := NewProjectService(c)
	ps.New()
	change(c)
	c.Flush()
	if len(NewProjectService(NewCore(nil, nil, data)).Recoverable()) != 1 {
		t.Fatal("Flush schreibt keine Sicherung")
	}
	if _, err := ps.SaveAs(filepath.Join(t.TempDir(), "neu.zip")); err != nil {
		t.Fatal(err)
	}
	if n := len(NewProjectService(NewCore(nil, nil, data)).Recoverable()); n != 0 {
		t.Errorf("%d Sicherungen nach dem Speichern", n)
	}
}

func TestMayClose(t *testing.T) {
	rec := &recorder{}
	c := NewCore(rec, nil, t.TempDir())
	ps := NewProjectService(c)
	if !c.MayClose() {
		t.Error("ohne Dokument keine Rückfrage erwartet")
	}
	ps.New()
	change(c)
	if c.MayClose() || rec.count(EventCloseRequest) != 1 {
		t.Error("mit Änderungen erwartet: Rückfrage")
	}
	closed := false
	NewAppService(c, func() { closed = true }).ConfirmClose()
	if !closed || !c.MayClose() {
		t.Error("nach ConfirmClose darf das Fenster schließen")
	}
}
