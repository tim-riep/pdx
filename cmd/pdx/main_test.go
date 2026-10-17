package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderCLI(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "r.pdf")
	tree := filepath.Join(dir, "r.txt")
	att := filepath.Join(dir, "daten.xml")
	os.WriteFile(att, []byte("<a/>"), 0o644)
	var so, se bytes.Buffer
	code := run([]string{"render", "../../testdata/fo/brief.fo", "-o", out, "--profile", "pdfa-3b",
		"--attach", att + ":Data:application/xml", "--creation-date", "2026-01-01T00:00:00Z", "--area-tree", tree}, &so, &se)
	if code != 0 {
		t.Fatalf("Exit-Code %d: %s", code, se.String())
	}
	a, _ := os.ReadFile(out)
	if !bytes.Contains(a, []byte("/UF (daten.xml)")) || !bytes.Contains(a, []byte("/AFRelationship /Data")) {
		t.Error("Anhang aus --attach fehlt")
	}
	if _, err := os.Stat(tree); err != nil {
		t.Error("Area Tree nicht geschrieben")
	}
	// Gleiches Datum ergibt gleiche Bytes, auch über SOURCE_DATE_EPOCH.
	out2 := filepath.Join(dir, "r2.pdf")
	t.Setenv("SOURCE_DATE_EPOCH", "1767225600") // 2026-01-01T00:00:00Z
	if code := run([]string{"render", "../../testdata/fo/brief.fo", "-o", out2, "--attach", att + ":Data:application/xml"}, &so, &se); code != 0 {
		t.Fatalf("Exit-Code %d: %s", code, se.String())
	}
	b, _ := os.ReadFile(out2)
	if !bytes.Equal(a, b) {
		t.Error("SOURCE_DATE_EPOCH ergibt andere Bytes als --creation-date")
	}
}

func TestUsageErrors(t *testing.T) {
	cases := [][]string{
		{},
		{"unbekannt"},
		{"render"},
		{"render", "a.fo", "--creation-date", "gestern"},
		{"render", "a.fo", "--attach", "ohne-trenner"},
		{"verify"},
		{"verify", "a.pdf", "--profile", "pdf-1.7"},
		{"verify", "a.pdf", "--format", "xml"},
		{"render", "a.fo", "--strict"},
		{"compile"},
		{"check"},
		{"check", "a", "--format", "xml"},
		{"fmt"},
		{"pack"},
	}
	for _, c := range cases {
		var so, se bytes.Buffer
		if code := run(c, &so, &se); code != 2 {
			t.Errorf("%v: Exit-Code %d statt 2 (%s)", c, code, se.String())
		}
	}
	var so, se bytes.Buffer
	if code := run([]string{"render", "fehlt.fo", "--creation-date", "2026-01-01T00:00:00Z"}, &so, &se); code != 1 {
		t.Errorf("fehlende Eingabe: Exit-Code %d statt 1", code)
	}
	if code := run([]string{"version"}, &so, &se); code != 0 || !strings.Contains(so.String(), "veraPDF 1.30.3") {
		t.Errorf("version: %d %s", code, so.String())
	}
}

func TestVerifyCLI(t *testing.T) {
	if os.Getenv("PDX_CONFORMANCE") != "1" {
		t.Skip("PDX_CONFORMANCE=1 setzen, um veraPDF aufzurufen")
	}
	var so, se bytes.Buffer
	if code := run([]string{"verify", "../../testdata/golden/rechnung.pdf"}, &so, &se); code != 0 {
		t.Fatalf("konforme Datei: Exit-Code %d\n%s%s", code, so.String(), se.String())
	}
	so.Reset()
	if code := run([]string{"verify", "../../pdf/render/testdata/text.pdf"}, &so, &se); code != 1 {
		t.Fatalf("nicht konforme Datei: Exit-Code %d", code)
	}
	if !strings.Contains(so.String(), "Klausel 6.6.2.1 Test 1") {
		t.Errorf("Befund fehlt in der Ausgabe:\n%s", so.String())
	}
}
