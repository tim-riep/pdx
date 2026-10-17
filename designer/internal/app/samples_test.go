package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tim-riep/pdx/template"
)

func TestSamples(t *testing.T) {
	c := NewCore(nil, nil, t.TempDir())
	if _, err := NewProjectService(c).Open(muster); err != nil {
		t.Fatal(err)
	}
	ss := NewSampleDataService(c)
	l, err := ss.List()
	if err != nil || len(l.Items) != 1 || !l.Items[0].Selected || l.Placeholder {
		t.Fatalf("Liste %+v %v", l, err)
	}

	// Randfälle erzeugen: Datei im Paket, gewählt, rückgängig machbar.
	for kind, want := range map[string]string{"minimal": "samples/rand-minimal.xml", "maximal": "samples/rand-maximal.xml", "lang": "samples/rand-lange-texte.xml"} {
		if _, err := ss.Generate(kind); err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		l, _ = ss.List()
		var sel string
		for _, it := range l.Items {
			if it.Selected {
				sel = it.Path
			}
		}
		if sel != want {
			t.Fatalf("%s: gewählt %q", kind, sel)
		}
	}
	files := c.doc.Pkg.Files
	if n := strings.Count(string(files["samples/rand-maximal.xml"]), "<inv:item>"); n != 60 {
		t.Fatalf("maximal hat %d Positionen", n)
	}
	if strings.Contains(string(files["samples/rand-minimal.xml"]), "paymentTerms") {
		t.Fatal("minimal enthält optionale Knoten")
	}
	if !strings.Contains(string(files["samples/rand-lange-texte.xml"]), "Donaudampfschifffahrt") {
		t.Fatal("lange Texte fehlen")
	}
	// Die Randfälle setzen ohne Fehler.
	for _, p := range runChecks(template.Write(c.doc.Pkg.Template), files) {
		if p.Level == "Fehler" && p.Source != "beispiel" {
			t.Errorf("Befund %+v", p)
		}
	}
	if _, err := NewTemplateService(c).Undo(); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.doc.Pkg.Files["samples/rand-lange-texte.xml"]; ok {
		t.Fatal("Undo entfernt den Randfall nicht")
	}

	// Import und Auswahl.
	src := filepath.Join(t.TempDir(), "Kunde März.xml")
	os.WriteFile(src, files["samples/rechnung.xml"], 0o644)
	if _, err := ss.Import(src); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.doc.Pkg.Files["samples/Kunde-M-rz.xml"]; !ok {
		t.Fatalf("Import: %v", c.doc.Pkg.Names())
	}
	if err := ss.Select("samples/rechnung.xml"); err != nil {
		t.Fatal(err)
	}
	if err := ss.Select("samples/fehlt.xml"); err == nil {
		t.Fatal("unbekannte Auswahl angenommen")
	}
	if _, err := ss.Remove("samples/Kunde-M-rz.xml"); err != nil {
		t.Fatal(err)
	}
	if _, err := ss.Remove("schema/invoice.xsd"); err == nil {
		t.Fatal("Schema als Beispieldaten entfernt")
	}
}
