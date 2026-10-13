package pdf

import (
	"bytes"
	"compress/zlib"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/tim-riep/pdx/internal/testutil"
)

func TestSerialize(t *testing.T) {
	cases := []struct {
		o    Object
		want string
	}{
		{Null{}, "null"},
		{Bool(true), "true"},
		{Int(-42), "-42"},
		{Real(1.5), "1.5"},
		{Real(2), "2"},
		{Real(-0.000001), "0"},
		{Real(0.123456), "0.12346"},
		{Name("A B#/"), "/A#20B#23#2F"},
		{String("a(b)\\c\n"), `(a\(b\)\\c\n)`},
		{HexString{0x00, 0xff}, "<00FF>"},
		{Array{Int(1), Name("X"), Ref{3}}, "[1 /X 3 0 R]"},
		{D("Z", Int(1), "A", Int(2)), "<</A 2/Z 1>>"},
		{TextString("Grüße"), "(\xfe\xff\x00G\x00r\x00\xfc\x00\xdf\x00e)"},
	}
	for _, c := range cases {
		if got := Serialize(c.o); got != c.want {
			t.Errorf("Serialize(%#v) = %q, erwartet %q", c.o, got, c.want)
		}
	}
}

func TestDate(t *testing.T) {
	loc := time.FixedZone("x", -(5*3600 + 30*60))
	got := string(Date(time.Date(2026, 1, 2, 3, 4, 5, 0, loc)))
	if got != "D:20260102030405-05'30'" {
		t.Errorf("Date = %q", got)
	}
	got = string(Date(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)))
	if got != "D:20260101000000+00'00'" {
		t.Errorf("Date UTC = %q", got)
	}
}

func TestFlateRoundTrip(t *testing.T) {
	raw := bytes.Repeat([]byte("pdx "), 100)
	s := NewFlateStream(nil, raw)
	r, err := zlib.NewReader(bytes.NewReader(s.Data))
	if err != nil {
		t.Fatal(err)
	}
	back, _ := io.ReadAll(r)
	if !bytes.Equal(back, raw) {
		t.Fatal("Flate-Daten nicht rekonstruierbar")
	}
}

// minimal baut ein PDF mit einer leeren A4-Seite.
func minimal(p Policy) *Document {
	d := NewDocument(p)
	pages := d.Reserve()
	page := d.Add(D("Type", Name("Page"), "Parent", pages,
		"MediaBox", Array{Int(0), Int(0), Real(595.276), Real(841.89)},
		"Contents", d.Add(NewFlateStream(nil, []byte("0 0 1 rg\n10 10 100 100 re\nf\n"))),
		"Resources", NewDict()))
	d.Set(pages, D("Type", Name("Pages"), "Kids", Array{page}, "Count", Int(1)))
	d.Root = d.Add(D("Type", Name("Catalog"), "Pages", pages))
	d.Info = d.Add(D("Producer", String("pdx"), "CreationDate", Date(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))))
	return d
}

func TestMinimalGolden(t *testing.T) {
	b, err := minimal(nil).Bytes()
	if err != nil {
		t.Fatal(err)
	}
	n, err := testutil.CheckStructure(b)
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Errorf("%d Objekte, erwartet 5", n)
	}
	testutil.Golden(t, "testdata/minimal.pdf", b)
}

func TestReproducible(t *testing.T) {
	a, _ := minimal(nil).Bytes()
	b, _ := minimal(nil).Bytes()
	if !bytes.Equal(a, b) {
		t.Fatal("zwei Läufe ergeben unterschiedliche Bytes")
	}
}

func TestUnfilledReserve(t *testing.T) {
	d := minimal(nil)
	d.Reserve()
	if _, err := d.Bytes(); err == nil {
		t.Fatal("reserviertes, leeres Objekt nicht erkannt")
	}
}

type denyAll struct{}

func (denyAll) Name() string { return "test" }
func (denyAll) Allow(f Feature) error {
	return &FeatureError{Feature: f, Profile: "test"}
}

func TestPolicyScan(t *testing.T) {
	cases := map[string]Object{
		"javascript": D("S", Name("JavaScript"), "JS", String("app.alert(1)")),
		"lzw":        NewStream(D("Filter", Name("LZWDecode")), nil),
		"lzw-array":  NewStream(D("Filter", Array{Name("ASCII85Decode"), Name("LZWDecode")}), nil),
		"extern":     NewStream(D("F", String("datei.bin")), nil),
		"smask":      D("Type", Name("XObject"), "SMask", Ref{1}),
		"alpha":      D("Type", Name("ExtGState"), "ca", Real(0.5)),
		"blend":      D("Type", Name("ExtGState"), "BM", Name("Multiply")),
		"std14":      D("Type", Name("Font"), "Subtype", Name("Type1"), "BaseFont", Name("Helvetica")),
		"fontdesc":   D("Type", Name("FontDescriptor"), "FontName", Name("X")),
		"anhang":     NewStream(D("Type", Name("EmbeddedFile")), nil),
	}
	for name, o := range cases {
		t.Run(name, func(t *testing.T) {
			d := minimal(denyAll{})
			d.Add(o)
			_, err := d.Bytes()
			var fe *FeatureError
			if !errors.As(err, &fe) {
				t.Fatalf("erwartet FeatureError, erhalten %v", err)
			}
		})
	}
	// Ohne Policy sind JavaScript und XFA trotzdem verboten.
	d := minimal(nil)
	d.Add(D("XFA", Array{}))
	if _, err := d.Bytes(); err == nil {
		t.Fatal("XFA ohne Policy nicht verweigert")
	}
	// Unkritische Objekte passieren auch denyAll.
	d = minimal(denyAll{})
	d.Add(D("Type", Name("ExtGState"), "ca", Real(1), "SMask", Name("None")))
	if _, err := d.Bytes(); err != nil {
		t.Fatalf("unerwarteter Fehler: %v", err)
	}
}

func TestContent(t *testing.T) {
	var c Content
	c.Save()
	c.FillRGB(1, 0.5, 0)
	c.Rect(0, 0, 10.25, 20)
	c.Fill()
	c.BeginText()
	c.Font("F1", 12)
	c.TextMatrix(1, 0, 0, 1, 72, 700)
	c.ShowGlyphs([]uint16{0x24, 0x1F4A, 0x10}, []float64{0, 0, 25})
	c.EndText()
	c.Restore()
	want := "q\n1 0.5 0 rg\n0 0 10.25 20 re\nf\nBT\n/F1 12 Tf\n1 0 0 1 72 700 Tm\n[<00241F4A> 25 <0010>] TJ\nET\nQ\n"
	if got := string(c.Bytes()); got != want {
		t.Errorf("Content =\n%s\nerwartet\n%s", got, want)
	}
}
