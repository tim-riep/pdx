package profile

import (
	"strings"
	"testing"

	"github.com/tim-riep/pdx/pdf"
)

// fake ist ein Profil für Tests der Kombinationsregeln, bevor die echten
// Profile für PDF/UA und PDF/A-4 existieren.
type fake struct {
	name string
	c    Claim
}

func (f fake) Name() string          { return f.name }
func (fake) Allow(pdf.Feature) error { return nil }
func (fake) Prepare(*Context) error  { return nil }
func (f fake) claim() Claim          { return f.c }

func init() {
	for _, f := range []fake{
		{"test-ua1", Claim{Version: "1.7", UA: 1}},
		{"test-ua2", Claim{Version: "2.0", UA: 2}},
		{"test-a4f", Claim{Version: "2.0", PDFA: 4, Level: "F"}},
		{"test-a3a", Claim{Version: "1.7", PDFA: 3, Level: "A"}},
	} {
		f := f
		register(f.name, func() Profile { return f })
	}
}

func TestLookupSingle(t *testing.T) {
	p, err := Lookup("pdfa-3b")
	if err != nil || p.Name() != "pdfa-3b" {
		t.Fatalf("pdfa-3b: %v %v", p, err)
	}
	if c := ClaimOf(p); c != (Claim{Version: "1.7", PDFA: 3, Level: "B"}) {
		t.Fatalf("Anspruch %+v", c)
	}
	if c := ClaimOf(PDF17{}); c.Version != "1.7" || c.PDFA != 0 || c.UA != 0 {
		t.Fatalf("pdf-1.7 %+v", c)
	}
}

func TestLookupCombinations(t *testing.T) {
	ok := map[string]Claim{
		"test-a3a,test-ua1":   {Version: "1.7", PDFA: 3, Level: "A", UA: 1},
		" pdfa-3b , test-ua1": {Version: "1.7", PDFA: 3, Level: "B", UA: 1},
		"test-a4f,test-ua2":   {Version: "2.0", PDFA: 4, Level: "F", UA: 2},
	}
	for spec, want := range ok {
		p, err := Lookup(spec)
		if err != nil {
			t.Errorf("%s: %v", spec, err)
			continue
		}
		if got := ClaimOf(p); got != want {
			t.Errorf("%s: %+v, erwartet %+v", spec, got, want)
		}
	}
	if p, _ := Lookup("test-a3a,test-ua1"); p.Name() != "test-a3a,test-ua1" {
		t.Errorf("Name %q", p.Name())
	}
}

// Unzulässige Kombinationen nennen alle Gründe auf einmal.
func TestLookupRejects(t *testing.T) {
	cases := map[string][]string{
		"test-a3a,test-ua2":          {"verlangt PDF 1.7", "verlangt PDF 2.0"},
		"test-a4f,test-ua1":          {"verlangt PDF 2.0", "verlangt PDF 1.7"},
		"pdfa-3b,pdf-1.7":            {"pdf-1.7 steht für PDF ohne Konformitätsanspruch"},
		"pdfa-3b,test-a3a":           {"nur ein PDF/A-Profil je Datei"},
		"pdfa-3b,test-a4f,test-ua1":  {"nur ein PDF/A-Profil", "verlangt PDF 2.0"},
		"test-ua1,test-ua2":          {"nur ein PDF/UA-Profil", "verlangt PDF"},
		"pdfa-3b,pdfa-3b,gibtsnicht": {"doppelt", "unbekanntes Profil \"gibtsnicht\""},
		"":                           {"kein Profil angegeben"},
	}
	for spec, wants := range cases {
		_, err := Lookup(spec)
		if err == nil {
			t.Errorf("%q angenommen", spec)
			continue
		}
		for _, w := range wants {
			if !strings.Contains(err.Error(), w) {
				t.Errorf("%q: Meldung %q enthält nicht %q", spec, err, w)
			}
		}
	}
}

func TestSetAllow(t *testing.T) {
	p, err := Lookup("pdfa-3b,test-ua1")
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Allow(pdf.LZW); err == nil || !strings.Contains(err.Error(), "6.1.7.2") {
		t.Fatalf("LZW: %v", err)
	}
	if err := p.Allow(pdf.Attachment); err != nil {
		t.Fatalf("Anhang: %v", err)
	}
}
