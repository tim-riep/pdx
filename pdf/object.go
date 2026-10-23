// Package pdf is the low-level writer of pdx: a small object model,
// indirect objects, Flate streams, xref table and trailer.
//
// The package knows neither layout nor fonts. It writes deterministically:
// dictionary keys are sorted, numbers have a fixed format and objects are
// numbered in the order of their creation. Identical calls therefore yield
// byte-identical files.
package pdf

import (
	"bytes"
	"sort"
	"strconv"
	"unicode/utf16"
)

// Object is any PDF object.
type Object interface {
	writePDF(b *bytes.Buffer)
}

// Null is the PDF object null.
type Null struct{}

// Bool is a boolean value.
type Bool bool

// Int is an integer.
type Int int64

// Real is a floating-point number. It is written with at most five decimal
// places, without exponent and without trailing zeros.
type Real float64

// Name is a PDF name without the leading slash.
type Name string

// String is a literal string in parentheses. The bytes are taken over
// unchanged and only escaped.
type String []byte

// HexString is a string in hexadecimal notation.
type HexString []byte

// Array is a PDF array.
type Array []Object

// Ref refers to an indirect object.
type Ref struct {
	Num int
}

// Dict is a PDF dictionary. The keys are sorted when writing.
type Dict struct {
	m map[Name]Object
}

// NewDict creates an empty dictionary.
func NewDict() *Dict { return &Dict{m: map[Name]Object{}} }

// D is a shorthand to build a dictionary from key-value pairs.
// Values of type nil are skipped.
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

// Set sets an entry. A nil value removes the key.
func (d *Dict) Set(k Name, v Object) *Dict {
	if v == nil {
		delete(d.m, k)
		return d
	}
	d.m[k] = v
	return d
}

// Get returns an entry or nil.
func (d *Dict) Get(k Name) Object { return d.m[k] }

// Has reports whether the key is present.
func (d *Dict) Has(k Name) bool { _, ok := d.m[k]; return ok }

// Keys returns the keys sorted.
func (d *Dict) Keys() []Name {
	ks := make([]Name, 0, len(d.m))
	for k := range d.m {
		ks = append(ks, k)
	}
	sort.Slice(ks, func(i, j int) bool { return ks[i] < ks[j] })
	return ks
}

// Len returns the number of entries.
func (d *Dict) Len() int { return len(d.m) }

// TextString encodes a text string according to ISO 32000 7.9.2.2. Pure
// ASCII text stays literal, everything else becomes UTF-16BE with BOM.
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

// FormatNumber formats a number for PDF: at most five decimal places,
// no exponent, no trailing zeros, no negative zero.
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

// Serialize returns the PDF notation of an object, for example for tests.
func Serialize(o Object) string {
	var b bytes.Buffer
	o.writePDF(&b)
	return b.String()
}
