package font

import (
	"bytes"
	"compress/zlib"
	"io"
	"strings"
	"testing"

	"github.com/boxesandglue/textshape/ot"
	"github.com/tim-riep/pdx/pdf"
)

// policy erlaubt alles außer den angegebenen Funktionen.
type policy struct{ deny pdf.Feature }

func (policy) Name() string { return "test" }
func (p policy) Allow(f pdf.Feature) error {
	if f == p.deny {
		return &pdf.FeatureError{Feature: f, Profile: "test", Reason: "verboten"}
	}
	return nil
}

func helvetica(t *testing.T) *Face {
	t.Helper()
	f, err := NewRegistry().Lookup("Helvetica", 400, false)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// toUnicodeOf liefert die entpackte ToUnicode-CMap einer eingebetteten Schrift.
func toUnicodeOf(t *testing.T, d *pdf.Document, e *Embedded) string {
	t.Helper()
	font := d.Get(e.Ref).(*pdf.Dict)
	st := d.Get(font.Get("ToUnicode").(pdf.Ref)).(*pdf.Stream)
	r, err := zlib.NewReader(bytes.NewReader(st.Data))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(r)
	return string(b)
}

// Folgeglyphen eines Clusters tragen keinen Text, sie bekommen das Zeichen
// aus der cmap der Schrift.
func TestToUnicodeFromCmap(t *testing.T) {
	f := helvetica(t)
	gid, ok := f.GlyphFor('a')
	if !ok {
		t.Fatal("a fehlt")
	}
	u := NewUsage(f)
	u.Add(gid, "")
	d := pdf.NewDocument(policy{deny: pdf.UnmappedGlyph})
	e, err := Embed(d, u)
	if err != nil {
		t.Fatal(err)
	}
	if cm := toUnicodeOf(t, d, e); !strings.Contains(cm, "<0061>") {
		t.Fatalf("a fehlt in der ToUnicode-CMap:\n%s", cm)
	}
}

// Ohne Zuordnung entscheidet das Profil: die Stufen U und A lehnen ab, mit
// Schrift und Glyphe in der Meldung.
func TestUnmappedGlyph(t *testing.T) {
	f := helvetica(t)
	reverse := f.face.Cmap().CollectReverseMapping()
	var gid uint16
	for g := uint16(1); g < 3000; g++ {
		if _, ok := reverse[ot.GlyphID(g)]; !ok && f.Advance(g) > 0 {
			gid = g
			break
		}
	}
	if gid == 0 {
		t.Skip("keine Glyphe ohne cmap-Eintrag gefunden")
	}
	u := NewUsage(f)
	u.Add(gid, "")
	_, err := Embed(pdf.NewDocument(policy{deny: pdf.UnmappedGlyph}), u)
	if err == nil || !strings.Contains(err.Error(), f.PostScript) || !strings.Contains(err.Error(), "Unicode") {
		t.Fatalf("Fehler %v", err)
	}
	if _, err := Embed(pdf.NewDocument(policy{}), u); err != nil {
		t.Fatalf("ohne Anspruch abgelehnt: %v", err)
	}
}
