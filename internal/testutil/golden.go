// Package testutil contains helpers for golden file tests and a simple
// structure check of generated PDF files.
package testutil

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// Update reports whether golden files should be rewritten. It is set through
// the environment variable UPDATE_GOLDEN=1 (see make golden), because a flag
// would not be understood by all test binaries.
func Update() bool { return os.Getenv("UPDATE_GOLDEN") == "1" }

// Golden compares got with the file path. With UPDATE_GOLDEN=1 the file is
// written instead.
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
