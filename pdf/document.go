package pdf

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"io"
	"strconv"
)

// Document sammelt indirekte Objekte und schreibt sie als PDF-Datei.
//
// Objektnummern werden in der Reihenfolge von Add und Reserve vergeben. Vor
// dem Schreiben prüft Write den gesamten Objektgraphen gegen die Policy und
// verweigert Dateien, die eine nicht erlaubte Funktion enthalten.
type Document struct {
	objs   []Object
	policy Policy

	// Root verweist auf den Katalog. Pflicht.
	Root Ref
	// Info verweist auf das Info-Dictionary. Optional.
	Info Ref
}

// NewDocument legt ein leeres Dokument an. Eine Policy nil erlaubt alles außer
// den Funktionen, die pdx grundsätzlich nicht schreibt.
func NewDocument(p Policy) *Document {
	return &Document{policy: p}
}

// Policy liefert die Policy des Dokuments, eventuell nil.
func (d *Document) Policy() Policy { return d.policy }

// Add legt ein indirektes Objekt an.
func (d *Document) Add(o Object) Ref {
	if o == nil {
		panic("pdf: Add mit nil")
	}
	d.objs = append(d.objs, o)
	return Ref{Num: len(d.objs)}
}

// Reserve vergibt eine Objektnummer, deren Inhalt später mit Set folgt.
func (d *Document) Reserve() Ref {
	d.objs = append(d.objs, nil)
	return Ref{Num: len(d.objs)}
}

// Set füllt eine reservierte oder ersetzt eine vorhandene Objektnummer.
func (d *Document) Set(r Ref, o Object) {
	d.objs[r.Num-1] = o
}

// Get liefert das Objekt hinter einer Referenz oder nil.
func (d *Document) Get(r Ref) Object {
	if r.Num < 1 || r.Num > len(d.objs) {
		return nil
	}
	return d.objs[r.Num-1]
}

// Resolve löst Referenzen auf und liefert direkte Objekte unverändert.
func (d *Document) Resolve(o Object) Object {
	for i := 0; i < 32; i++ {
		r, ok := o.(Ref)
		if !ok {
			return o
		}
		o = d.Get(r)
	}
	return nil
}

// NumObjects liefert die Anzahl der indirekten Objekte.
func (d *Document) NumObjects() int { return len(d.objs) }

// Use prüft, ob eine Funktion genutzt werden darf.
func (d *Document) Use(f Feature) error {
	name := "pdx"
	if d.policy != nil {
		name = d.policy.Name()
	}
	if reason, ok := alwaysForbidden[f]; ok {
		return &FeatureError{Feature: f, Profile: name, Reason: reason}
	}
	if d.policy == nil {
		return nil
	}
	return d.policy.Allow(f)
}

// Header ist der Dateikopf mit Binärkommentar nach ISO 32000 7.5.2.
const Header = "%PDF-1.7\n%\xe2\xe3\xcf\xd3\n"

// Bytes schreibt das Dokument in einen Puffer.
func (d *Document) Bytes() ([]byte, error) {
	var b bytes.Buffer
	if err := d.Write(&b); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// Write prüft das Dokument und schreibt es. Die Datei-ID im Trailer ist der
// MD5-Hash aller Bytes vor der Xref-Tabelle, beide Einträge sind gleich.
func (d *Document) Write(w io.Writer) error {
	if d.Root.Num == 0 {
		return fmt.Errorf("pdf: kein Katalog gesetzt")
	}
	for i, o := range d.objs {
		if o == nil {
			return fmt.Errorf("pdf: Objekt %d ist reserviert, aber nicht gefüllt", i+1)
		}
	}
	if err := d.check(); err != nil {
		return err
	}

	var b bytes.Buffer
	b.WriteString(Header)
	offsets := make([]int, len(d.objs))
	for i, o := range d.objs {
		offsets[i] = b.Len()
		b.WriteString(strconv.Itoa(i + 1))
		b.WriteString(" 0 obj\n")
		o.writePDF(&b)
		b.WriteString("\nendobj\n")
	}
	id := md5.Sum(b.Bytes())

	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f\r\n", len(d.objs)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n\r\n", off)
	}
	trailer := D(
		"Size", Int(len(d.objs)+1),
		"Root", d.Root,
		"ID", Array{HexString(id[:]), HexString(id[:])},
	)
	if d.Info.Num != 0 {
		trailer.Set("Info", d.Info)
	}
	b.WriteString("trailer\n")
	trailer.writePDF(&b)
	fmt.Fprintf(&b, "\nstartxref\n%d\n%%%%EOF\n", xref)
	_, err := w.Write(b.Bytes())
	return err
}

// check durchsucht den Objektgraphen nach Funktionen, die die Policy freigeben
// muss. Das ist die zweite Verteidigungslinie hinter den Aufrufen von Use in
// den höheren Schichten.
func (d *Document) check() error {
	found := map[Feature]bool{}
	for _, o := range d.objs {
		scan(o, found)
	}
	for f := Transparency; f <= UnembeddedFont; f++ {
		if found[f] {
			if err := d.Use(f); err != nil {
				return err
			}
		}
	}
	return nil
}

func scan(o Object, found map[Feature]bool) {
	switch v := o.(type) {
	case Array:
		for _, e := range v {
			scan(e, found)
		}
	case *Stream:
		scanDict(v.Dict, found, true)
	case *Dict:
		scanDict(v, found, false)
	}
}

func scanDict(d *Dict, found map[Feature]bool, isStream bool) {
	if n, ok := d.Get("S").(Name); ok && n == "JavaScript" {
		found[JavaScript] = true
	}
	if d.Has("JS") {
		found[JavaScript] = true
	}
	if d.Has("XFA") {
		found[XFA] = true
	}
	if d.Has("Encrypt") {
		found[Encryption] = true
	}
	if isStream {
		if hasName(d.Get("Filter"), "LZWDecode") {
			found[LZW] = true
		}
		if d.Has("F") || d.Has("FFilter") || d.Has("FDecodeParms") {
			found[ExternalContent] = true
		}
	}
	typ, _ := d.Get("Type").(Name)
	switch typ {
	case "EmbeddedFile":
		found[Attachment] = true
	case "FontDescriptor":
		if !d.Has("FontFile") && !d.Has("FontFile2") && !d.Has("FontFile3") {
			found[UnembeddedFont] = true
		}
	case "Font":
		sub, _ := d.Get("Subtype").(Name)
		if (sub == "Type1" || sub == "TrueType" || sub == "MMType1") && !d.Has("FontDescriptor") {
			found[UnembeddedFont] = true
		}
	}
	if sm := d.Get("SMask"); sm != nil {
		if n, isName := sm.(Name); !isName || n != "None" {
			found[Transparency] = true
		}
	}
	for _, k := range []Name{"CA", "ca"} {
		if alphaBelowOne(d.Get(k)) {
			found[Transparency] = true
		}
	}
	if bm, ok := d.Get("BM").(Name); ok && bm != "Normal" && bm != "Compatible" {
		found[Transparency] = true
	}
	if n, ok := d.Get("Type").(Name); ok && n == "Filespec" && !d.Has("EF") {
		found[ExternalContent] = true
	}
	for _, k := range d.Keys() {
		scan(d.Get(k), found)
	}
}

func hasName(o Object, n Name) bool {
	switch v := o.(type) {
	case Name:
		return v == n
	case Array:
		for _, e := range v {
			if hasName(e, n) {
				return true
			}
		}
	}
	return false
}

func alphaBelowOne(o Object) bool {
	switch v := o.(type) {
	case Real:
		return v < 1
	case Int:
		return v < 1
	}
	return false
}
