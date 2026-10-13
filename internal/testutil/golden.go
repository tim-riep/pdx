// Package testutil enthält Helfer für Golden-File-Tests und eine einfache
// Strukturprüfung erzeugter PDF-Dateien.
package testutil

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// Update meldet, ob Golden Files neu geschrieben werden sollen. Gesetzt wird
// das über die Umgebungsvariable UPDATE_GOLDEN=1 (siehe make golden), weil ein
// Flag nicht von allen Testbinaries verstanden würde.
func Update() bool { return os.Getenv("UPDATE_GOLDEN") == "1" }

// Golden vergleicht got mit der Datei path. Mit UPDATE_GOLDEN=1 wird die Datei
// stattdessen geschrieben.
func Golden(t testing.TB, path string, got []byte) {
	t.Helper()
	if Update() {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Golden File fehlt (UPDATE_GOLDEN=1 erzeugt es): %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("Ausgabe weicht von %s ab (%d statt %d Bytes, erste Abweichung bei Byte %d)",
			path, len(got), len(want), firstDiff(got, want))
	}
}

func firstDiff(a, b []byte) int {
	n := min(len(a), len(b))
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}
