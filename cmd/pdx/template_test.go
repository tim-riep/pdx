package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const musterDir = "../../testdata/templates/musterrechnung"

func TestTemplateCLI(t *testing.T) {
	dir := t.TempDir()
	zip := filepath.Join(dir, "rechnung.zip")
	var so, se bytes.Buffer
	if code := run([]string{"pack", musterDir, "-o", zip}, &so, &se); code != 0 {
		t.Fatalf("pack: %d %s", code, se.String())
	}
	if code := run([]string{"check", zip}, &so, &se); code != 0 {
		t.Fatalf("check: %d %s %s", code, so.String(), se.String())
	}
	if !strings.Contains(so.String(), "keine Befunde") {
		t.Errorf("check meldet %q", so.String())
	}
	xsl := filepath.Join(dir, "rechnung.xsl")
	if code := run([]string{"compile", zip, "-o", xsl, "--source-map", filepath.Join(dir, "map.json")}, &so, &se); code != 0 {
		t.Fatalf("compile: %d %s", code, se.String())
	}
	if b, _ := os.ReadFile(xsl); !bytes.Contains(b, []byte(`<xsl:stylesheet version="3.0"`)) {
		t.Error("compile schreibt kein XSLT")
	}
	pdf := filepath.Join(dir, "r.pdf")
	data := filepath.Join(musterDir, "samples", "rechnung.xml")
	fo := filepath.Join(dir, "r.fo")
	if code := run([]string{"render", "--template", zip, data, "-o", pdf, "--strict", "--fo", fo,
		"--creation-date", "2026-01-15T09:00:00Z"}, &so, &se); code != 0 {
		t.Fatalf("render --template: %d %s", code, se.String())
	}
	b, _ := os.ReadFile(pdf)
	if !bytes.Contains(b, []byte("/UF (rechnung.xml)")) {
		t.Error("Datenanhang fehlt im PDF")
	}
	if _, err := os.Stat(fo); err != nil {
		t.Error("--fo nicht geschrieben")
	}
	// Aus Verzeichnis und ZIP entsteht dieselbe Datei.
	pdf2 := filepath.Join(dir, "r2.pdf")
	if code := run([]string{"render", "--template", musterDir, data, "-o", pdf2, "--creation-date", "2026-01-15T09:00:00Z"}, &so, &se); code != 0 {
		t.Fatalf("render --template Verzeichnis: %d %s", code, se.String())
	}
	if b2, _ := os.ReadFile(pdf2); !bytes.Equal(b, b2) {
		t.Error("ZIP und Verzeichnis ergeben verschiedene PDF-Dateien")
	}
}

// copyDir kopiert das Vorlagenverzeichnis, damit Tests es ändern können.
func copyDir(t *testing.T, src string) string {
	t.Helper()
	dst := t.TempDir()
	filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if info.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		b, _ := os.ReadFile(p)
		return os.WriteFile(filepath.Join(dst, rel), b, 0o644)
	})
	return dst
}

func TestCheckFindings(t *testing.T) {
	dir := copyDir(t, musterDir)
	tp := filepath.Join(dir, "template.xml")
	src, _ := os.ReadFile(tp)
	s := string(src)
	for _, r := range [][2]string{
		{`bind="inv:seller/inv:name"/></text>`, `bind="inv:seller/inv:nam"/></text>`},       // Pfad
		{`bind="inv:description"`, `bind="inv:description" format="betrag"`},                // Typ
		{`bind="inv:number" fontWeight`, `bind="inv:items/inv:item/inv:amount" fontWeight`}, // Kardinalität
	} {
		if !strings.Contains(s, r[0]) {
			t.Fatalf("%q nicht in der Vorlage", r[0])
		}
		s = strings.Replace(s, r[0], r[1], 1)
	}
	os.WriteFile(tp, []byte(s), 0o644)
	var so, se bytes.Buffer
	code := run([]string{"check", dir, "--format", "json"}, &so, &se)
	if code != 1 {
		t.Fatalf("Exit-Code %d, erwartet 1: %s", code, se.String())
	}
	var fs []struct{ Level, ID, Msg string }
	if err := json.Unmarshal(so.Bytes(), &fs); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"briefkopf-firma-name": false, "z-leistung-f": false, "rechnungsnummer-wert": false}
	for _, f := range fs {
		if _, ok := want[f.ID]; ok && f.Level == "Fehler" {
			want[f.ID] = true
		}
	}
	for id, found := range want {
		if !found {
			t.Errorf("kein Fehler mit ID %s in %s", id, so.String())
		}
	}
}

func TestFmt(t *testing.T) {
	dir := t.TempDir()
	tp := filepath.Join(dir, "template.xml")
	src, _ := os.ReadFile(filepath.Join(musterDir, "template.xml"))
	messy := strings.Replace(string(src), `id="musterrechnung" dataRoot="inv:invoice"`, `dataRoot="inv:invoice"   id="musterrechnung"`, 1)
	os.WriteFile(tp, []byte(messy), 0o644)
	var so, se bytes.Buffer
	if code := run([]string{"fmt", "-w", tp}, &so, &se); code != 0 {
		t.Fatalf("fmt: %d %s", code, se.String())
	}
	if got, _ := os.ReadFile(tp); !bytes.Equal(got, src) {
		t.Error("fmt stellt die kanonische Form nicht her")
	}
}
