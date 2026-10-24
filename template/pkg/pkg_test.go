package pkg

import (
	"archive/zip"
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/tim-riep/go-xslt/engine"
	"github.com/tim-riep/pdx/template"
)

const dir = "../../testdata/templates/musterrechnung"

func TestDirAndZipRoundTrip(t *testing.T) {
	p, err := ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Samples(); len(got) != 1 || got[0] != "samples/rechnung.xml" {
		t.Errorf("Beispiele: %v", got)
	}
	a, err := p.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := p.Bytes()
	if !bytes.Equal(a, b) {
		t.Fatal("ZIP nicht reproduzierbar")
	}
	p2, err := Read(a)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := p2.Bytes()
	if !bytes.Equal(a, c) {
		t.Fatal("ZIP nach erneutem Laden verschieden")
	}
	// The first entry is template.xml, sorted after that.
	zr, _ := zip.NewReader(bytes.NewReader(a), int64(len(a)))
	if zr.File[0].Name != "template.xml" || zr.File[1].Name > zr.File[2].Name {
		t.Errorf("Reihenfolge: %s, %s, %s", zr.File[0].Name, zr.File[1].Name, zr.File[2].Name)
	}
}

func TestSampleDataIsValid(t *testing.T) {
	p, err := ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	main, all := p.Schemas()
	data, _ := p.File("samples/rechnung.xml")
	res := engine.Validate(engine.ValidateRequest{Schemas: []string{string(all[main])}, Instance: string(data)})
	if !res.Valid {
		t.Fatalf("Beispieldaten ungültig: %+v", res.Diagnostics)
	}
}

func zipOf(files map[string]string) []byte {
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for n, c := range files {
		w, _ := zw.Create(n)
		w.Write([]byte(c))
	}
	zw.Close()
	return b.Bytes()
}

const minimal = `<template xmlns="https://github.com/tim-riep/pdx/ns/template/1" id="t" lang="de" version="1"><pageSet id="ps"><pageMaster id="pm"/></pageSet><body id="b"/></template>`

func TestRejects(t *testing.T) {
	cases := map[string]map[string]string{
		"unzulässig":   {"template.xml": minimal, "../etc/passwd": "x"},
		"gehört nicht": {"template.xml": minimal, "readme.txt": "x"},
		"fehlt":        {"assets/logo.png": "x"},
		// Entity declarations in schemas and samples go to the parser of go-xslt.
		"schema/a.xsd: XML mit Entity": {"template.xml": minimal, "schema/a.xsd": `<!DOCTYPE s [<!ENTITY a "x">]><s/>`},
		"ENTITY":                       {"template.xml": minimal, "samples/a.xml": `<!DOCTYPE r [<!ENTITY a "x">]><r>&a;</r>`},
	}
	for want, files := range cases {
		if _, err := Read(zipOf(files)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("erwartet %q, erhalten %v", want, err)
		}
	}
	// A reference to a missing file carries the ID of the element.
	tpl := strings.Replace(minimal, `<body id="b"/>`, `<body id="b"><image id="logo" src="assets/logo.png"/></body>`, 1)
	_, err := Read(zipOf(map[string]string{"template.xml": tpl}))
	var errs template.Errors
	if !errors.As(err, &errs) || errs[0].ID != "logo" {
		t.Fatalf("erwartet Fehler mit ID logo, erhalten %v", err)
	}
	if _, err := Read([]byte("kein zip")); err == nil {
		t.Error("Nicht-ZIP angenommen")
	}
}
