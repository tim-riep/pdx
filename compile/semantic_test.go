package compile

import (
	"strings"
	"testing"
)

// Die Angaben für Tagged PDF gelangen als FO-Attribute an alle Elemente,
// der Alternativtext auch aus den Daten.
func TestSemanticAttributes(t *testing.T) {
	tpl := mini(t, "", `<text id="h" role="H2" lang="en-GB">Total</text>`+
		`<barcode id="qr" type="qr" value="x" altBind="concat('Betrag ', n)"/>`+
		`<subform id="deko" layout="flowed" artifact="true"><text id="t2" alt="Logo">x</text></subform>`+
		`<text id="t3"><span id="s1" role="Reference" lang="fr">voir</span> <field id="f1" bind="n" alt="Zahl"/></text>`+
		`<table id="tb" bind="r"><column id="c1"/><column id="c2"/>`+
		`<header id="hd"><row id="hr"><cell id="k1" scope="Column"><text id="kt1">Name</text></cell><cell id="k2"><text id="kt2">Wert</text></cell></row>`+
		`<row id="hu"><cell id="u1"/><cell id="u2"><carryOver id="co" of="v"/></cell></row></header>`+
		`<rows id="rs"><row id="r1"><cell id="z1" scope="Row"><field id="zf" bind="."/></cell><cell id="z2"/></row></rows>`+
		`<footer id="ft"><row id="fr"><cell id="s"/><cell id="ss"><subtotal id="st" of="v"/></cell></row>`+
		`<row id="ftot"><cell id="tc"/><cell id="tt"><total id="to" of="v"/></cell></row></footer></table>`)
	res, err := Compile(tpl, Options{})
	if err != nil {
		t.Fatal(err)
	}
	fo, err := res.Transform([]byte(`<d><n>5</n><r><v>1</v></r><r><v>2</v></r></d>`))
	if err != nil {
		t.Fatal(err)
	}
	s := string(fo)
	for _, want := range []string{
		`pdx:id="h" role="H2" xml:lang="en-GB"`,
		`pdx:id="qr" pdx:alt="Betrag 5"`,
		`pdx:id="deko" pdx:artifact="true"`,
		`pdx:id="t2" pdx:alt="Logo"`,
		`pdx:id="s1" role="Reference" xml:lang="fr"`,
		`pdx:id="f1" pdx:alt="Zahl"`,
		`pdx:id="k1" pdx:scope="Column"`,
		`pdx:id="z1" pdx:scope="Row"`,
		`pdx:id="hu" pdx:artifact="true"`, // Übertrag
		`pdx:id="fr" pdx:artifact="true"`, // Zwischensumme
	} {
		if !strings.Contains(s, want) {
			t.Errorf("%s fehlt im FO", want)
		}
	}
	// Die Gesamtsumme bleibt Inhalt.
	if i := strings.Index(s, `pdx:id="ftot"`); i < 0 || strings.Contains(s[i:i+80], "pdx:artifact") {
		t.Errorf("Summenzeile als Artefakt markiert")
	}
}

// Die Dokumentsprache folgt der gewählten Sprache.
func TestDocumentLanguage(t *testing.T) {
	res, err := Compile(mini(t, "", `<text id="x">a</text>`), Options{Lang: "en"})
	if err != nil {
		t.Fatal(err)
	}
	fo, err := res.Transform([]byte(`<d/>`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(fo), `xml:lang="en"`) {
		t.Fatalf("Sprache: %.300s", fo)
	}
}
