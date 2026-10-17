package template

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
)

const sample = "../testdata/templates/musterrechnung/template.xml"

func TestParseSample(t *testing.T) {
	data, err := os.ReadFile(sample)
	if err != nil {
		t.Fatal(err)
	}
	tpl, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if tpl.ByID("positionen") == nil || tpl.ByID("positionen").Name != "table" {
		t.Error("Tabelle nicht gefunden")
	}
	f := tpl.ByID("rechnungsnummer-wert")
	if f == nil || f.Get("bind", "") != "rsm:ExchangedDocument/ram:ID" {
		t.Errorf("Feld: %+v", f)
	}
	// Gemischter Inhalt bleibt erhalten.
	txt := tpl.ByID("absenderzeile")
	if len(txt.Children) != 3 || txt.Children[1].Kind != Text || txt.Children[1].Data != " · " {
		t.Errorf("gemischter Inhalt: %d Kinder", len(txt.Children))
	}
}

func TestCanonicalRoundTrip(t *testing.T) {
	data, _ := os.ReadFile(sample)
	tpl, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	once := Write(tpl)
	tpl2, err := Parse(once)
	if err != nil {
		t.Fatal(err)
	}
	twice := Write(tpl2)
	if !bytes.Equal(once, twice) {
		t.Fatal("kanonische Form ist nicht stabil")
	}
	// Die Musterrechnung liegt bereits kanonisch vor.
	if !bytes.Equal(once, data) {
		t.Error("template.xml der Musterrechnung ist nicht kanonisch formatiert")
	}
}

func TestSchemaErrorsCarryID(t *testing.T) {
	data, _ := os.ReadFile(sample)
	bad := strings.Replace(string(data), `<field id="z-menge-f" bind="ram:SpecifiedLineTradeDelivery/ram:BilledQuantity" format="menge"/>`,
		`<field id="z-menge-f" format="menge"/>`, 1)
	_, err := Parse([]byte(bad))
	var errs Errors
	if !errors.As(err, &errs) || len(errs) == 0 {
		t.Fatalf("erwartet Schemafehler, erhalten %v", err)
	}
	if errs[0].ID != "z-menge-f" || errs[0].Line == 0 {
		t.Errorf("Fehler ohne passende ID oder Position: %v", errs[0])
	}
	cases := map[string]string{
		`<template xmlns="urn:falsch"/>`: "Namespace",
		`<template xmlns="https://pdx.example/ns/template/0" id="a" lang="de" version="0"><pageSet id="p"><pageMaster id="m"/></pageSet><body id="b"><unbekannt id="u"/></body></template>`: "Schema",
		`<template xmlns="https://pdx.example/ns/template/0" id="a" lang="de" version="0"><pageSet id="p"><pageMaster id="m"/></pageSet><body id="b"><text id="x"/><text id="x"/></body></template>`: "Schema",
		`<template xmlns="https://pdx.example/ns/template/0" id="a" lang="de" version="0"><pageSet id="p">Text</pageSet></template>`: "keinen Text",
	}
	for in, want := range cases {
		if _, err := Parse([]byte(in)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: erwartet %q, erhalten %v", in, want, err)
		}
	}
}

func FuzzTemplateParse(f *testing.F) {
	data, err := os.ReadFile(sample)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(data)
	f.Add([]byte(`<template xmlns="https://pdx.example/ns/template/0" id="a" lang="de" version="0"/>`))
	f.Fuzz(func(t *testing.T, b []byte) {
		tpl, err := Parse(b)
		if err != nil {
			return
		}
		// Gültige Vorlagen müssen sich stabil kanonisch schreiben lassen.
		once := Write(tpl)
		tpl2, err := Parse(once)
		if err != nil {
			t.Fatalf("kanonische Form ungültig: %v", err)
		}
		if !bytes.Equal(once, Write(tpl2)) {
			t.Fatal("kanonische Form nicht stabil")
		}
	})
}
