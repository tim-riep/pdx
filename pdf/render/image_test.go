package render

import (
	"errors"
	"os"
	"testing"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/image"
	"github.com/tim-riep/pdx/internal/testutil"
	"github.com/tim-riep/pdx/pdf"
)

func imageDoc(t testing.TB, files ...string) *areatree.Document {
	t.Helper()
	doc := &areatree.Document{Meta: areatree.Metadata{Title: "Bilder"}}
	page := &areatree.Page{Number: 1, Width: areatree.Points(595.276), Height: areatree.Points(841.89)}
	y := 72 * areatree.Pt
	for i, f := range files {
		data, err := os.ReadFile("../../testdata/images/" + f)
		if err != nil {
			t.Fatal(err)
		}
		im, err := image.Load(data)
		if err != nil {
			t.Fatal(err)
		}
		doc.Images = append(doc.Images, areatree.ImageRef{Source: im})
		w, h := im.IntrinsicSize()
		page.Areas = append(page.Areas, &areatree.Image{Rect: areatree.Rect{X: 72 * areatree.Pt, Y: y, W: w, H: h}, Image: areatree.ImageID(i)})
		// Dasselbe Bild ein zweites Mal halb so groß, es wird nur einmal eingebettet.
		page.Areas = append(page.Areas, &areatree.Image{Rect: areatree.Rect{X: 72*areatree.Pt + w + 20*areatree.Pt, Y: y, W: w / 2, H: h / 2}, Image: areatree.ImageID(i)})
		y += h + 20*areatree.Pt
	}
	doc.Pages = []*areatree.Page{page}
	return doc
}

func TestImagesPDFA(t *testing.T) {
	doc := imageDoc(t, "foto.jpg", "logo-alpha.png", "verlauf-grau.png")
	b, err := Render(doc, Options{Profile: pdfa3b(t), CreationDate: testDate})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testutil.CheckStructure(b); err != nil {
		t.Fatal(err)
	}
	testutil.Golden(t, "testdata/images.pdf", b)
	testutil.Conform(t, "images.pdf", b, "pdfa-3b")
}

// noTransparency verbietet Transparenz, wie später PDF/A-1.
type noTransparency struct{ denyAttach }

func (noTransparency) Name() string { return "ohne-transparenz" }
func (n noTransparency) Allow(f pdf.Feature) error {
	if f == pdf.Transparency {
		return &pdf.FeatureError{Feature: f, Profile: n.Name(), Reason: "Test"}
	}
	return nil
}

func TestAlphaNeedsTransparency(t *testing.T) {
	_, err := Render(imageDoc(t, "logo-alpha.png"), Options{Profile: noTransparency{}, CreationDate: testDate})
	var fe *pdf.FeatureError
	if !errors.As(err, &fe) || fe.Feature != pdf.Transparency {
		t.Fatalf("erwartet Verweigerung der Transparenz, erhalten %v", err)
	}
	if _, err := Render(imageDoc(t, "foto.jpg"), Options{Profile: noTransparency{}, CreationDate: testDate}); err != nil {
		t.Fatalf("JPEG ohne Alpha muss gehen: %v", err)
	}
}
