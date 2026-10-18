// Package incr ändert fertige PDF-Dateien per inkrementellem Update
// (ISO 32000-1 7.5.6): Die ursprünglichen Bytes bleiben unverändert, neue
// und geänderte Objekte folgen samt eigener Xref und einem Trailer mit Prev.
// So bleibt eine bestehende Signatur gültig und PDF/A-Konformität erhalten,
// wenn die Änderung selbst konform ist. Signieren baut darauf auf.
package incr

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"sort"
	"strconv"

	"github.com/tim-riep/pdx/pdf"
	"github.com/tim-riep/pdx/pdf/reader"
)

// Update sammelt die Änderungen an einer Datei.
type Update struct {
	base []byte
	f    *reader.File
	objs map[int]pdf.Object
	next int
}

// Open liest eine Datei für ein Update.
func Open(base []byte) (*Update, error) {
	f, err := reader.Open(base)
	if err != nil {
		return nil, err
	}
	if f.Size() <= 0 {
		return nil, fmt.Errorf("Trailer ohne gültige Size")
	}
	if _, ok := f.Trailer["Root"].(reader.Ref); !ok {
		return nil, fmt.Errorf("Trailer ohne Root")
	}
	return &Update{base: base, f: f, objs: map[int]pdf.Object{}, next: f.Size()}, nil
}

// File liefert die gelesene Ausgangsdatei.
func (u *Update) File() *reader.File { return u.f }

// Root liefert den Verweis auf den Katalog.
func (u *Update) Root() pdf.Ref {
	r := u.f.Trailer["Root"].(reader.Ref)
	return pdf.Ref{Num: r.Num}
}

// Add legt ein neues Objekt an.
func (u *Update) Add(o pdf.Object) pdf.Ref {
	r := u.Reserve()
	u.objs[r.Num] = o
	return r
}

// Reserve vergibt eine neue Objektnummer, deren Inhalt mit Set folgt.
func (u *Update) Reserve() pdf.Ref {
	r := pdf.Ref{Num: u.next}
	u.next++
	u.objs[r.Num] = nil
	return r
}

// Set ersetzt ein vorhandenes Objekt oder füllt ein reserviertes.
func (u *Update) Set(r pdf.Ref, o pdf.Object) { u.objs[r.Num] = o }

// Object liefert ein vorhandenes Objekt als Writer-Objekt, etwa um den
// Katalog zu ändern und mit Set zu ersetzen.
func (u *Update) Object(r pdf.Ref) (pdf.Object, error) {
	o, err := u.f.Get(reader.Ref{Num: r.Num})
	if err != nil {
		return nil, err
	}
	return Convert(o)
}

// Dict liefert ein vorhandenes Dictionary als Writer-Objekt.
func (u *Update) Dict(r pdf.Ref) (*pdf.Dict, error) {
	o, err := u.Object(r)
	if err != nil {
		return nil, err
	}
	d, ok := o.(*pdf.Dict)
	if !ok {
		return nil, fmt.Errorf("Objekt %d ist kein Dictionary", r.Num)
	}
	return d, nil
}

// Convert wandelt ein Objekt des Readers in ein Writer-Objekt. Streams
// behalten ihre kodierten Bytes und Filter. Verweise mit Generation über 0
// lassen sich nicht ausdrücken und sind ein Fehler.
func Convert(o reader.Object) (pdf.Object, error) {
	switch v := o.(type) {
	case nil:
		return pdf.Null{}, nil
	case bool:
		return pdf.Bool(v), nil
	case int64:
		return pdf.Int(v), nil
	case float64:
		return pdf.Real(v), nil
	case reader.Name:
		return pdf.Name(v), nil
	case reader.String:
		return pdf.String(append([]byte(nil), v...)), nil
	case reader.Ref:
		if v.Gen != 0 {
			return nil, fmt.Errorf("Verweis %d %d R mit Generation über 0 wird nicht unterstützt", v.Num, v.Gen)
		}
		return pdf.Ref{Num: v.Num}, nil
	case reader.Array:
		out := make(pdf.Array, 0, len(v))
		for _, e := range v {
			c, err := Convert(e)
			if err != nil {
				return nil, err
			}
			out = append(out, c)
		}
		return out, nil
	case reader.Dict:
		return convertDict(v)
	case *reader.Stream:
		d, err := convertDict(v.Dict)
		if err != nil {
			return nil, err
		}
		return &pdf.Stream{Dict: d, Data: append([]byte(nil), v.Raw...)}, nil
	}
	return nil, fmt.Errorf("Objekt vom Typ %T lässt sich nicht übernehmen", o)
}

func convertDict(v reader.Dict) (*pdf.Dict, error) {
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, string(k))
	}
	sort.Strings(keys)
	d := pdf.NewDict()
	for _, k := range keys {
		c, err := Convert(v[reader.Name(k)])
		if err != nil {
			return nil, err
		}
		d.Set(pdf.Name(k), c)
	}
	return d, nil
}

// Result ist eine geschriebene Datei mit den Offsets der neuen Objekte.
type Result struct {
	Data    []byte
	Offsets map[int]int
}

// Bytes schreibt die Ausgangsdatei mit dem angehängten Update. Die Xref folgt
// der Art der Ausgangsdatei (Tabelle oder Stream). Die Datei-ID behält den
// ersten Teil und bekommt als zweiten den MD5 der angehängten Bytes. Nach
// %%EOF folgt nur ein Zeilenwechsel (ISO 19005-2 6.1.3).
func (u *Update) Bytes() (*Result, error) {
	nums := make([]int, 0, len(u.objs))
	for n, o := range u.objs {
		if o == nil {
			return nil, fmt.Errorf("Objekt %d ist reserviert, aber nicht gefüllt", n)
		}
		nums = append(nums, n)
	}
	if len(nums) == 0 {
		return nil, fmt.Errorf("das Update enthält keine Objekte")
	}
	sort.Ints(nums)
	var b bytes.Buffer
	b.Write(u.base)
	if !bytes.HasSuffix(u.base, []byte("\n")) {
		b.WriteByte('\n')
	}
	start := b.Len()
	res := &Result{Offsets: map[int]int{}}
	for _, n := range nums {
		res.Offsets[n] = b.Len()
		b.WriteString(strconv.Itoa(n) + " 0 obj\n")
		b.WriteString(pdf.Serialize(u.objs[n]))
		b.WriteString("\nendobj\n")
	}
	sum := md5.Sum(b.Bytes()[start:])
	id0 := sum[:]
	if ids, ok := u.f.Trailer["ID"].(reader.Array); ok && len(ids) > 0 {
		if s, ok := u.f.Resolve(ids[0]).(reader.String); ok {
			id0 = s
		}
	}
	size := max(u.next, u.f.Size())
	trailer := pdf.D(
		"Root", u.Root(),
		"Prev", pdf.Int(u.f.StartXref()),
		"ID", pdf.Array{pdf.HexString(id0), pdf.HexString(sum[:])},
	)
	if info, ok := u.f.Trailer["Info"].(reader.Ref); ok {
		trailer.Set("Info", pdf.Ref{Num: info.Num})
	}
	xref := b.Len()
	if u.f.XRefStream() {
		// Der Xref-Stream bekommt die nächste Nummer und steht in sich selbst.
		self := size
		size++
		res.Offsets[self] = xref
		all := append(append([]int(nil), nums...), self)
		var rows bytes.Buffer
		var index pdf.Array
		for i := 0; i < len(all); {
			j := i + 1
			for j < len(all) && all[j] == all[j-1]+1 {
				j++
			}
			index = append(index, pdf.Int(all[i]), pdf.Int(j-i))
			for _, n := range all[i:j] {
				rows.WriteByte(1)
				var off [4]byte
				binary.BigEndian.PutUint32(off[:], uint32(res.Offsets[n]))
				rows.Write(off[:])
				rows.Write([]byte{0, 0})
			}
			i = j
		}
		trailer.Set("Type", pdf.Name("XRef"))
		trailer.Set("Size", pdf.Int(size))
		trailer.Set("W", pdf.Array{pdf.Int(1), pdf.Int(4), pdf.Int(2)})
		trailer.Set("Index", index)
		b.WriteString(strconv.Itoa(self) + " 0 obj\n")
		b.WriteString(pdf.Serialize(&pdf.Stream{Dict: trailer, Data: rows.Bytes()}))
		b.WriteString("\nendobj\n")
	} else {
		b.WriteString("xref\n")
		for i := 0; i < len(nums); {
			j := i + 1
			for j < len(nums) && nums[j] == nums[j-1]+1 {
				j++
			}
			fmt.Fprintf(&b, "%d %d\n", nums[i], j-i)
			for _, n := range nums[i:j] {
				fmt.Fprintf(&b, "%010d 00000 n\r\n", res.Offsets[n])
			}
			i = j
		}
		trailer.Set("Size", pdf.Int(size))
		b.WriteString("trailer\n")
		b.WriteString(pdf.Serialize(trailer))
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "startxref\n%d\n%%%%EOF\n", xref)
	res.Data = b.Bytes()
	return res, nil
}
