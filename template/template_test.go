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
	if f == nil || f.Get("bind", "") != "inv:number" {
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
	bad := strings.Replace(string(data), `<field id="z-menge-f" bind="inv:quantity" format="menge"/>`,
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
		`<template xmlns="https://github.com/tim-riep/pdx/ns/template/1" id="a" lang="de" version="1"><pageSet id="p"><pageMaster id="m"/></pageSet><body id="b"><unbekannt id="u"/></body></template>`:          "Schema",
		`<template xmlns="https://github.com/tim-riep/pdx/ns/template/1" id="a" lang="de" version="1"><pageSet id="p"><pageMaster id="m"/></pageSet><body id="b"><text id="x"/><text id="x"/></body></template>`: "Schema",
		`<template xmlns="https://github.com/tim-riep/pdx/ns/template/1" id="a" lang="de" version="1"><pageSet id="p">Text</pageSet></template>`:                                                                 "keinen Text",
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
	f.Add([]byte(`<template xmlns="https://github.com/tim-riep/pdx/ns/template/1" id="a" lang="de" version="1"/>`))
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

// Vorlagen im Format 0 (bis pdx 0.1.0) werden gelesen und auf Format 1
// gehoben: neuer Namespace beim Schreiben, version 1, Schema gültig.
func TestLegacyFormat0(t *testing.T) {
	cur, err := os.ReadFile(sample)
	if err != nil {
		t.Fatal(err)
	}
	old := strings.Replace(string(cur), NS, LegacyNS, 1)
	old = strings.Replace(old, `version="1"`, `version="0"`, 1)
	if !strings.Contains(old, LegacyNS) || !strings.Contains(old, `version="0"`) {
		t.Fatal("Testvorlage im Format 0 nicht gebaut")
	}
	tpl, err := Parse([]byte(old))
	if err != nil {
		t.Fatalf("Format 0 nicht gelesen: %v", err)
	}
	if v := tpl.Root.Get("version", ""); v != Version {
		t.Errorf("version %q statt %q", v, Version)
	}
	out := string(Write(tpl))
	if !strings.Contains(out, `xmlns="`+NS+`"`) || strings.Contains(out, LegacyNS) {
		t.Error("geschrieben nicht im Namespace von Format 1")
	}
	want, _ := Parse(cur)
	if !bytes.Equal(Write(want), Write(tpl)) {
		t.Error("Format 0 ergibt eine andere Vorlage als Format 1")
	}
	if _, err := Parse([]byte(strings.Replace(string(cur), NS, "urn:fremd", 1))); err == nil {
		t.Error("fremder Namespace angenommen")
	}
}
