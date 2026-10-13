package fopcompare

import (
	"image"
	"image/color"
	"os"
	"testing"
)

func TestParseAreaTree(t *testing.T) {
	data, err := os.ReadFile("testdata/brief.at.xml")
	if err != nil {
		t.Fatal(err)
	}
	blocks, pages, err := ParseAreaTree(data)
	if err != nil {
		t.Fatal(err)
	}
	if pages != 1 || len(blocks) < 10 {
		t.Fatalf("%d Seiten, %d Blöcke", pages, len(blocks))
	}
	b := blocks[0]
	// Erster Block: Region beginnt bei 127.559 pt, keine Abstände davor.
	if b.FirstLine != "Beispiel GmbH · Hauptstraße 1 · 50667 Köln" || b.Top != 127.559 || b.Lines != 1 {
		t.Errorf("erster Block: %+v", b)
	}
	if blocks[1].Top != 127.559+16.969 {
		t.Errorf("zweiter Block beginnt bei %v", blocks[1].Top)
	}
}

func TestParseBBox(t *testing.T) {
	data, err := os.ReadFile("testdata/brief.bbox.html")
	if err != nil {
		t.Fatal(err)
	}
	lines, pages, err := parseBBox(data)
	if err != nil {
		t.Fatal(err)
	}
	if pages != 1 || len(lines) < 10 || lines[0].Text != "Beispiel GmbH · Hauptstraße 1 · 50667 Köln" {
		t.Fatalf("%d Seiten, %d Zeilen, erste %+v", pages, len(lines), lines[0])
	}
}

func TestLCS(t *testing.T) {
	a := []string{"a", "b", "c", "d"}
	b := []string{"a", "c", "x", "d"}
	got := lcs(len(a), len(b), func(i, j int) bool { return a[i] == b[j] })
	if len(got) != 3 || got[1] != [2]int{2, 1} {
		t.Errorf("lcs = %v", got)
	}
}

func TestCompareLinesTolerance(t *testing.T) {
	r := &Report{PagesPDX: 1, PagesFOP: 1, Tol: Default}
	pdx := []cmpLine{{Page: 1, Text: "A", X: 10, Y: 10, HasX: true}, {Page: 1, Text: "B", X: 10, Y: 30, HasX: true}, {Page: 1, Text: "nur pdx", Y: 50}}
	fop := []cmpLine{{Page: 1, Text: "B", X: 10, Y: 31.5, HasX: true}, {Page: 1, Text: "A", X: 10.9, Y: 9.2, HasX: true}}
	compareLines(r, pdx, fop, Default)
	if r.LinesCompared != 2 || len(r.Lines) != 2 {
		t.Fatalf("verglichen %d, Abweichungen %+v", r.LinesCompared, r.Lines)
	}
	if r.Lines[0].PDX != "B" || r.Lines[1].FOP != "" {
		t.Errorf("Abweichungen %+v", r.Lines)
	}
}

func TestPixelDiff(t *testing.T) {
	a := image.NewGray(image.Rect(0, 0, 100, 100))
	b := image.NewGray(image.Rect(0, 0, 100, 100))
	for i := range a.Pix {
		a.Pix[i], b.Pix[i] = 255, 255
	}
	// Ein schwarzer Strich, in b um 1 px verschoben: gilt als gleich.
	for y := 10; y < 90; y++ {
		a.SetGray(50, y, color.Gray{0})
		b.SetGray(51, y, color.Gray{0})
	}
	if r := PixelDiff(a, b, Default); r != 0 {
		t.Errorf("Verschiebung um 1 px ergibt %v", r)
	}
	// Um 3 px verschoben: 2 * 80 Pixel weichen ab.
	b2 := image.NewGray(b.Rect)
	copy(b2.Pix, a.Pix)
	for y := 10; y < 90; y++ {
		b2.SetGray(50, y, color.Gray{255})
		b2.SetGray(53, y, color.Gray{0})
	}
	if r := PixelDiff(a, b2, Default); r != 160.0/10000 {
		t.Errorf("Verschiebung um 3 px ergibt %v", r)
	}
}
