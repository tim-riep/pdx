package model

import (
	"bytes"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tim-riep/pdx/template"
)

// templates returns the templates from testdata/templates.
func templates(t *testing.T) []string {
	t.Helper()
	dirs, err := filepath.Glob("../../../testdata/templates/*")
	if err != nil || len(dirs) == 0 {
		t.Fatalf("keine Vorlagen gefunden: %v", err)
	}
	return dirs
}

// TestRoundTrip opens every template, saves it as a ZIP and as a
// directory and reads it back. Content and bytes stay the same.
func TestRoundTrip(t *testing.T) {
	for _, dir := range templates(t) {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			d, err := Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			want := template.Write(d.Pkg.Template)
			tmp := t.TempDir()
			for _, target := range []string{filepath.Join(tmp, "a.zip"), filepath.Join(tmp, "a")} {
				if err := d.SaveAs(target); err != nil {
					t.Fatal(err)
				}
				e, err := Open(target)
				if err != nil {
					t.Fatal(err)
				}
				if got := template.Write(e.Pkg.Template); !bytes.Equal(got, want) {
					t.Errorf("%s: template.xml verändert", target)
				}
				if !maps.EqualFunc(e.Pkg.Files, d.Pkg.Files, bytes.Equal) {
					t.Errorf("%s: Dateien verändert", target)
				}
			}
			// Saving twice yields the same ZIP bytes (stable diffs).
			a, _ := os.ReadFile(filepath.Join(tmp, "a.zip"))
			if err := d.SaveAs(filepath.Join(tmp, "b.zip")); err != nil {
				t.Fatal(err)
			}
			b, _ := os.ReadFile(filepath.Join(tmp, "b.zip"))
			if !bytes.Equal(a, b) {
				t.Error("zwei Speichervorgänge ergeben verschiedene ZIP-Dateien")
			}
			// The directory contains template.xml in canonical form.
			src, _ := os.ReadFile(filepath.Join(tmp, "a", "template.xml"))
			if !bytes.Equal(src, want) {
				t.Error("template.xml im Verzeichnis nicht kanonisch")
			}
		})
	}
}

func TestNew(t *testing.T) {
	d := New()
	if err := d.Pkg.Template.Validate(); err != nil {
		t.Fatal(err)
	}
	if d.Dirty() || d.Path != "" {
		t.Errorf("neue Vorlage: Dirty %v, Pfad %q", d.Dirty(), d.Path)
	}
	d.Touch()
	if !d.Dirty() {
		t.Error("nach Touch nicht geändert")
	}
	if err := d.Save(); err == nil {
		t.Error("Save ohne Pfad gelingt")
	}
	if err := d.SaveAs(filepath.Join(t.TempDir(), "neu.zip")); err != nil {
		t.Fatal(err)
	}
	if d.Dirty() || d.Format != Zip || d.Name() != "neu" {
		t.Errorf("nach SaveAs: Dirty %v, Format %s, Name %q", d.Dirty(), d.Format, d.Name())
	}
}

func TestOpenMissing(t *testing.T) {
	_, err := Open(filepath.Join(t.TempDir(), "fehlt.zip"))
	if err == nil || !strings.Contains(err.Error(), "nicht gefunden") {
		t.Errorf("Fehler %v", err)
	}
}
