package testutil

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tim-riep/pdx/verapdf"
)

// ConformanceEnabled reports whether tests should call veraPDF. It is set
// through PDX_CONFORMANCE=1, because veraPDF needs Docker or Java.
func ConformanceEnabled() bool { return os.Getenv("PDX_CONFORMANCE") == "1" }

// Conform checks PDF bytes with veraPDF against a pdx profile and fails the
// test on every failed check. Without PDX_CONFORMANCE=1 the function returns
// immediately.
func Conform(t testing.TB, name string, pdf []byte, profile string) {
	t.Helper()
	if !ConformanceEnabled() {
		return
	}
	flavours, err := verapdf.Flavours(profile)
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
	// One run per flavour, all findings in one message.
	for _, flavour := range flavours {
		rep, err := r.Validate(context.Background(), file, flavour)
		if err != nil {
			t.Fatal(err)
		}
		if rep.Failed() {
			var b bytes.Buffer
			rep.WriteText(&b)
			t.Errorf("%s ist nicht konform zu %s (Flavour %s):\n%s", name, profile, flavour, b.String())
		}
	}
}
