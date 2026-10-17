package pkg

import (
	"bytes"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/tim-riep/pdx/template"
)

// TestWriteDir schreibt die Musterrechnung in ein Verzeichnis, liest sie
// verlustfrei zurück und entfernt nur Dateien, die das Paket nicht mehr hat.
func TestWriteDir(t *testing.T) {
	p, err := ReadDir("../../testdata/templates/musterrechnung")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("ref"), 0o644)
	if err := p.WriteDir(dir); err != nil {
		t.Fatal(err)
	}
	q, err := ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(template.Write(q.Template), template.Write(p.Template)) {
		t.Error("template.xml nach dem Zurücklesen verändert")
	}
	if !maps.EqualFunc(q.Files, p.Files, bytes.Equal) {
		t.Errorf("Dateien verändert: %v statt %v", q.Names(), p.Names())
	}

	delete(p.Files, "samples/rechnung.xml")
	if err := p.WriteDir(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "samples", "rechnung.xml")); !os.IsNotExist(err) {
		t.Error("entfernte Beispieldatei liegt noch im Verzeichnis")
	}
	if _, err := os.Stat(filepath.Join(dir, ".git", "HEAD")); err != nil {
		t.Error("Datei außerhalb der Paketordner wurde entfernt")
	}
}

// TestNew prüft Pfade und Verweise beim Bauen aus einer geladenen Vorlage.
func TestNew(t *testing.T) {
	p, err := ReadDir("../../testdata/templates/musterrechnung")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := New(p.Template, p.Files); err != nil {
		t.Fatalf("New mit den Dateien der Musterrechnung: %v", err)
	}
	if _, err := New(p.Template, map[string][]byte{"geheim.txt": nil}); err == nil {
		t.Error("New akzeptiert eine Datei außerhalb der Paketordner")
	}
	if _, err := New(p.Template, nil); err == nil {
		t.Error("New akzeptiert ein Paket ohne das Daten-XSD")
	}
}
