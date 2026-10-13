package render

import (
	"bytes"
	"testing"
	"time"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/internal/testutil"
)

var testDate = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func shapes() *areatree.Document {
	red := areatree.RGBColor(0.8, 0.1, 0.1)
	grey := areatree.GrayColor(0.9)
	solid := func(w areatree.Length) areatree.Border {
		return areatree.Border{Width: w, Style: areatree.Solid, Color: areatree.Black}
	}
	return &areatree.Document{
		Meta: areatree.Metadata{Title: "Formen", Lang: "de-DE"},
		Pages: []*areatree.Page{{
			Number: 1, Width: areatree.Points(595.276), Height: areatree.Points(841.89),
			Areas: []areatree.Area{
				&areatree.Container{Kind: areatree.KindBlock, Rect: areatree.Rect{X: 72000, Y: 72000, W: 200000, H: 50000},
					Background: &grey, Border: [4]areatree.Border{solid(2000), solid(1000), solid(2000), solid(1000)}},
				&areatree.Container{Kind: areatree.KindViewport, Rect: areatree.Rect{X: 72000, Y: 150000, W: 100000, H: 100000},
					Border: [4]areatree.Border{
						{Width: 1000, Style: areatree.Dashed, Color: red}, {Width: 1000, Style: areatree.Dotted, Color: red},
						{Width: 3000, Style: areatree.Double, Color: red}, {}},
					Children: []areatree.Area{&areatree.Box{Rect: areatree.Rect{X: 50000, Y: 130000, W: 100000, H: 100000}, Fill: &red}}},
				&areatree.Line{From: areatree.Point{X: 72000, Y: 300000}, To: areatree.Point{X: 523000, Y: 300000},
					Width: 500, Style: areatree.Solid, Color: areatree.Black},
			},
		}},
	}
}

func TestShapesGolden(t *testing.T) {
	b, err := Render(shapes(), Options{CreationDate: testDate})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testutil.CheckStructure(b); err != nil {
		t.Fatal(err)
	}
	testutil.Golden(t, "testdata/shapes.pdf", b)
	b2, _ := Render(shapes(), Options{CreationDate: testDate})
	if !bytes.Equal(b, b2) {
		t.Fatal("nicht reproduzierbar")
	}
}

func TestNeedsDate(t *testing.T) {
	if _, err := Render(shapes(), Options{}); err == nil {
		t.Fatal("fehlendes Erstellungsdatum nicht erkannt")
	}
}
