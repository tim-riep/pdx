// Package xmlsafe refuses XML that declares entities in a DTD before it
// reaches the XML parser of go-xslt. Entities that refer to each other
// ("billion laughs") expand a few hundred bytes into gigabytes
// (GHSA-3qwg-cpmj-qx68, GHSA-6xxr-4cc9-4m89). go-xslt v0.3.3 bounds the
// expansion, this check stays as a second line of defence: data, templates
// and schemas of pdx never need entity declarations, so pdx refuses them
// outright.
package xmlsafe

import (
	"bytes"
	"errors"
	"unicode/utf16"
)

// ErrEntities reports XML with an entity declaration.
var ErrEntities = errors.New("XML mit Entity-Deklarationen (<!ENTITY) wird nicht verarbeitet, Schutz vor Entity-Expansion")

// decl is the declaration in UTF-8 and in both byte orders of UTF-16,
// because the parser also reads UTF-16 documents.
var decl = func() [][]byte {
	const s = "<!ENTITY"
	le, be := make([]byte, 0, 2*len(s)), make([]byte, 0, 2*len(s))
	for _, u := range utf16.Encode([]rune(s)) {
		le = append(le, byte(u), byte(u>>8))
		be = append(be, byte(u>>8), byte(u))
	}
	return [][]byte{[]byte(s), le, be}
}()

// Check returns ErrEntities if b declares an entity. A declaration can only
// stand in a DTD, so ordinary documents never trip it. The text inside
// a comment or CDATA section counts as well, which errs on the safe side.
func Check(b []byte) error {
	for _, d := range decl {
		if bytes.Contains(b, d) {
			return ErrEntities
		}
	}
	return nil
}
