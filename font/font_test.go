package font

import (
	"errors"
	"os"
	"testing"

	"github.com/tim-riep/pdx/areatree"
)

func sourceSerif(t testing.TB) *Face {
	t.Helper()
	data, err := os.ReadFile("../testdata/fonts/source-serif-4/SourceSerif4-Regular.otf")
	if err != nil {
		t.Fatal(err)
	}
	f, err := Load(data, 0)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestRegistryStandardFonts(t *testing.T) {
	r := NewRegistry()
	cases := []struct {
		family string
		weight int
		italic bool
		want   string
	}{
		{"Helvetica", 400, false, "LiberationSans"},
		{"Helvetica", 700, false, "LiberationSans-Bold"},
		{"'Times New Roman', serif", 400, true, "LiberationSerif-Italic"},
		{"Unbekannt, Courier", 700, true, "LiberationMono-BoldItalic"},
		{"sans-serif", 600, false, "LiberationSans-Bold"},
		{"sans-serif", 300, false, "LiberationSans"},
	}
	for _, c := range cases {
		f, err := r.Lookup(c.family, c.weight, c.italic)
		if err != nil {
			t.Fatalf("%s: %v", c.family, err)
		}
		if f.PostScript != c.want {
			t.Errorf("Lookup(%q, %d, %v) = %s, erwartet %s", c.family, c.weight, c.italic, f.PostScript, c.want)
		}
	}
	// Unbekannte Familien fallen nach XSL 1.1 7.9.3 auf die Ersatzfamilie.
	if f, err := r.Lookup("Symbol", 700, false); err != nil || f.PostScript != "LiberationSans-Bold" {
		t.Errorf("Ersatz für Symbol: %v %v", f, err)
	}
	if f, err := r.Lookup("cursive", 400, false); err != nil || f.PostScript != "LiberationSerif" {
		t.Errorf("cursive: %v %v", f, err)
	}
}

func TestOTFMetadata(t *testing.T) {
	f := sourceSerif(t)
	if !f.CFF || f.PostScript != "SourceSerif4-Regular" || f.Weight != 400 || f.Italic {
		t.Errorf("unerwartete Metadaten: %+v", f)
	}
	if !f.Embeddable() {
		t.Error("Source Serif muss einbettbar sein")
	}
}

func TestShapeLigatureAndClusters(t *testing.T) {
	f := sourceSerif(t)
	gs, err := f.Shape("ffi Grüße", 10*areatree.Pt, ShapeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var text string
	for _, g := range gs {
		text += g.Text
		if g.ID == 0 {
			t.Fatal(".notdef im Ergebnis")
		}
	}
	if text != "ffi Grüße" {
		t.Errorf("Clustertext %q", text)
	}
	if len(gs) >= 9 {
		t.Errorf("Ligatur ffi nicht gebildet, %d Glyphen", len(gs))
	}
	if gs[0].Text != "ffi" {
		t.Errorf("erste Glyphe trägt %q statt ffi", gs[0].Text)
	}
	plain, _ := f.Shape("ffi", 10*areatree.Pt, ShapeOptions{Features: []string{"-liga"}})
	if len(plain) != 3 {
		t.Errorf("mit -liga %d Glyphen statt 3", len(plain))
	}
}

func TestShapeMissingGlyph(t *testing.T) {
	r := NewRegistry()
	f, _ := r.Lookup("Helvetica", 400, false)
	_, err := f.Shape("A中", 10*areatree.Pt, ShapeOptions{})
	var me *MissingGlyphError
	if !errors.As(err, &me) || len(me.Runes) != 1 || me.Runes[0] != 0x4e2d {
		t.Fatalf("erwartet MissingGlyphError für U+4E2D, erhalten %v", err)
	}
}

func TestLoadRejectsGarbage(t *testing.T) {
	for _, data := range [][]byte{nil, []byte("kein Font"), make([]byte, 64)} {
		if _, err := Load(data, 0); err == nil {
			t.Errorf("Load(%q) ohne Fehler", data)
		}
	}
}

func FuzzLoad(f *testing.F) {
	data, err := os.ReadFile("fonts/liberation/LiberationMono-Regular.ttf")
	if err != nil {
		f.Fatal(err)
	}
	f.Add(data[:4096])
	f.Add([]byte("OTTO\x00\x01\x00\x00"))
	f.Add([]byte("ttcf\x00\x01\x00\x00\x00\x00\x00\x01\x00\x00\x00\x10"))
	f.Fuzz(func(t *testing.T, b []byte) {
		face, err := Load(b, 0)
		if err != nil {
			return
		}
		face.Shape("Ab1 ß€", 10*areatree.Pt, ShapeOptions{})
	})
}
