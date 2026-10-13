// Package pdf ist der Low-Level-Writer von pdx: ein kleines Objektmodell,
// indirekte Objekte, Flate-Streams, Xref-Tabelle und Trailer.
//
// Das Paket kennt weder Layout noch Schriften. Es schreibt deterministisch:
// Dictionary-Schlüssel werden sortiert, Zahlen fest formatiert und Objekte in
// der Reihenfolge ihres Anlegens nummeriert. Gleiche Aufrufe ergeben daher
// byte-identische Dateien.
package pdf

import (
	"bytes"
	"sort"
	"strconv"
	"unicode/utf16"
)

// Object ist ein beliebiges PDF-Objekt.
type Object interface {
	writePDF(b *bytes.Buffer)
}

// Null ist das PDF-Objekt null.
type Null struct{}

// Bool ist ein Wahrheitswert.
type Bool bool

// Int ist eine ganze Zahl.
type Int int64

// Real ist eine Gleitkommazahl. Sie wird mit höchstens fünf Nachkommastellen
// geschrieben, ohne Exponent und ohne abschließende Nullen.
type Real float64

// Name ist ein PDF-Name ohne den führenden Schrägstrich.
type Name string

// String ist ein literaler String in runden Klammern. Die Bytes werden
// unverändert übernommen und nur maskiert.
type String []byte

// HexString ist ein String in Hexadezimalschreibweise.
type HexString []byte

// Array ist ein PDF-Array.
type Array []Object

// Ref verweist auf ein indirektes Objekt.
type Ref struct {
	Num int
}

// Dict ist ein PDF-Dictionary. Die Schlüssel werden beim Schreiben sortiert.
type Dict struct {
	m map[Name]Object
}

// NewDict legt ein leeres Dictionary an.
func NewDict() *Dict { return &Dict{m: map[Name]Object{}} }

// D ist eine Kurzform, um ein Dictionary aus Schlüssel-Wert-Paaren zu bauen.
// Werte vom Typ nil werden übersprungen.
func D(kv ...any) *Dict {
	if len(kv)%2 != 0 {
		panic("pdf.D: ungerade Anzahl von Argumenten")
	}
	d := NewDict()
	for i := 0; i < len(kv); i += 2 {
		k, ok := kv[i].(string)
		if !ok {
			panic("pdf.D: Schlüssel muss ein string sein")
		}
		if kv[i+1] == nil {
			continue
		}
		d.Set(Name(k), kv[i+1].(Object))
	}
	return d
}

// Set setzt einen Eintrag. Ein Wert nil entfernt den Schlüssel.
func (d *Dict) Set(k Name, v Object) *Dict {
	if v == nil {
		delete(d.m, k)
		return d
	}
	d.m[k] = v
	return d
}

// Get liefert einen Eintrag oder nil.
func (d *Dict) Get(k Name) Object { return d.m[k] }

// Has meldet, ob der Schlüssel vorhanden ist.
func (d *Dict) Has(k Name) bool { _, ok := d.m[k]; return ok }

// Keys liefert die Schlüssel sortiert.
func (d *Dict) Keys() []Name {
	ks := make([]Name, 0, len(d.m))
	for k := range d.m {
		ks = append(ks, k)
	}
	sort.Slice(ks, func(i, j int) bool { return ks[i] < ks[j] })
	return ks
}

// Len liefert die Anzahl der Einträge.
func (d *Dict) Len() int { return len(d.m) }

// TextString kodiert einen Textstring nach ISO 32000 7.9.2.2. Reiner
// ASCII-Text bleibt literal, alles andere wird UTF-16BE mit BOM.
func TextString(s string) Object {
	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] > 0x7e {
			ascii = false
			break
		}
	}
	if ascii {
		return String(s)
	}
	u := utf16.Encode([]rune(s))
	b := make([]byte, 2, 2+2*len(u))
	b[0], b[1] = 0xfe, 0xff
	for _, c := range u {
		b = append(b, byte(c>>8), byte(c))
	}
	return String(b)
}

func (Null) writePDF(b *bytes.Buffer) { b.WriteString("null") }

func (v Bool) writePDF(b *bytes.Buffer) {
	if v {
		b.WriteString("true")
	} else {
		b.WriteString("false")
	}
}

func (v Int) writePDF(b *bytes.Buffer) { b.WriteString(strconv.FormatInt(int64(v), 10)) }

func (v Real) writePDF(b *bytes.Buffer) { b.WriteString(FormatNumber(float64(v))) }

// FormatNumber formatiert eine Zahl für PDF: höchstens fünf Nachkommastellen,
// kein Exponent, keine abschließenden Nullen, kein negatives Null.
func FormatNumber(v float64) string {
	s := strconv.FormatFloat(v, 'f', 5, 64)
	s = trimZeros(s)
	if s == "-0" {
		return "0"
	}
	return s
}

func trimZeros(s string) string {
	if i := indexByte(s, '.'); i >= 0 {
		j := len(s)
		for j > i+1 && s[j-1] == '0' {
			j--
		}
		if j == i+1 {
			j = i
		}
		s = s[:j]
	}
	return s
}

func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func (v Name) writePDF(b *bytes.Buffer) {
	b.WriteByte('/')
	const hex = "0123456789ABCDEF"
	for i := 0; i < len(v); i++ {
		c := v[i]
		if c < 0x21 || c > 0x7e || c == '#' || isDelimiter(c) {
			b.WriteByte('#')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&15])
			continue
		}
		b.WriteByte(c)
	}
}

func isDelimiter(c byte) bool {
	switch c {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return false
}

func (v String) writePDF(b *bytes.Buffer) {
	b.WriteByte('(')
	for _, c := range []byte(v) {
		switch c {
		case '(', ')', '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte(')')
}

func (v HexString) writePDF(b *bytes.Buffer) {
	const hex = "0123456789ABCDEF"
	b.WriteByte('<')
	for _, c := range []byte(v) {
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&15])
	}
	b.WriteByte('>')
}

func (v Array) writePDF(b *bytes.Buffer) {
	b.WriteByte('[')
	for i, o := range v {
		if i > 0 {
			b.WriteByte(' ')
		}
		o.writePDF(b)
	}
	b.WriteByte(']')
}

func (v Ref) writePDF(b *bytes.Buffer) {
	b.WriteString(strconv.Itoa(v.Num))
	b.WriteString(" 0 R")
}

func (d *Dict) writePDF(b *bytes.Buffer) {
	b.WriteString("<<")
	for _, k := range d.Keys() {
		k.writePDF(b)
		b.WriteByte(' ')
		d.m[k].writePDF(b)
	}
	b.WriteString(">>")
}

// Serialize liefert die PDF-Schreibweise eines Objekts, etwa für Tests.
func Serialize(o Object) string {
	var b bytes.Buffer
	o.writePDF(&b)
	return b.String()
}
