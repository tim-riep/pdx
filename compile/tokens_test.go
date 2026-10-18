package compile

import (
	"bytes"
	"strings"
	"testing"

	"github.com/tim-riep/pdx/template"
)

const tokensHead = `<styles><style id="st" color="$primaer" padding="$m 2mm" font="$fliess"/></styles>
<tokens>
  <color id="marke" value="#1f4e79"/>
  <color id="primaer" value="$marke"/>
  <color id="linie" value="#cccccc"/>
  <length id="m" value="4mm"/>
  <length id="duenn" value="0.5pt"/>
  <font id="fliess" family="Liberation Sans" size="$klein" weight="normal" style="italic"/>
  <length id="klein" value="9pt"/>
</tokens>`

func TestTokens(t *testing.T) {
	tpl := mini(t, tokensHead,
		`<text id="x" style="st" fontStyle="normal" border="$duenn solid $linie" spaceAfter="$m">a</text>`+
			`<text id="y" font="$fliess" fontFamily="$fliess" fontWeight="bold">b</text>`)
	before := template.Write(tpl)
	res, err := Compile(tpl, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, template.Write(tpl)) {
		t.Fatal("Compile hat die Vorlage verändert")
	}
	x := string(res.XSLT)
	for _, want := range []string{
		`color="#1f4e79"`, `padding="4mm 2mm"`, `font-family="Liberation Sans"`, `font-size="9pt"`,
		`border="0.5pt solid #cccccc"`, `space-after="4mm"`, `font-weight="bold"`,
	} {
		if !strings.Contains(x, want) {
			t.Errorf("%s fehlt im XSLT", want)
		}
	}
	if strings.Contains(x, "$") && strings.Contains(x, "$primaer") {
		t.Error("Verweis nicht aufgelöst")
	}
	// Eigene Attribute schlagen das Schrift-Token des Stils: x ist normal.
	i := strings.Index(x, `pdx:id="x"`)
	if i < 0 || !strings.Contains(x[i:i+400], `font-style="normal"`) {
		t.Errorf("fontStyle des Elements geht vor font des Stils: %s", x[max(0, i):min(len(x), i+400)])
	}
}

func TestTokenErrors(t *testing.T) {
	cases := []struct {
		name, head, body, id, msg string
	}{
		{"unbekannt", "", `<text id="x" color="$fehlt">a</text>`, "x", "Token $fehlt gibt es nicht"},
		{"kreis", `<tokens><color id="p" value="$q"/><color id="q" value="$p"/></tokens>`, `<text id="x">a</text>`, "p", "im Kreis"},
		{"art", `<tokens><length id="m" value="4mm"/></tokens>`, `<text id="x" color="$m">a</text>`, "x", "color erwartet ein Farb-Token, $m ist ein Längen-Token"},
		{"font", `<tokens><color id="c" value="red"/></tokens>`, `<text id="x" font="$c">a</text>`, "x", "kein Schrift-Token"},
		{"kette", `<tokens><length id="m" value="4mm"/><color id="c" value="$m"/></tokens>`, `<text id="x">a</text>`, "c", "Token $c verweist auf $m, das ist ein Längen-Token"},
		{"im stil", `<styles><style id="s" spaceBefore="$weg"/></styles>`, `<text id="x" style="s">a</text>`, "s", "$weg"},
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

func TestTokenAttrsFromSchema(t *testing.T) {
	for name, kind := range map[string]string{"color": kindColor, "fill": kindColor, "x": kindLength, "thickness": kindLength, "padding": kindLength, "fontSize": kindLength, "border": kindBorder} {
		if tokenAttrs[name] != kind {
			t.Errorf("%s: %q, erwartet %q", name, tokenAttrs[name], kind)
		}
	}
	for _, name := range []string{"bind", "presence", "of", "id", "style"} {
		if _, ok := tokenAttrs[name]; ok {
			t.Errorf("%s darf keine Tokens aufnehmen", name)
		}
	}
}

func TestTokensSchema(t *testing.T) {
	// Verweise sind nur dort gültig, wo das XSD sie zulässt.
	if _, err := template.Parse([]byte(`<template xmlns="https://github.com/tim-riep/pdx/ns/template/1" id="t" lang="de-DE" version="1"><pageSet id="ps"><pageMaster id="pm"/></pageSet><body id="b"><text id="x" textAlign="$a">a</text></body></template>`)); err == nil {
		t.Fatal("Verweis in textAlign angenommen")
	}
}
