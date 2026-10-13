package layout

import (
	"strings"
	"testing"

	"github.com/tim-riep/pdx/areatree"
)

// pageTexts liefert je Seite die Texte der Zeilen im Fluss.
func pageTexts(doc *areatree.Document) [][]string {
	var out [][]string
	for _, p := range doc.Pages {
		var lines []string
		areatree.Walk(p.Areas[:1], func(a areatree.Area) bool {
			if c, ok := a.(*areatree.Container); ok && c.Kind == areatree.KindLine {
				var b strings.Builder
				areatree.Walk(c.Children, func(x areatree.Area) bool {
					if g, ok := x.(*areatree.GlyphRun); ok {
						for _, gl := range g.Glyphs {
							b.WriteString(gl.Text)
						}
					}
					return true
				})
				lines = append(lines, strings.TrimSpace(b.String()))
				return false
			}
			return true
		})
		out = append(out, lines)
	}
	return out
}

func blocks(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString(`<fo:block>Z` + itoa(i+1) + `</fo:block>`)
	}
	return b.String()
}

func TestKeepWithNext(t *testing.T) {
	// 10 Zeilen passen auf eine Seite. Die Überschrift an Position 10 muss
	// mit dem folgenden Block auf die nächste Seite.
	doc := layoutFO(t, smallPage(blocks(9)+`<fo:block keep-with-next="always">Titel</fo:block><fo:block>Text</fo:block>`))
	pages := pageTexts(doc)
	if len(pages) != 2 || pages[1][0] != "Titel" {
		t.Fatalf("Seiten: %v", pages)
	}
}

func TestKeepWithPrevious(t *testing.T) {
	doc := layoutFO(t, smallPage(blocks(10)+`<fo:block keep-with-previous="always">Nachsatz</fo:block>`))
	pages := pageTexts(doc)
	if len(pages) != 2 || pages[1][0] != "Z10" {
		t.Fatalf("Seiten: %v", pages)
	}
}

func TestKeepTogetherBlock(t *testing.T) {
	doc := layoutFO(t, smallPage(blocks(8)+`<fo:block keep-together.within-page="always"><fo:block>A</fo:block><fo:block>B</fo:block><fo:block>C</fo:block></fo:block>`))
	pages := pageTexts(doc)
	if len(pages) != 2 || pages[1][0] != "A" || len(pages[0]) != 8 {
		t.Fatalf("Seiten: %v", pages)
	}
}

func TestBreakBeforeAndOddPage(t *testing.T) {
	doc := layoutFO(t, smallPage(`<fo:block>Eins</fo:block><fo:block break-before="page">Zwei</fo:block><fo:block break-before="odd-page">Drei</fo:block>`))
	pages := pageTexts(doc)
	// Seite 1: Eins, Seite 2: Zwei, Seite 3 ist ungerade, also keine Leerseite.
	if len(pages) != 3 || pages[1][0] != "Zwei" || pages[2][0] != "Drei" {
		t.Fatalf("Seiten: %v", pages)
	}
	doc = layoutFO(t, smallPage(`<fo:block break-after="page">Eins</fo:block><fo:block break-before="even-page">Zwei</fo:block>`))
	if pages = pageTexts(doc); len(pages) != 2 || pages[1][0] != "Zwei" {
		t.Fatalf("even-page: %v", pages)
	}
	doc = layoutFO(t, smallPage(`<fo:block break-after="page">Eins</fo:block><fo:block break-before="odd-page">Drei</fo:block>`))
	if pages = pageTexts(doc); len(pages) != 3 || len(pages[1]) != 0 || pages[2][0] != "Drei" {
		t.Fatalf("odd-page mit Leerseite: %v", pages)
	}
}

func TestWidowsOrphans(t *testing.T) {
	// 9 Einzelzeilen füllen die Seite fast. Ein Absatz mit 4 Zeilen würde
	// mit einer Zeile beginnen (Waise), er muss ganz auf die nächste Seite
	// oder mindestens 2 Zeilen mitnehmen.
	para := `<fo:block>` + strings.Repeat("Wort ", 120) + `</fo:block>`
	doc := layoutFO(t, smallPage(blocks(9)+para))
	pages := pageTexts(doc)
	if len(pages[0]) == 10 {
		t.Fatalf("Waise auf Seite 1: %v", pages[0])
	}
	// Mit orphans="1" darf die erste Zeile allein bleiben.
	doc = layoutFO(t, smallPage(blocks(9)+`<fo:block orphans="1">`+strings.Repeat("Wort ", 120)+`</fo:block>`))
	if pages = pageTexts(doc); len(pages[0]) != 10 {
		t.Fatalf("orphans=1: %d Zeilen auf Seite 1", len(pages[0]))
	}
}
