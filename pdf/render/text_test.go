package render

import (
	"bytes"
	"os"
	"testing"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/font"
	"github.com/tim-riep/pdx/internal/testutil"
)

// textDoc sets a few lines in Liberation Sans (TrueType) and Source Serif 4
// (OpenType/CFF) without layout engine.
func textDoc(t testing.TB) *areatree.Document {
	t.Helper()
	reg := font.NewRegistry()
	sans, err := reg.Lookup("Helvetica", 400, false)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../../testdata/fonts/source-serif-4/SourceSerif4-Regular.otf")
	if err != nil {
		t.Fatal(err)
	}
	serif, err := font.Load(data, 0)
	if err != nil {
		t.Fatal(err)
	}
	doc := &areatree.Document{Fonts: []areatree.FontRef{{Source: sans}, {Source: serif}}}
	page := &areatree.Page{Number: 1, Width: areatree.Points(595.276), Height: areatree.Points(841.89)}
	y := 72 * areatree.Pt
	lines := []struct {
		id   areatree.FontID
		text string
	}{
		{0, "Grüße aus Köln: 12,50 € für „Straße“ – AVATAR"},
		{1, "Offizielle Effizienz: ffi fl ß Ä Ö Ü € … ‚x‘"},
		{1, "Kerning: AV To Ty Wa"},
	}
	for _, l := range lines {
		face := doc.Fonts[l.id].Source.(*font.Face)
		size := 14 * areatree.Pt
		gs, err := face.Shape(l.text, size, font.ShapeOptions{})
		if err != nil {
			t.Fatal(err)
		}
		asc, desc, _ := face.Metrics(size)
		page.Areas = append(page.Areas, &areatree.GlyphRun{Origin: areatree.Point{X: 72 * areatree.Pt, Y: y},
			Font: l.id, Size: size, Color: areatree.Black, Glyphs: gs, Ascent: asc, Descent: desc})
		y += 24 * areatree.Pt
	}
	doc.Pages = []*areatree.Page{page}
	return doc
}

func TestTextGolden(t *testing.T) {
	b, err := Render(textDoc(t), Options{CreationDate: testDate})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testutil.CheckStructure(b); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"/CIDFontType2", "/CIDFontType0", "/FontFile2", "/FontFile3", "/Identity-H", "/ToUnicode", "+LiberationSans", "+SourceSerif4-Regular"} {
		if !bytes.Contains(b, []byte(want)) {
			t.Errorf("%s fehlt in der Ausgabe", want)
		}
	}
	testutil.Golden(t, "testdata/text.pdf", b)
	b2, _ := Render(textDoc(t), Options{CreationDate: testDate})
	if !bytes.Equal(b, b2) {
		t.Fatal("Textausgabe nicht reproduzierbar")
	}
}
