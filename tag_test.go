package pdx

import (
	"fmt"
	"strings"
	"testing"

	"github.com/tim-riep/pdx/areatree"
)

// containers sammelt alle Container eines Dokuments.
func containers(doc *areatree.Document) []*areatree.Container {
	var out []*areatree.Container
	for _, p := range doc.Pages {
		areatree.Walk(p.Areas, func(a areatree.Area) bool {
			if c, ok := a.(*areatree.Container); ok {
				out = append(out, c)
			}
			return true
		})
	}
	return out
}

// Die Angaben für Tagged PDF aus dem FO stehen an den Bereichen: Rolle,
// Alternativtext, Sprache, Lesereihenfolge, Artefakt, Bezug von Kopfzellen,
// Zeilen und Wiederholungen. Inline-Elemente stehen in Document.Inline.
func TestLayoutTags(t *testing.T) {
	var rows strings.Builder
	for i := 0; i < 70; i++ {
		fmt.Fprintf(&rows, `<fo:table-row><fo:table-cell><fo:block>Position %d</fo:block></fo:table-cell></fo:table-row>`, i)
	}
	fo := `<fo:root xmlns:fo="http://www.w3.org/1999/XSL/Format" xmlns:pdx="https://pdx.example/ns/pdx/1.0" xml:lang="de-DE">
<fo:layout-master-set><fo:simple-page-master master-name="A4" page-width="210mm" page-height="297mm" margin="20mm"><fo:region-body margin-top="20mm"/><fo:region-before extent="15mm"/></fo:simple-page-master></fo:layout-master-set>
<fo:page-sequence master-reference="A4"><fo:static-content flow-name="xsl-region-before"><fo:block pdx:id="kopf">Kopfzeile</fo:block>
<fo:block-container pdx:id="anschrift" absolute-position="fixed" left="20mm" top="5mm" width="80mm" pdx:artifact="false"><fo:block-container pdx:id="zeile" absolute-position="absolute" left="0mm" top="0mm" width="80mm"><fo:block>Muster AG</fo:block></fo:block-container></fo:block-container>
</fo:static-content><fo:flow flow-name="xsl-region-body">
<fo:block pdx:id="h" role="H1">Titel</fo:block>
<fo:block pdx:id="en" xml:lang="en-GB">English <fo:inline pdx:id="ref" role="Reference" xml:lang="fr">voir</fo:inline> <fo:basic-link pdx:id="lnk" external-destination="url('https://example.org')">Link</fo:basic-link></fo:block>
<fo:block pdx:id="bild" pdx:alt="Firmenlogo">Logo</fo:block>
<fo:block pdx:id="deko" pdx:artifact="true">Deko</fo:block>
<fo:block-container pdx:id="pos" absolute-position="absolute" left="0mm" top="200mm" width="50mm" pdx:reading-order="2"><fo:block>Fenster</fo:block></fo:block-container>
<fo:table pdx:id="tb" role="Table"><fo:table-column column-width="100mm"/>
<fo:table-header><fo:table-row><fo:table-cell pdx:id="k" pdx:scope="Column"><fo:block>Kopf</fo:block></fo:table-cell></fo:table-row></fo:table-header>
<fo:table-body>` + rows.String() + `</fo:table-body></fo:table>
<fo:block-container width="25mm"><fo:block hyphenate="true" language="de" text-align="justify">Die Donaudampfschifffahrtsgesellschaft verkauft Kapitänsmützenabzeichen</fo:block></fo:block-container>
</fo:flow></fo:page-sequence></fo:root>`
	doc, err := Layout(strings.NewReader(fo), Options{})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]*areatree.Container{}
	var heads []*areatree.Container
	for _, c := range containers(doc) {
		if c.ID != "" && byID[c.ID] == nil {
			byID[c.ID] = c
		}
		if c.ID == "k" && strings.HasPrefix(c.Source, "fo:table-cell") {
			heads = append(heads, c)
		}
	}
	if byID["h"].Role != "H1" {
		t.Errorf("Rolle %q", byID["h"].Role)
	}
	if tg := byID["en"].Tag; tg == nil || tg.Lang != "en-GB" {
		t.Errorf("Sprache %+v", tg)
	}
	if tg := byID["bild"].Tag; tg == nil || tg.Alt != "Firmenlogo" {
		t.Errorf("Alternativtext %+v", tg)
	}
	if tg := byID["deko"].Tag; tg == nil || !tg.Artifact {
		t.Errorf("Artefakt %+v", tg)
	}
	if tg := byID["pos"].Tag; tg == nil || tg.Order != 2 {
		t.Errorf("Lesereihenfolge %+v", tg)
	}
	// Der Kopf steht auf jeder Seite, nur der erste ist kein Repeat.
	if len(heads) < 2 {
		t.Fatalf("Kopf %d-mal", len(heads))
	}
	for i, h := range heads {
		if h.Tag == nil || h.Tag.Scope != "Column" || h.Tag.Section != "header" || h.Tag.Repeat != (i > 0) {
			t.Errorf("Kopf %d: %+v", i, h.Tag)
		}
		if i > 0 && h.Tag.Row != heads[0].Tag.Row {
			t.Errorf("Wiederholung mit anderer Zeilennummer")
		}
	}
	if in := doc.Inline["ref"]; in.Role != "Reference" || in.Tag.Lang != "fr" {
		t.Errorf("Inline %+v", in)
	}
	if doc.Inline["lnk"].Role != "Link" {
		t.Errorf("Verweis %+v", doc.Inline["lnk"])
	}
	// Kopf ist statisch, die Anschrift ausdrücklich Inhalt, die innere Zeile
	// kennt ihren positionierten Vorfahren.
	if tg := byID["kopf"].Tag; tg == nil || !tg.Static || tg.Content {
		t.Errorf("Kopf %+v", tg)
	}
	if tg := byID["anschrift"].Tag; tg == nil || !tg.Static || !tg.Content {
		t.Errorf("Anschrift %+v", tg)
	}
	if tg := byID["zeile"].Tag; tg == nil || !strings.HasPrefix(tg.Parent, "fo:block-container@") {
		t.Errorf("Zeile %+v", tg)
	}
	// Die Tabelle geht über mehrere Seiten, ihre Fortsetzung ist markiert.
	var tables []*areatree.Container
	for _, c := range containers(doc) {
		if c.ID == "tb" && strings.HasPrefix(c.Source, "fo:table@") {
			tables = append(tables, c)
		}
	}
	if len(tables) < 2 || (tables[0].Tag != nil && tables[0].Tag.Continued) || tables[1].Tag == nil || !tables[1].Tag.Continued {
		t.Errorf("Fortsetzung der Tabelle: %d Fragmente", len(tables))
	}
	// Automatische Trennstriche sind markiert.
	hyph := 0
	for _, p := range doc.Pages {
		areatree.Walk(p.Areas, func(a areatree.Area) bool {
			if r, ok := a.(*areatree.GlyphRun); ok {
				for _, g := range r.Glyphs {
					if g.Hyphen {
						hyph++
					}
				}
			}
			return true
		})
	}
	if hyph == 0 {
		t.Error("kein Trennstrich markiert")
	}
}
