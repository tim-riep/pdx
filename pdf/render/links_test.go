package render

import (
	"bytes"
	"testing"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/internal/testutil"
)

func TestLinksAndOutlines(t *testing.T) {
	doc := shapes()
	doc.Pages = append(doc.Pages, &areatree.Page{Number: 2, Width: doc.Pages[0].Width, Height: doc.Pages[0].Height})
	doc.Dests = map[string]areatree.Dest{"ziel": {Page: 1, At: areatree.Point{X: 72000, Y: 100000}}}
	doc.Pages[0].Areas = append(doc.Pages[0].Areas,
		&areatree.Link{Rect: areatree.Rect{X: 72000, Y: 400000, W: 100000, H: 14000}, Dest: "ziel"},
		&areatree.Link{Rect: areatree.Rect{X: 72000, Y: 420000, W: 100000, H: 14000}, URI: "https://example.org/"})
	doc.Bookmarks = []areatree.Bookmark{
		{Title: "Kapitel", Dest: "ziel", Open: true, Children: []areatree.Bookmark{{Title: "Unterpunkt", Dest: "ziel"}}},
		{Title: "Extern", URI: "https://example.org/"},
	}
	b, err := Render(doc, Options{Profile: pdfa3b(t), CreationDate: testDate})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"/Subtype /Link", "/F 4", "/S /URI", "/XYZ 72 741.89", "/Outlines", "/Title (Unterpunkt)", "/Count 3"} {
		if !bytes.Contains(b, []byte(want)) {
			t.Errorf("%q fehlt", want)
		}
	}
	testutil.Conform(t, "links.pdf", b, "pdfa-3b")
	// An unknown destination is an error.
	doc.Bookmarks[0].Dest = "fehlt"
	if _, err := Render(doc, Options{CreationDate: testDate}); err == nil {
		t.Error("unbekanntes Sprungziel nicht erkannt")
	}
}
