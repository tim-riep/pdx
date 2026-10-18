package sign

import (
	"bytes"
	"encoding/asn1"
	"math/big"
	"sort"
)

// Kleiner Baukasten für DER (ITU-T X.690). encoding/asn1 kann SET OF nicht
// nach DER sortieren und keine impliziten Tags an fertigen Werten setzen,
// beides braucht CMS.

// tlv kodiert Tag, Länge und Inhalt.
func tlv(tag byte, content ...[]byte) []byte {
	n := 0
	for _, c := range content {
		n += len(c)
	}
	var b bytes.Buffer
	b.WriteByte(tag)
	switch {
	case n < 0x80:
		b.WriteByte(byte(n))
	default:
		var l []byte
		for v := n; v > 0; v >>= 8 {
			l = append([]byte{byte(v)}, l...)
		}
		b.WriteByte(0x80 | byte(len(l)))
		b.Write(l)
	}
	for _, c := range content {
		b.Write(c)
	}
	return b.Bytes()
}

func seq(items ...[]byte) []byte { return tlv(0x30, items...) }

// set kodiert SET OF mit nach DER sortierten Elementen (X.690 11.6).
func set(items ...[]byte) []byte {
	s := append([][]byte(nil), items...)
	sort.Slice(s, func(i, j int) bool { return bytes.Compare(s[i], s[j]) < 0 })
	return tlv(0x31, s...)
}

func oid(o asn1.ObjectIdentifier) []byte {
	b, _ := asn1.Marshal(o)
	return b
}

func integer(n *big.Int) []byte {
	b, _ := asn1.Marshal(n)
	return b
}

func octets(b []byte) []byte { return tlv(0x04, b) }

// explicit setzt ein explizites kontextspezifisches Tag um einen Wert.
func explicit(n byte, inner ...[]byte) []byte { return tlv(0xA0|n, inner...) }

// implicitCons ersetzt das Tag eines zusammengesetzten Werts durch ein
// implizites kontextspezifisches Tag [n].
func implicitCons(n byte, der []byte) []byte {
	out := append([]byte(nil), der...)
	out[0] = 0xA0 | n
	return out
}

func null() []byte { return []byte{0x05, 0x00} }

// algorithm kodiert einen AlgorithmIdentifier, mit NULL-Parameter, wenn
// withNull gesetzt ist (RSA, Hashes nach RFC 5754 ohne).
func algorithm(o asn1.ObjectIdentifier, withNull bool) []byte {
	if withNull {
		return seq(oid(o), null())
	}
	return seq(oid(o))
}
