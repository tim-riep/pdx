package areatree

import (
	"testing"

	"github.com/tim-riep/pdx/internal/testutil"
)

type fakeFont string

func (f fakeFont) PostScriptName() string { return string(f) }

type fakeImage struct{}

func (fakeImage) Key() string           { return "sha256:abc" }
func (fakeImage) PixelSize() (int, int) { return 640, 480 }

func TestLengthString(t *testing.T) {
	cases := map[Length]string{0: "0", 1000: "1", 1500: "1.5", 12: "0.012", -2250: "-2.25", 595276: "595.276"}
	for l, want := range cases {
		if got := l.String(); got != want {
			t.Errorf("%d.String() = %q, erwartet %q", int64(l), got, want)
		}
	}
	if Points(595.2756) != 595276 || Points(-1.0004) != -1000 {
		t.Error("Points rundet falsch")
	}
}

// Sample liefert einen kleinen Baum mit allen Bereichsarten.
func Sample() *Document {
	red := RGBColor(1, 0, 0)
	return &Document{
		Meta:        Metadata{Title: "Probe", Lang: "de-DE"},
		Fonts:       []FontRef{{Source: fakeFont("LiberationSans")}},
		Images:      []ImageRef{{Source: fakeImage{}}},
		Attachments: []Attachment{{Name: "a.xml", MIMEType: "text/xml", Relationship: Alternative, Data: []byte("<a/>")}},
		Pages: []*Page{{
			Number: 1, Width: Points(595.276), Height: Points(841.89),
			Areas: []Area{
				&Container{Rect: Rect{X: 56693, Y: 56693, W: 481890, H: 30000}, Kind: KindBlock, Background: &red,
					Border: [4]Border{Top: {Width: 500, Style: Solid, Color: Black}}, Source: "fo:block@3:7",
					Children: []Area{&Container{Rect: Rect{X: 56693, Y: 56693, W: 481890, H: 14400}, Kind: KindLine,
						Children: []Area{&GlyphRun{Origin: Point{X: 56693, Y: 67000}, Size: 12 * Pt, Color: Black,
							Ascent: 10860, Descent: 2544,
							Glyphs: []Glyph{{ID: 43, Advance: 8664, Text: "H"}, {ID: 1, Advance: 6672, XOffset: 10, Text: "a"}}}}}}},
				&Image{Rect: Rect{X: 0, Y: 100000, W: 64000, H: 48000}},
				&Line{From: Point{0, 0}, To: Point{100000, 0}, Width: 250, Style: Dashed, Color: GrayColor(0.5)},
				&Box{Rect: Rect{W: 1000, H: 1000}, Fill: &red, Stroke: &Border{Width: 100, Style: Solid}},
			},
		}},
	}
}

func TestDumpGolden(t *testing.T) {
	testutil.Golden(t, "testdata/sample.txt", Dump(Sample()))
}

func TestWalkAndBounds(t *testing.T) {
	n := 0
	Walk(Sample().Pages[0].Areas, func(Area) bool { n++; return true })
	if n != 6 {
		t.Errorf("Walk besuchte %d Bereiche, erwartet 6", n)
	}
	g := Sample().Pages[0].Areas[0].(*Container).Children[0].(*Container).Children[0].(*GlyphRun)
	if b := g.Bounds(); b.W != 15336 || b.Y != 67000-10860 || b.H != 13404 {
		t.Errorf("GlyphRun.Bounds = %+v", b)
	}
}
