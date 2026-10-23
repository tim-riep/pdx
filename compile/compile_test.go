package compile

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/fo/layout"
	"github.com/tim-riep/pdx/fo/tree"
	"github.com/tim-riep/pdx/internal/testutil"
	"github.com/tim-riep/pdx/template"
	"github.com/tim-riep/pdx/template/pkg"
)

const musterDir = "../testdata/templates/musterrechnung"

// render lays out FO without PDF. The root package pdx imports compile, so
// the tests lay out directly with fo/layout.
func render(fo []byte) (*areatree.Document, error) {
	root, err := tree.Parse(bytes.NewReader(fo))
	if err != nil {
		return nil, err
	}
	return layout.Layout(root, layout.Options{Resolver: layout.DirResolver{}})
}

func compileMuster(t testing.TB) (*pkg.Package, *Result) {
	t.Helper()
	p, err := pkg.ReadDir(musterDir)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Compile(p.Template, Options{})
	if err != nil {
		t.Fatal(err)
	}
	return p, res
}

// TestMusterrechnung checks XSLT and FO against golden files and lays out
// the result: at least three pages, carry-over and subtotal per page.
func TestMusterrechnung(t *testing.T) {
	p, res := compileMuster(t)
	testutil.Golden(t, "../testdata/golden/musterrechnung.xsl", res.XSLT)
	data, _ := p.File("samples/rechnung.xml")
	fo, err := res.Transform(data)
	if err != nil {
		t.Fatal(err)
	}
	testutil.Golden(t, "../testdata/golden/musterrechnung.fo", fo)
	r, err := render(fo)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(r.Pages); n < 3 {
		t.Fatalf("%d Seiten, erwartet mindestens 3", n)
	}
	pages := pageLines(r)
	// Page 1 ends with the subtotal, page 2 starts with it as the carry-over.
	p1, p2 := pages[0], pages[1]
	sub := after(t, p1, "Zwischensumme")
	if after(t, p2, "Übertrag") != sub {
		t.Errorf("Übertrag auf Seite 2 ist nicht die Zwischensumme %s von Seite 1", sub)
	}
	if contains(p1, "Übertrag") {
		t.Error("Seite 1 zeigt einen Übertrag")
	}
	if !contains(pages[len(pages)-1], "Gesamtbetrag: 80.992,49 €") {
		t.Error("Summen fehlen auf der letzten Seite")
	}
	if !contains(p1, "Seite 1 von 3") {
		t.Error("Seitenzahl fehlt")
	}
}

// pageLines returns the texts of the lines per page.
func pageLines(d *areatree.Document) [][]string {
	var pages [][]string
	for _, l := range strings.Split(string(areatree.Dump(d)), "\n") {
		t := strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(l, "  page "):
			pages = append(pages, nil)
		case strings.HasPrefix(t, "line ") && len(pages) > 0:
			pages[len(pages)-1] = append(pages[len(pages)-1], "")
		case strings.HasPrefix(t, "text ") && len(pages) > 0:
			p := pages[len(pages)-1]
			if i := strings.Index(t, ` "`); i >= 0 && len(p) > 0 {
				if q, err := strconv.QuotedPrefix(t[i+1:]); err == nil {
					s, _ := strconv.Unquote(q)
					p[len(p)-1] += s
				}
			}
		}
	}
	return pages
}

func contains(lines []string, s string) bool {
	for _, l := range lines {
		if l == s {
			return true
		}
	}
	return false
}

// after returns the line after the last line containing the text.
func after(t *testing.T, lines []string, s string) string {
	t.Helper()
	for i := len(lines) - 2; i >= 0; i-- {
		if lines[i] == s {
			return lines[i+1]
		}
	}
	t.Fatalf("%q fehlt in %q", s, lines)
	return ""
}

func TestDeterministic(t *testing.T) {
	_, a := compileMuster(t)
	_, b := compileMuster(t)
	if !bytes.Equal(a.XSLT, b.XSLT) {
		t.Fatal("zwei Läufe ergeben verschiedenes XSLT")
	}
}

func TestSourceMap(t *testing.T) {
	_, res := compileMuster(t)
	lines := strings.Split(string(res.XSLT), "\n")
	for i, l := range lines {
		if strings.Contains(l, `pdx:id="z-menge-f"`) {
			if id := res.IDAt(i + 1); id != "z-menge-f" {
				t.Fatalf("Zeile %d gehört zu %q, erwartet z-menge-f", i+1, id)
			}
			return
		}
	}
	t.Fatal("z-menge-f nicht im XSLT")
}

// errorIDs returns the template IDs of the errors.
func errorIDs(err error) []string {
	var es template.Errors
	if !errors.As(err, &es) {
		var e *template.Error
		if errors.As(err, &e) {
			return []string{e.ID}
		}
		return nil
	}
	var ids []string
	for _, e := range es {
		ids = append(ids, e.ID)
	}
	return ids
}

func TestRuntimeErrorID(t *testing.T) {
	p, res := compileMuster(t)
	data, _ := p.File("samples/rechnung.xml")
	bad := bytes.Replace(data, []byte(`unit="DAY">`), []byte(`unit="DAY">viel`), 1)
	if bytes.Equal(bad, data) {
		t.Fatal("Beispieldaten haben keine Menge mit unit=\"DAY\"")
	}
	_, err := res.Transform(bad)
	if err == nil {
		t.Fatal("keine Fehlermeldung bei ungültiger Menge")
	}
	if ids := errorIDs(err); len(ids) == 0 || ids[0] != "z-menge-f" {
		t.Fatalf("Fehler %v hat IDs %v, erwartet z-menge-f", err, ids)
	}
}

func TestWrongRoot(t *testing.T) {
	_, res := compileMuster(t)
	_, err := res.Transform([]byte(`<rechnung/>`))
	if err == nil || !strings.Contains(err.Error(), "inv:invoice") {
		t.Fatalf("Fehler %v nennt das erwartete Wurzelelement nicht", err)
	}
}

// mini builds a small template around the content of body.
func mini(t *testing.T, head, body string) *template.Template {
	t.Helper()
	src := `<template xmlns="https://github.com/tim-riep/pdx/ns/template/1" id="t" lang="de-DE" version="1" dataRoot="d">` + head +
		`<pageSet id="ps"><pageMaster id="pm"/></pageSet><body id="b">` + body + `</body></template>`
	tpl, err := template.Parse([]byte(src))
	if err != nil {
		t.Fatalf("Vorlage ungültig: %v", err)
	}
	return tpl
}

func TestCompileErrors(t *testing.T) {
	formats := `<formats><locale id="l" lang="de-DE"><pattern id="p" kind="date" name="datum" picture="[D]"/></locale></formats>`
	cases := []struct {
		name, head, body, id, msg string
	}{
		{"xpath", "", `<field id="f" bind="a/("/>`, "f", "XSLT"},
		{"format", formats, `<field id="f" bind="a" format="betrag"/>`, "f", "betrag"},
		{"ohne formats", "", `<field id="f" bind="a" format="betrag"/>`, "f", "formats"},
		{"text", "", `<text id="x" ref="fehlt"/>`, "x", "fehlt"},
		{"platzhalter", "", `<pageNumber id="n" pattern="{seite}"/>`, "n", "{seite}"},
		{"stil", "", `<text id="x" style="b">a</text>`, "x", "kein Stil"},
		{"carry", "", `<table id="tb" bind="r"><column id="c"/><rows id="rs"><row id="r"><cell id="z"><carryOver id="co" of="a"/></cell></row></rows></table>`, "co", "Tabellenkopf"},
		{"datum als summe", formats, `<table id="tb" bind="r"><column id="c"/><rows id="rs"><row id="r"><cell id="z"/></row></rows><footer id="ft"><row id="fr"><cell id="fz"><total id="to" of="a" format="datum"/></cell></row></footer></table>`, "to", "Zahlenformat"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Compile(mini(t, tc.head, tc.body), Options{})
			if err == nil {
				t.Fatal("kein Fehler")
			}
			ids := errorIDs(err)
			if len(ids) == 0 || ids[0] != tc.id || !strings.Contains(err.Error(), tc.msg) {
				t.Fatalf("Fehler %q mit IDs %v, erwartet ID %s und %q", err, ids, tc.id, tc.msg)
			}
		})
	}
}

func TestGroups(t *testing.T) {
	tpl := mini(t, `<formats><locale id="l" lang="de-DE" decimalSeparator="," groupingSeparator="."><pattern id="p" kind="number" name="n" picture="0,00"/></locale></formats>`,
		`<table id="tb" bind="i">
		  <column id="c1"/><column id="c2"/>
		  <group id="g" by="@k">
		    <header id="gh"><row id="ghr"><cell id="ghc" colspan="2"><text id="ght">Gruppe <field id="ghf" bind="@k"/></text></cell></row></header>
		    <footer id="gf"><row id="gfr"><cell id="gfc"><text id="gft">Summe</text></cell><cell id="gfs"><total id="gt" of="@v" format="n"/></cell></row></footer>
		  </group>
		  <rows id="rs"><row id="r"><cell id="z1"><field id="f1" bind="@n"/></cell><cell id="z2"><field id="f2" bind="@v" format="n"/></cell></row></rows>
		  <footer id="ft"><row id="fr"><cell id="fc"><text id="fct">Gesamt</text></cell><cell id="fs"><total id="all" of="@v" format="n"/></cell></row></footer>
		</table>`)
	res, err := Compile(tpl, Options{})
	if err != nil {
		t.Fatal(err)
	}
	data := `<d><i k="A" n="1" v="1.5"/><i k="B" n="2" v="2"/><i k="A" n="3" v="0.25"/></d>`
	fo, err := res.Transform([]byte(data))
	if err != nil {
		t.Fatalf("%v\n%s", err, res.XSLT)
	}
	r, err := render(fo)
	if err != nil {
		t.Fatalf("%v\n%s", err, fo)
	}
	var got []string
	for _, l := range pageLines(r)[0] {
		if l != "" {
			got = append(got, l)
		}
	}
	want := []string{"Gruppe A", "1", "1,50", "3", "0,25", "Summe", "1,75", "Gruppe B", "2", "2,00", "Summe", "2,00", "Gesamt", "3,75"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("Zeilen\n%v\nerwartet\n%v", got, want)
	}
}

func TestDataAttachment(t *testing.T) {
	_, res := compileMuster(t)
	if len(res.Attachments) != 1 || res.Attachments[0].Name != "rechnung.xml" || res.Attachments[0].Relationship != "Alternative" {
		t.Fatalf("Anhänge %+v", res.Attachments)
	}
}

func FuzzCompile(f *testing.F) {
	data, err := os.ReadFile(filepath.Join(musterDir, "template.xml"))
	if err != nil {
		f.Fatal(err)
	}
	f.Add(data)
	f.Fuzz(func(t *testing.T, b []byte) {
		tpl, err := template.Parse(b)
		if err != nil {
			return
		}
		// Valid templates yield XSLT or errors with a message, never a
		// crash, and the same result twice.
		r1, err1 := Compile(tpl, Options{})
		r2, err2 := Compile(tpl, Options{})
		if (err1 == nil) != (err2 == nil) {
			t.Fatal("Ergebnis nicht deterministisch")
		}
		if err1 == nil && !bytes.Equal(r1.XSLT, r2.XSLT) {
			t.Fatal("XSLT nicht deterministisch")
		}
	})
}

func TestInlines(t *testing.T) {
	tpl := mini(t, "", `<text id="x">Erste<br id="u"/>Zweite <span id="s" href="https://pdx.example/" fontWeight="bold">Link</span> <field id="f" bind="@w" default="leer"/></text><pageNumber id="n" pattern="{current}/{total}"/>`)
	res, err := Compile(tpl, Options{})
	if err != nil {
		t.Fatal(err)
	}
	fo, err := res.Transform([]byte(`<d w=" "/>`))
	if err != nil {
		t.Fatal(err)
	}
	r, err := render(fo)
	if err != nil {
		t.Fatalf("%v\n%s", err, fo)
	}
	got := strings.Join(pageLines(r)[0], "|")
	if got != "Erste|Zweite Link leer|1/1" {
		t.Fatalf("Zeilen %q", got)
	}
	if !bytes.Contains(areatree.Dump(r), []byte(`uri="https://pdx.example/"`)) {
		t.Error("Link fehlt")
	}
}

// TestFormular checks the XSLT of the form: field types, options from the
// enumeration and the context path for writing back.
func TestFormular(t *testing.T) {
	p, err := pkg.ReadDir("../testdata/templates/formular")
	if err != nil {
		t.Fatal(err)
	}
	res, err := Compile(p.Template, Options{Enums: map[string][]string{"anrede": {"Frau", "Herr", "divers"}, "tarif": {"Basis", "Komfort", "Premium"}}})
	if err != nil {
		t.Fatal(err)
	}
	testutil.Golden(t, "../testdata/golden/formular.xsl", res.XSLT)
	data, _ := p.File("samples/antrag.xml")
	fo, err := res.Transform(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`type="radio" name="anrede" label="Anrede" export="Frau" value="Frau"`,
		`type="text" name="name" label="Name" max-length="40" required="true" value="Erika Musterfrau"`,
		`context="/Q{https://github.com/tim-riep/pdx/ns/examples/antrag}antrag[1]/Q{https://github.com/tim-riep/pdx/ns/examples/antrag}personen[1]/Q{https://github.com/tim-riep/pdx/ns/examples/antrag}person[2]"`,
		`<pdx:option value="Komfort" label="Komfort"/>`,
		`type="signature" name="unterschrift"`,
	} {
		if !bytes.Contains(fo, []byte(want)) {
			t.Errorf("%q fehlt im FO", want)
		}
	}
	// Without options and enumeration the radio button group is an error.
	if _, err := Compile(p.Template, Options{}); err == nil || !strings.Contains(err.Error(), "[anrede]") {
		t.Errorf("Optionsfeldgruppe ohne Optionen: %v", err)
	}
}
