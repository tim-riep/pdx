package reader

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
)

// File ist eine geöffnete PDF-Datei.
type File struct {
	data    []byte
	Trailer Dict
	// offs sind die Positionen der Objekte, inStm die Objekte in
	// Objektstreams (Stream-Nummer und Index).
	offs  map[int]int
	inStm map[int][2]int
	cache map[int]Object
	// busy sind die Objekte, die gerade gelesen werden, um zyklische
	// Verweise (etwa eine Length, die auf sich selbst zeigt) zu erkennen.
	busy map[int]bool
}

// MaxDecoded begrenzt die entpackte Größe eines Streams, damit
// komprimierte Bomben weder Speicher noch Zeit aufbrauchen.
var MaxDecoded int64 = 256 << 20

var reStartXref = regexp.MustCompile(`startxref\s+(\d+)`)

// Open liest die Xref-Daten einer PDF-Datei.
func Open(data []byte) (*File, error) {
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		return nil, fmt.Errorf("keine PDF-Datei")
	}
	f := &File{data: data, offs: map[int]int{}, inStm: map[int][2]int{}, cache: map[int]Object{}, busy: map[int]bool{}}
	ms := reStartXref.FindAllSubmatch(data, -1)
	if ms == nil {
		return nil, fmt.Errorf("startxref fehlt")
	}
	off, _ := strconv.Atoi(string(ms[len(ms)-1][1]))
	seen := map[int]bool{}
	for off > 0 && !seen[off] {
		seen[off] = true
		tr, err := f.xref(off)
		if err != nil {
			return nil, err
		}
		if f.Trailer == nil {
			f.Trailer = tr
		}
		next, ok := tr["Prev"].(int64)
		if !ok {
			break
		}
		off = int(next)
	}
	if _, ok := f.Trailer["Encrypt"]; ok {
		return nil, fmt.Errorf("verschlüsselte PDF-Dateien werden nicht unterstützt")
	}
	return f, nil
}

// xref liest einen Xref-Abschnitt. Ältere Einträge überschreiben neuere
// nicht.
func (f *File) xref(off int) (Dict, error) {
	if off >= len(f.data) {
		return nil, fmt.Errorf("Xref hinter dem Dateiende")
	}
	l := &lexer{b: f.data, pos: off}
	t, err := l.token()
	if err != nil {
		return nil, err
	}
	if t == Operator("xref") {
		return f.xrefTable(l)
	}
	// Xref-Stream: "n g obj << … >> stream".
	l.pos = off
	o, err := f.indirect(l)
	if err != nil {
		return nil, fmt.Errorf("Xref-Stream: %w", err)
	}
	s, ok := o.(*Stream)
	if !ok || s.Dict["Type"] != Name("XRef") {
		return nil, fmt.Errorf("an Position %d steht weder xref noch ein Xref-Stream", off)
	}
	return s.Dict, f.xrefStream(s)
}

func (f *File) xrefTable(l *lexer) (Dict, error) {
	for {
		t, err := l.token()
		if err != nil {
			return nil, err
		}
		if t == Operator("trailer") {
			o, err := l.object(true)
			if err != nil {
				return nil, err
			}
			d, ok := o.(Dict)
			if !ok {
				return nil, fmt.Errorf("Trailer ist kein Dictionary")
			}
			// Hybriddateien verweisen zusätzlich auf einen Xref-Stream.
			if xs, ok := d["XRefStm"].(int64); ok {
				if _, err := f.xref(int(xs)); err != nil {
					return nil, err
				}
			}
			return d, nil
		}
		start, ok1 := t.(int64)
		cnt, err := l.token()
		n, ok2 := cnt.(int64)
		if err != nil || !ok1 || !ok2 {
			return nil, fmt.Errorf("Xref-Tabelle beschädigt")
		}
		for i := 0; i < int(n); i++ {
			o, err1 := l.token()
			_, err2 := l.token()
			k, err3 := l.token()
			if err1 != nil || err2 != nil || err3 != nil {
				return nil, fmt.Errorf("Xref-Tabelle zu kurz")
			}
			num := int(start) + i
			if k == Operator("n") {
				if _, dup := f.offs[num]; !dup {
					if _, dup2 := f.inStm[num]; !dup2 {
						f.offs[num] = int(o.(int64))
					}
				}
			}
		}
	}
}

func (f *File) xrefStream(s *Stream) error {
	data, err := f.Decode(s)
	if err != nil {
		return err
	}
	w, _ := s.Dict["W"].(Array)
	if len(w) != 3 {
		return fmt.Errorf("Xref-Stream ohne gültiges /W")
	}
	var ws [3]int
	for i := range ws {
		v, _ := w[i].(int64)
		ws[i] = int(v)
	}
	size, _ := s.Dict["Size"].(int64)
	index := Array{int64(0), size}
	if idx, ok := s.Dict["Index"].(Array); ok {
		index = idx
	}
	row := ws[0] + ws[1] + ws[2]
	p := 0
	field := func(n int) int {
		v := 0
		for i := 0; i < n; i++ {
			v = v<<8 | int(data[p])
			p++
		}
		return v
	}
	for i := 0; i+1 < len(index); i += 2 {
		start, _ := index[i].(int64)
		n, _ := index[i+1].(int64)
		for j := 0; j < int(n); j++ {
			if p+row > len(data) {
				return fmt.Errorf("Xref-Stream zu kurz")
			}
			typ := 1
			if ws[0] > 0 {
				typ = field(ws[0])
			}
			a, b := field(ws[1]), field(ws[2])
			num := int(start) + j
			_, d1 := f.offs[num]
			_, d2 := f.inStm[num]
			if d1 || d2 {
				continue
			}
			switch typ {
			case 1:
				f.offs[num] = a
			case 2:
				f.inStm[num] = [2]int{a, b}
			}
		}
	}
	return nil
}

// indirect liest "n g obj … endobj" ab der aktuellen Position.
func (f *File) indirect(l *lexer) (Object, error) {
	for i := 0; i < 3; i++ {
		if _, err := l.token(); err != nil {
			return nil, err
		}
	}
	o, err := l.object(true)
	if err != nil {
		return nil, err
	}
	d, ok := o.(Dict)
	if !ok {
		return o, nil
	}
	save := l.pos
	t, err := l.token()
	if err != nil || t != Operator("stream") {
		l.pos = save
		return d, nil
	}
	// Nach "stream" folgt CRLF oder LF.
	if l.pos < len(l.b) && l.b[l.pos] == '\r' {
		l.pos++
	}
	if l.pos < len(l.b) && l.b[l.pos] == '\n' {
		l.pos++
	}
	start := l.pos
	n := -1
	switch v := d["Length"].(type) {
	case int64:
		n = int(v)
	case Ref:
		if lo, err := f.Get(v); err == nil {
			if k, ok := lo.(int64); ok {
				n = int(k)
			}
		}
	}
	if n < 0 || start+n > len(l.b) {
		// Länge unbrauchbar, bis endstream suchen.
		e := bytes.Index(l.b[start:], []byte("endstream"))
		if e < 0 {
			return nil, errEOF
		}
		n = e
	}
	return &Stream{Dict: d, Raw: l.b[start : start+n]}, nil
}

// Get liefert das Objekt mit der Nummer des Verweises.
func (f *File) Get(r Ref) (Object, error) {
	if o, ok := f.cache[r.Num]; ok {
		return o, nil
	}
	if f.busy[r.Num] {
		return nil, fmt.Errorf("Objekt %d verweist zyklisch auf sich selbst", r.Num)
	}
	f.busy[r.Num] = true
	defer delete(f.busy, r.Num)
	var o Object
	var err error
	if off, ok := f.offs[r.Num]; ok {
		o, err = f.indirect(&lexer{b: f.data, pos: off})
	} else if at, ok := f.inStm[r.Num]; ok {
		o, err = f.fromObjStm(at[0], at[1])
	} else {
		return nil, nil // fehlende Objekte gelten als null
	}
	if err != nil {
		return nil, fmt.Errorf("Objekt %d: %w", r.Num, err)
	}
	f.cache[r.Num] = o
	return o, nil
}

func (f *File) fromObjStm(num, idx int) (Object, error) {
	o, err := f.Get(Ref{num, 0})
	if err != nil {
		return nil, err
	}
	s, ok := o.(*Stream)
	if !ok {
		return nil, fmt.Errorf("Objektstream %d fehlt", num)
	}
	data, err := f.Decode(s)
	if err != nil {
		return nil, err
	}
	n, _ := s.Dict["N"].(int64)
	first, _ := s.Dict["First"].(int64)
	if idx >= int(n) {
		return nil, fmt.Errorf("Index %d im Objektstream %d fehlt", idx, num)
	}
	l := &lexer{b: data}
	var off int64
	for i := 0; i <= idx; i++ {
		l.token()
		t, err := l.token()
		if err != nil {
			return nil, err
		}
		off, _ = t.(int64)
	}
	l.pos = int(first + off)
	return l.object(true)
}

// Resolve löst Verweise auf, andere Objekte bleiben unverändert.
func (f *File) Resolve(o Object) Object {
	for i := 0; i < 32; i++ {
		r, ok := o.(Ref)
		if !ok {
			return o
		}
		o, _ = f.Get(r)
	}
	return nil
}

// Dict liefert ein aufgelöstes Dictionary oder das eines Streams.
func (f *File) Dict(o Object) Dict {
	switch v := f.Resolve(o).(type) {
	case Dict:
		return v
	case *Stream:
		return v.Dict
	}
	return nil
}

// Array liefert ein aufgelöstes Array.
func (f *File) Array(o Object) Array {
	a, _ := f.Resolve(o).(Array)
	return a
}

// Int liefert eine aufgelöste Ganzzahl.
func (f *File) Int(o Object) (int, bool) {
	v, ok := f.Resolve(o).(int64)
	return int(v), ok
}

// Root liefert den Katalog.
func (f *File) Root() Dict { return f.Dict(f.Trailer["Root"]) }

// Pages liefert die Seiten in Dokumentreihenfolge mit ihren Verweisen.
func (f *File) Pages() ([]Dict, []Ref) {
	var ds []Dict
	var rs []Ref
	var walk func(o Object, depth int)
	walk = func(o Object, depth int) {
		if depth > 64 {
			return
		}
		d := f.Dict(o)
		if d == nil {
			return
		}
		if d["Type"] == Name("Pages") || d["Kids"] != nil {
			for _, k := range f.Array(d["Kids"]) {
				walk(k, depth+1)
			}
			return
		}
		ds = append(ds, d)
		r, _ := o.(Ref)
		rs = append(rs, r)
	}
	walk(f.Root()["Pages"], 0)
	return ds, rs
}

// Decode liefert die entpackten Bytes eines Streams. Unterstützt werden
// FlateDecode mit PNG-Prädiktor und ungefilterte Streams.
func (f *File) Decode(s *Stream) ([]byte, error) {
	var filters Array
	switch v := f.Resolve(s.Dict["Filter"]).(type) {
	case Name:
		filters = Array{v}
	case Array:
		filters = v
	}
	parms := f.Resolve(s.Dict["DecodeParms"])
	data := s.Raw
	for i, flt := range filters {
		switch f.Resolve(flt) {
		case Name("FlateDecode"):
			zr, err := zlib.NewReader(bytes.NewReader(data))
			if err != nil {
				return nil, err
			}
			out, err := io.ReadAll(io.LimitReader(zr, MaxDecoded+1))
			if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
				return nil, err
			}
			if int64(len(out)) > MaxDecoded {
				return nil, fmt.Errorf("Stream ist entpackt größer als %d Bytes", MaxDecoded)
			}
			var p Dict
			switch v := parms.(type) {
			case Dict:
				p = v
			case Array:
				if i < len(v) {
					p = f.Dict(v[i])
				}
			}
			if out, err = unpredict(out, p); err != nil {
				return nil, err
			}
			data = out
		default:
			return nil, fmt.Errorf("Filter %v wird nicht unterstützt", flt)
		}
	}
	return data, nil
}

// unpredict hebt den PNG-Prädiktor auf (ISO 32000-1 7.4.4.4).
func unpredict(b []byte, p Dict) ([]byte, error) {
	pred, _ := p["Predictor"].(int64)
	if pred < 10 {
		return b, nil
	}
	cols := int64(1)
	if c, ok := p["Columns"].(int64); ok {
		cols = c
	}
	row := int(cols)
	var out []byte
	prev := make([]byte, row)
	for i := 0; i+row+1 <= len(b); i += row + 1 {
		typ, cur := b[i], append([]byte(nil), b[i+1:i+1+row]...)
		for j := range cur {
			var left, up, ul byte
			if j > 0 {
				left, ul = cur[j-1], prev[j-1]
			}
			up = prev[j]
			switch typ {
			case 1:
				cur[j] += left
			case 2:
				cur[j] += up
			case 3:
				cur[j] += byte((int(left) + int(up)) / 2)
			case 4:
				cur[j] += paeth(left, up, ul)
			}
		}
		out = append(out, cur...)
		prev = cur
	}
	return out, nil
}

func paeth(a, b, c byte) byte {
	p := int(a) + int(b) - int(c)
	pa, pb, pc := abs(p-int(a)), abs(p-int(b)), abs(p-int(c))
	switch {
	case pa <= pb && pa <= pc:
		return a
	case pb <= pc:
		return b
	}
	return c
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
