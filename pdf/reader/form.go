package reader

import "fmt"

// Field ist ein Formularfeld mit den von Eltern geerbten Angaben
// (ISO 32000-1 12.7.3.1).
type Field struct {
	// Name ist der volle Feldname aus den Teilnamen T, mit Punkt getrennt.
	Name string
	// Type ist FT: Tx, Btn, Ch oder Sig.
	Type Name
	// Value ist V als Text: Textstring, bei Schaltflächen der Name des
	// Zustands, bei Listen mit Mehrfachauswahl der erste Wert. Empty meldet,
	// dass V fehlt.
	Value string
	Empty bool
	Flags int
	// TU ist der zugängliche Name, TM der Name für den Export.
	TU, TM string
}

// Fields liefert die Felder des AcroForm in Dokumentreihenfolge. Felder ohne
// eigene Felder unter sich (Endfelder) erscheinen, Zwischenknoten nicht.
func (f *File) Fields() []Field {
	acro := f.Dict(f.Root()["AcroForm"])
	if acro == nil {
		return nil
	}
	var out []Field
	type inherited struct {
		name  string
		typ   Name
		v     Object
		flags int
	}
	seen := map[Ref]bool{}
	var walk func(o Object, in inherited, depth int)
	walk = func(o Object, in inherited, depth int) {
		if r, ok := o.(Ref); ok {
			if seen[r] {
				return
			}
			seen[r] = true
		}
		d := f.Dict(o)
		if d == nil || depth > 32 {
			return
		}
		if t, ok := f.Resolve(d["T"]).(String); ok {
			part := TextString(t)
			if in.name != "" {
				in.name += "." + part
			} else {
				in.name = part
			}
		}
		if ft, ok := f.Resolve(d["FT"]).(Name); ok {
			in.typ = ft
		}
		if v, ok := d["V"]; ok {
			in.v = v
		}
		if ff, ok := f.Int(d["Ff"]); ok {
			in.flags = ff
		}
		// Kinder mit eigenem T sind Felder, ohne T nur Widgets.
		var fieldKids []Object
		for _, k := range f.Array(d["Kids"]) {
			if kd := f.Dict(k); kd != nil && kd["T"] != nil {
				fieldKids = append(fieldKids, k)
			}
		}
		if len(fieldKids) > 0 {
			for _, k := range fieldKids {
				walk(k, in, depth+1)
			}
			return
		}
		fl := Field{Name: in.name, Type: in.typ, Flags: in.flags}
		if s, ok := f.Resolve(d["TU"]).(String); ok {
			fl.TU = TextString(s)
		}
		if s, ok := f.Resolve(d["TM"]).(String); ok {
			fl.TM = TextString(s)
		}
		switch v := f.Resolve(in.v).(type) {
		case nil:
			fl.Empty = true
		case String:
			fl.Value = TextString(v)
		case Name:
			fl.Value = string(v)
		case Array:
			if len(v) > 0 {
				if s, ok := f.Resolve(v[0]).(String); ok {
					fl.Value = TextString(s)
				}
			}
		}
		out = append(out, fl)
	}
	for _, o := range f.Array(acro["Fields"]) {
		walk(o, inherited{}, 0)
	}
	return out
}

// Field sucht ein Feld nach vollem Namen.
func (f *File) Field(name string) (Field, bool) {
	for _, fl := range f.Fields() {
		if fl.Name == name {
			return fl, true
		}
	}
	return Field{}, false
}

// EmbeddedFiles liefert die eingebetteten Dateien aus dem Namensbaum
// EmbeddedFiles (ISO 32000-1 7.11.4) nach Namen.
func (f *File) EmbeddedFiles() (map[string][]byte, error) {
	out := map[string][]byte{}
	names := f.Dict(f.Root()["Names"])
	if names == nil {
		return out, nil
	}
	var firstErr error
	var walk func(o Object, depth int)
	walk = func(o Object, depth int) {
		d := f.Dict(o)
		if d == nil || depth > 32 {
			return
		}
		kv := f.Array(d["Names"])
		for i := 0; i+1 < len(kv); i += 2 {
			key, ok := f.Resolve(kv[i]).(String)
			if !ok {
				continue
			}
			spec := f.Dict(kv[i+1])
			ef := f.Dict(spec["EF"])
			s, ok := f.Resolve(ef["F"]).(*Stream)
			if !ok {
				continue
			}
			data, err := f.Decode(s)
			if err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("eingebettete Datei %s: %w", TextString(key), err)
				}
				continue
			}
			out[TextString(key)] = data
		}
		for _, k := range f.Array(d["Kids"]) {
			walk(k, depth+1)
		}
	}
	walk(names["EmbeddedFiles"], 0)
	return out, firstErr
}
