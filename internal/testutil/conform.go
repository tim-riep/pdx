package testutil

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tim-riep/pdx/internal/verapdf"
)

// ConformanceEnabled meldet, ob Tests veraPDF aufrufen sollen. Gesetzt wird
// das über PDX_CONFORMANCE=1, weil veraPDF Docker oder Java braucht.
func ConformanceEnabled() bool { return os.Getenv("PDX_CONFORMANCE") == "1" }

// Conform prüft PDF-Bytes mit veraPDF gegen ein pdx-Profil und lässt den Test
// bei jedem fehlgeschlagenen Test scheitern. Ohne PDX_CONFORMANCE=1 kehrt die
// Funktion sofort zurück.
func Conform(t testing.TB, name string, pdf []byte, profile string) {
	t.Helper()
	if !ConformanceEnabled() {
		return
	}
	flavour, err := verapdf.Flavour(profile)
	if err != nil {
		t.Fatal(err)
	}
	r, err := verapdf.Detect(context.Background(), func(s string) { t.Log(s) })
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(file, pdf, 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := r.Validate(context.Background(), file, flavour)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Failed() {
		var b bytes.Buffer
		rep.WriteText(&b)
		t.Errorf("%s ist nicht konform zu %s:\n%s", name, profile, b.String())
	}
}
