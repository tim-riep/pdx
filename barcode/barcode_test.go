package barcode

import (
	"strings"
	"testing"
)

func TestGiroPayload(t *testing.T) {
	g := Giro{Name: "Beispiel GmbH", IBAN: "DE02 1203 0000 0000 2020 51", BIC: "BYLADEM1001", Amount: "1190.00", Text: "RE-2026-0001"}
	p, err := g.Payload()
	if err != nil {
		t.Fatal(err)
	}
	want := "BCD\n002\n1\nSCT\nBYLADEM1001\nBeispiel GmbH\nDE02120300000000202051\nEUR1190.00\n\n\nRE-2026-0001"
	if p != want {
		t.Errorf("Nutzlast:\n%q\nerwartet\n%q", p, want)
	}
	bad := []Giro{
		{Name: "X", IBAN: "DE00120300000000202051"},                // Prüfziffer falsch
		{Name: "", IBAN: "DE02120300000000202051"},                 // Name fehlt
		{Name: "X", IBAN: "DE02120300000000202051", Amount: "1,5"}, // Komma
		{Name: "X", IBAN: "DE02120300000000202051", Reference: "RF18", Text: "x"},
	}
	for _, b := range bad {
		if _, err := b.Payload(); err == nil {
			t.Errorf("%+v: Fehler erwartet", b)
		}
	}
}

func TestGenerate(t *testing.T) {
	q, err := Generate(Spec{Type: "qr", Value: "https://pdx.example/"})
	if err != nil || q.Cols != q.Rows || q.Cols < 21 || q.Quiet != 4 {
		t.Fatalf("QR: %+v %v", q, err)
	}
	// Die drei Suchmuster des QR-Codes: Ecken oben links, oben rechts und unten links sind dunkel.
	if !q.Dark(0, 0) || !q.Dark(q.Cols-1, 0) || !q.Dark(0, q.Rows-1) {
		t.Error("Suchmuster des QR-Codes fehlen")
	}
	d, err := Generate(Spec{Type: "datamatrix", Value: "4711"})
	if err != nil || d.Cols < 10 || !d.Dark(0, d.Rows-1) {
		t.Fatalf("DataMatrix: %+v %v", d, err)
	}
	c, err := Generate(Spec{Type: "code128", Value: "RE-2026-0001"})
	if err != nil || c.Rows != 1 || !c.Linear || c.Cols < 60 {
		t.Fatalf("Code 128: %+v %v", c, err)
	}
	if len(c.Rects()) < 10 {
		t.Error("zu wenige Balken")
	}
	g, err := Generate(Spec{Type: "girocode", Giro: Giro{Name: "Beispiel GmbH", IBAN: "DE02120300000000202051", Amount: "10.00"}})
	if err != nil || g.Cols < 25 {
		t.Fatalf("GiroCode: %v", err)
	}
	if _, err := Generate(Spec{Type: "ean13", Value: "1"}); err == nil || !strings.Contains(err.Error(), "unbekannt") {
		t.Error("unbekannter Typ nicht erkannt")
	}
}
