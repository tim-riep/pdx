// Package reader liest PDF-Dateien so weit, wie pdx es zum Prüfen braucht:
// Objekte über Xref-Tabellen und Xref-Streams, Objektstreams, Flate und
// Content Streams. Verschlüsselte Dateien werden abgelehnt.
package reader

import (
	"bytes"
	"fmt"
	"strconv"
)

// Name ist ein PDF-Name ohne Schrägstrich.
type Name string

// String ist eine PDF-Zeichenkette als Bytes.
type String []byte

// Array ist ein PDF-Array.
type Array []Object

// Dict ist ein PDF-Dictionary.
type Dict map[Name]Object

// Ref ist ein indirekter Verweis.
type Ref struct{ Num, Gen int }

// Stream ist ein Stream mit Dictionary und rohen Bytes.
type Stream struct {
	Dict Dict
	Raw  []byte
}

// Operator ist ein Operator in einem Content Stream.
type Operator string

// Object ist eines von nil, bool, int64, float64, Name, String, Array, Dict,
// Ref, *Stream oder Operator.
type Object any

type lexer struct {
	b     []byte
	pos   int
	depth int // Verschachtelung von Arrays und Dictionaries
}

// maxDepth begrenzt die Verschachtelung, damit bösartige Eingaben den
// Stapel nicht sprengen.
const maxDepth = 256

func isSpace(c byte) bool {
	return c == ' ' || c == '\n' || c == '\r' || c == '\t' || c == '\f' || c == 0
}

func isDelim(c byte) bool {
	switch c {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return false
}

func (l *lexer) skip() {
	for l.pos < len(l.b) {
		c := l.b[l.pos]
		switch {
		case isSpace(c):
			l.pos++
		case c == '%':
			for l.pos < len(l.b) && l.b[l.pos] != '\n' && l.b[l.pos] != '\r' {
				l.pos++
			}
		default:
			return
		}
	}
}

// token liefert das nächste Grundelement. Klammern von Arrays und
// Dictionaries kommen als Operator "[", "]", "<<", ">>".
func (l *lexer) token() (Object, error) {
	l.skip()
	if l.pos >= len(l.b) {
		return nil, errEOF
	}
	c := l.b[l.pos]
	switch {
	case c == '/':
		l.pos++
		start := l.pos
		for l.pos < len(l.b) && !isSpace(l.b[l.pos]) && !isDelim(l.b[l.pos]) {
			l.pos++
		}
		return Name(unescapeName(l.b[start:l.pos])), nil
	case c == '(':
		return l.literal()
	case c == '<':
		if l.pos+1 < len(l.b) && l.b[l.pos+1] == '<' {
			l.pos += 2
			return Operator("<<"), nil
		}
		return l.hex()
	case c == '>':
		if l.pos+1 < len(l.b) && l.b[l.pos+1] == '>' {
			l.pos += 2
			return Operator(">>"), nil
		}
		return nil, fmt.Errorf("unerwartetes > an Position %d", l.pos)
	case c == '[' || c == ']' || c == '{' || c == '}':
		l.pos++
		return Operator(string(c)), nil
	}
	start := l.pos
	for l.pos < len(l.b) && !isSpace(l.b[l.pos]) && !isDelim(l.b[l.pos]) {
		l.pos++
	}
	w := l.b[start:l.pos]
	if len(w) == 0 {
		l.pos++
		return nil, fmt.Errorf("unerwartetes Zeichen %q an Position %d", c, start)
	}
	switch string(w) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "null":
		return nil, nil
	}
	if (w[0] >= '0' && w[0] <= '9') || w[0] == '-' || w[0] == '+' || w[0] == '.' {
		if bytes.IndexByte(w, '.') < 0 {
			if n, err := strconv.ParseInt(string(w), 10, 64); err == nil {
				return n, nil
			}
		}
		if f, err := strconv.ParseFloat(string(w), 64); err == nil {
			return f, nil
		}
	}
	return Operator(w), nil
}

var errEOF = fmt.Errorf("unerwartetes Dateiende")

func unescapeName(b []byte) string {
	if bytes.IndexByte(b, '#') < 0 {
		return string(b)
	}
	var out []byte
	for i := 0; i < len(b); i++ {
		if b[i] == '#' && i+2 < len(b) {
			if v, err := strconv.ParseUint(string(b[i+1:i+3]), 16, 8); err == nil {
				out = append(out, byte(v))
				i += 2
				continue
			}
		}
		out = append(out, b[i])
	}
	return string(out)
}

func (l *lexer) literal() (Object, error) {
	l.pos++ // (
	var out []byte
	depth := 1
	for l.pos < len(l.b) {
		c := l.b[l.pos]
		l.pos++
		switch c {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return String(out), nil
			}
		case '\\':
			if l.pos >= len(l.b) {
				return nil, errEOF
			}
			e := l.b[l.pos]
			l.pos++
			switch e {
			case 'n':
				c = '\n'
			case 'r':
				c = '\r'
			case 't':
				c = '\t'
			case 'b':
				c = '\b'
			case 'f':
				c = '\f'
			case '\r':
				if l.pos < len(l.b) && l.b[l.pos] == '\n' {
					l.pos++
				}
				continue
			case '\n':
				continue
			default:
				if e >= '0' && e <= '7' {
					v := int(e - '0')
					for k := 0; k < 2 && l.pos < len(l.b) && l.b[l.pos] >= '0' && l.b[l.pos] <= '7'; k++ {
						v = v*8 + int(l.b[l.pos]-'0')
						l.pos++
					}
					c = byte(v)
				} else {
					c = e
				}
			}
		}
		out = append(out, c)
	}
	return nil, errEOF
}

func (l *lexer) hex() (Object, error) {
	l.pos++ // <
	var out []byte
	var hi int = -1
	for l.pos < len(l.b) {
		c := l.b[l.pos]
		l.pos++
		if c == '>' {
			if hi >= 0 {
				out = append(out, byte(hi<<4))
			}
			return String(out), nil
		}
		v := hexVal(c)
		if v < 0 {
			continue
		}
		if hi < 0 {
			hi = v
		} else {
			out = append(out, byte(hi<<4|v))
			hi = -1
		}
	}
	return nil, errEOF
}

func hexVal(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

// object liest ein vollständiges Objekt. refs erlaubt "n g R" (nicht in
// Content Streams).
func (l *lexer) object(refs bool) (Object, error) {
	t, err := l.token()
	if err != nil {
		return nil, err
	}
	return l.complete(t, refs)
}

func (l *lexer) complete(t Object, refs bool) (Object, error) {
	switch v := t.(type) {
	case Operator:
		if v == "[" || v == "<<" {
			if l.depth >= maxDepth {
				return nil, fmt.Errorf("Verschachtelung tiefer als %d an Position %d", maxDepth, l.pos)
			}
			l.depth++
			defer func() { l.depth-- }()
		}
		switch v {
		case "[":
			var arr Array
			for {
				t, err := l.token()
				if err != nil {
					return nil, err
				}
				if t == Operator("]") {
					return arr, nil
				}
				o, err := l.complete(t, refs)
				if err != nil {
					return nil, err
				}
				arr = append(arr, o)
			}
		case "<<":
			d := Dict{}
			for {
				t, err := l.token()
				if err != nil {
					return nil, err
				}
				if t == Operator(">>") {
					return d, nil
				}
				k, ok := t.(Name)
				if !ok {
					return nil, fmt.Errorf("Schlüssel im Dictionary ist kein Name: %v", t)
				}
				o, err := l.object(refs)
				if err != nil {
					return nil, err
				}
				d[k] = o
			}
		}
	case int64:
		if !refs {
			return v, nil
		}
		// Vorausschauen auf "g R".
		save := l.pos
		t2, err := l.token()
		if g, ok := t2.(int64); err == nil && ok {
			t3, err := l.token()
			if err == nil && t3 == Operator("R") {
				return Ref{int(v), int(g)}, nil
			}
		}
		l.pos = save
		return v, nil
	}
	return t, nil
}
