package layout

import (
	"strings"
	"testing"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/fo/tree"
)

func layoutFO(t *testing.T, fo string) *areatree.Document {
	t.Helper()
	root, err := tree.ParseBytes([]byte(fo))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := Layout(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// smallPage liefert ein FO mit kleiner Seite, damit Umbrüche schnell nötig sind.
func smallPage(body string) string {
	return `<fo:root xmlns:fo="http://www.w3.org/1999/XSL/Format">
<fo:layout-master-set><fo:simple-page-master master-name="p" page-width="200pt" page-height="120pt">
<fo:region-body/></fo:simple-page-master></fo:layout-master-set>
<fo:page-sequence master-reference="p"><fo:flow flow-name="xsl-region-body" font-size="10pt" line-height="12pt">` +
		body + `</fo:flow></fo:page-sequence></fo:root>`
}

func tallRow(keep string) string {
	lines := strings.Repeat("<fo:block>Zeile</fo:block>", 15)
	return smallPage(`<fo:block>Davor</fo:block><fo:table><fo:table-body><fo:table-row ` + keep + `>
<fo:table-cell><fo:block>Links</fo:block></fo:table-cell>
<fo:table-cell>` + lines + `</fo:table-cell></fo:table-row></fo:table-body></fo:table>`)
}

func countLines(p *areatree.Page) int {
	n := 0
	areatree.Walk(p.Areas, func(a areatree.Area) bool {
		if c, ok := a.(*areatree.Container); ok && c.Kind == areatree.KindLine {
			n++
		}
		return true
	})
	return n
}

func TestRowSplits(t *testing.T) {
	doc := layoutFO(t, tallRow(""))
	if len(doc.Pages) != 2 {
		t.Fatalf("%d Seiten, erwartet 2", len(doc.Pages))
	}
	// Seite 1 (120pt, Zeilen à 12pt): Davor, daneben Links und 9 Zeilen der
	// hohen Zelle. Seite 2: die übrigen 6 Zeilen.
	if got := countLines(doc.Pages[0]); got != 11 {
		t.Errorf("Seite 1 hat %d Zeilen, erwartet 11", got)
	}
	if got := countLines(doc.Pages[1]); got != 6 {
		t.Errorf("Seite 2 hat %d Zeilen, erwartet 6", got)
	}
}

func TestRowKeepTogether(t *testing.T) {
	doc := layoutFO(t, tallRow(`keep-together.within-page="always"`))
	// Die Zeile passt auf keine Seite, sie wird trotzdem nicht vor der Seite
	// mit "Davor" geteilt, sondern beginnt auf Seite 2.
	if got := countLines(doc.Pages[0]); got != 1 {
		t.Errorf("Seite 1 hat %d Zeilen, erwartet nur Davor", got)
	}
}

func TestHeaderRepeats(t *testing.T) {
	var rows strings.Builder
	for i := 0; i < 20; i++ {
		rows.WriteString(`<fo:table-row><fo:table-cell><fo:block>R</fo:block></fo:table-cell></fo:table-row>`)
	}
	doc := layoutFO(t, smallPage(`<fo:table><fo:table-header><fo:table-row><fo:table-cell><fo:block>Kopf</fo:block></fo:table-cell></fo:table-row></fo:table-header>
<fo:table-footer><fo:table-row><fo:table-cell><fo:block>Fuss</fo:block></fo:table-cell></fo:table-row></fo:table-footer>
<fo:table-body>`+rows.String()+`</fo:table-body></fo:table>`))
	if len(doc.Pages) < 3 {
		t.Fatalf("%d Seiten", len(doc.Pages))
	}
	for _, p := range doc.Pages {
		dump := string(areatree.Dump(&areatree.Document{Fonts: doc.Fonts, Pages: []*areatree.Page{p}}))
		if !strings.Contains(dump, `"Kopf"`) || !strings.Contains(dump, `"Fuss"`) {
			t.Errorf("Seite %d ohne Kopf oder Fuß", p.Number)
		}
		if got := countLines(p); got > 10 {
			t.Errorf("Seite %d hat %d Zeilen, mehr als passen", p.Number, got)
		}
	}
}

// TestRowResumeAfterWidthChange prüft, dass ein Listenpunkt, der beim
// Wechsel zu einer breiteren Seite umbricht, keinen Text verliert und keinen
// doppelt setzt.
func TestRowResumeAfterWidthChange(t *testing.T) {
	var words []string
	for i := 1; i <= 80; i++ {
		words = append(words, "w"+itoa(i))
	}
	fo := `<fo:root xmlns:fo="http://www.w3.org/1999/XSL/Format">
<fo:layout-master-set>
<fo:simple-page-master master-name="schmal" page-width="150pt" page-height="72pt"><fo:region-body/></fo:simple-page-master>
<fo:simple-page-master master-name="breit" page-width="300pt" page-height="72pt"><fo:region-body/></fo:simple-page-master>
<fo:page-sequence-master master-name="folge"><fo:single-page-master-reference master-reference="schmal"/>
<fo:repeatable-page-master-reference master-reference="breit"/></fo:page-sequence-master>
</fo:layout-master-set>
<fo:page-sequence master-reference="folge"><fo:flow flow-name="xsl-region-body" font-size="10pt" line-height="12pt">
<fo:list-block provisional-distance-between-starts="20pt"><fo:list-item>
<fo:list-item-label end-indent="label-end()"><fo:block>*</fo:block></fo:list-item-label>
<fo:list-item-body start-indent="body-start()"><fo:block>` + strings.Join(words, " ") + `</fo:block></fo:list-item-body>
</fo:list-item></fo:list-block></fo:flow></fo:page-sequence></fo:root>`
	doc := layoutFO(t, fo)
	if len(doc.Pages) < 2 {
		t.Fatalf("%d Seiten, erwartet mindestens 2", len(doc.Pages))
	}
	var got []string
	for _, lines := range pageTexts(doc) {
		for _, l := range lines {
			for _, w := range strings.Fields(l) {
				if strings.HasPrefix(w, "w") {
					got = append(got, w)
				}
			}
		}
	}
	if strings.Join(got, " ") != strings.Join(words, " ") {
		t.Errorf("Wörter nach dem Breitenwechsel:\n%s", strings.Join(got, " "))
	}
}
