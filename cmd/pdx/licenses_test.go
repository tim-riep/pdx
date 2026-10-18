package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestLicensesCLI(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"licenses"}, &out, &errb); code != 0 {
		t.Fatal(errb.String())
	}
	for _, want := range []string{"Apache License", "SIL OPEN FONT LICENSE", "golang.org/x/text", "Copyright 2026 Tim Riep"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("%q fehlt", want)
		}
	}
	out.Reset()
	run([]string{"licenses", "--list"}, &out, &errb)
	if !strings.HasPrefix(out.String(), "pdx: Apache-2.0\n") || !strings.Contains(out.String(), "github.com/boombuler/barcode v1.1.0: MIT") {
		t.Errorf("Liste: %s", out.String())
	}
}
