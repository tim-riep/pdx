package hyph

import (
	"strings"
	"testing"
)

func show(word string, cuts []int) string {
	r := []rune(word)
	var b strings.Builder
	last := 0
	for _, c := range cuts {
		b.WriteString(string(r[last:c]))
		b.WriteByte('-')
		last = c
	}
	b.WriteString(string(r[last:]))
	return b.String()
}

func TestGerman(t *testing.T) {
	p, ok, err := ForLanguage("de-DE")
	if err != nil || !ok {
		t.Fatalf("Muster fehlen: %v", err)
	}
	cases := map[string]string{
		"Silbentrennung":  "Sil-ben-tren-nung",
		"Rechnungsnummer": "Rech-nungs-num-mer",
		"Geschäftsführer": "Ge-schäfts-füh-rer",
		"Straße":          "Stra-ße",
		"Zwischensumme,":  "Zwi-schen-sum-me,",
		"und":             "und",
		"RE-2026-0001":    "RE-2026-0001",
	}
	for in, want := range cases {
		if got := show(in, p.Hyphenate(in, 0, 0)); got != want {
			t.Errorf("%s: %s, erwartet %s", in, got, want)
		}
	}
	// Minimum lengths from hyphenation-remain- and -push-character-count.
	if got := show("Silbentrennung", p.Hyphenate("Silbentrennung", 4, 5)); got != "Silben-trennung" {
		t.Errorf("mit 4/5: %s", got)
	}
}

func TestEnglish(t *testing.T) {
	p, ok, err := ForLanguage("en-GB")
	if err != nil || !ok {
		t.Fatalf("Muster fehlen: %v", err)
	}
	cases := map[string]string{
		"hyphenation": "hy-phen-ation",
		"table":       "ta-ble",  // exception list
		"project":     "project", // exception list without hyphenation
		"typesetting": "type-set-ting",
	}
	for in, want := range cases {
		if got := show(in, p.Hyphenate(in, 0, 0)); got != want {
			t.Errorf("%s: %s, erwartet %s", in, got, want)
		}
	}
}

func TestUnknownLanguage(t *testing.T) {
	if _, ok, err := ForLanguage("fr"); ok || err != nil {
		t.Error("Französisch sollte nicht vorhanden sein")
	}
}
