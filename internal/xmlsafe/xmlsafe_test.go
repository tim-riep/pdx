package xmlsafe

import (
	"testing"
	"unicode/utf16"
)

func utf16Bytes(s string, bigEndian bool) []byte {
	var out []byte
	if bigEndian {
		out = []byte{0xFE, 0xFF}
	} else {
		out = []byte{0xFF, 0xFE}
	}
	for _, u := range utf16.Encode([]rune(s)) {
		if bigEndian {
			out = append(out, byte(u>>8), byte(u))
		} else {
			out = append(out, byte(u), byte(u>>8))
		}
	}
	return out
}

func TestCheck(t *testing.T) {
	bomb := `<?xml version="1.0"?><!DOCTYPE r [<!ENTITY a "x"><!ENTITY b "&a;&a;">]><r>&b;</r>`
	for name, b := range map[string][]byte{
		"UTF-8":     []byte(bomb),
		"UTF-16LE":  utf16Bytes(bomb, false),
		"UTF-16BE":  utf16Bytes(bomb, true),
		"Parameter": []byte(`<!DOCTYPE r [<!ENTITY % p "x">]><r/>`),
		"extern":    []byte(`<!DOCTYPE r [<!ENTITY e SYSTEM "file:///etc/passwd">]><r>&e;</r>`),
	} {
		if Check(b) != ErrEntities {
			t.Errorf("%s: nicht abgelehnt", name)
		}
	}
	for name, b := range map[string][]byte{
		"einfach":     []byte(`<?xml version="1.0"?><rechnung><betrag>1 &lt; 2 &amp; 3</betrag></rechnung>`),
		"DOCTYPE":     []byte(`<!DOCTYPE rechnung SYSTEM "rechnung.dtd"><rechnung/>`),
		"Zeichen":     []byte(`<r>&#x26;&#60;!ENTITY</r>`),
		"klein":       []byte(`<r>&lt;!entity</r>`),
		"UTF-16 ohne": utf16Bytes(`<r>ENTITY</r>`, false),
		"leer":        nil,
	} {
		if err := Check(b); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}
