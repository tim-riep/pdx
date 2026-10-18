package reader

import (
	"strings"
	"unicode/utf16"
)

// font dekodiert Text einer Schrift über ihre ToUnicode-CMap.
type font struct {
	bytes int // Bytes je Code
	uni   map[uint32]string
}

func (f *File) loadFont(d Dict) *font {
	ft := &font{bytes: 1, uni: map[uint32]string{}}
	if d["Subtype"] == Name("Type0") {
		ft.bytes = 2
	}
	s, ok := f.Resolve(d["ToUnicode"]).(*Stream)
	if !ok {
		return ft
	}
	data, err := f.Decode(s)
	if err != nil {
		return ft
	}
	l := &lexer{b: data}
	var stack []Object
	for {
		t, err := l.object(false)
		if err != nil {
			break
		}
		op, isOp := t.(Operator)
		if !isOp {
			stack = append(stack, t)
			continue
		}
		switch op {
		case "endcodespacerange":
			if len(stack) >= 1 {
				if b, ok := stack[0].(String); ok && len(b) > 0 {
					ft.bytes = len(b)
				}
			}
		case "endbfchar":
			for i := 0; i+1 < len(stack); i += 2 {
				src, ok1 := stack[i].(String)
				dst, ok2 := stack[i+1].(String)
				if ok1 && ok2 {
					ft.uni[code(src)] = utf16be(dst)
				}
			}
		case "endbfrange":
			for i := 0; i+2 < len(stack); i += 3 {
				lo, ok1 := stack[i].(String)
				hi, ok2 := stack[i+1].(String)
				if !ok1 || !ok2 {
					continue
				}
				a, b := code(lo), code(hi)
				switch dst := stack[i+2].(type) {
				case String:
					base := append([]byte(nil), dst...)
					for c := a; c <= b && c-a < 65536; c++ {
						ft.uni[c] = utf16be(base)
						if len(base) > 0 {
							base[len(base)-1]++
						}
					}
				case Array:
					for k, o := range dst {
						if s, ok := o.(String); ok {
							ft.uni[a+uint32(k)] = utf16be(s)
						}
					}
				}
			}
		}
		stack = stack[:0]
	}
	return ft
}

func num(o Object) float64 {
	switch v := o.(type) {
	case int64:
		return float64(v)
	case float64:
		return v
	}
	return 0
}

func code(b []byte) uint32 {
	var v uint32
	for _, c := range b {
		v = v<<8 | uint32(c)
	}
	return v
}

func utf16be(b []byte) string {
	u := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		u = append(u, uint16(b[i])<<8|uint16(b[i+1]))
	}
	return string(utf16.Decode(u))
}

func (ft *font) decode(s String) string {
	var b strings.Builder
	n := ft.bytes
	for i := 0; i+n <= len(s); i += n {
		c := code(s[i : i+n])
		if u, ok := ft.uni[c]; ok {
			b.WriteString(u)
		} else if n == 1 {
			b.WriteByte(byte(c))
		} else {
			b.WriteRune('�')
		}
	}
	return b.String()
}

// Marked ist eine markierte Inhaltsfolge mit MCID auf einer Seite.
type Marked struct {
	Tag  string
	MCID int
	Text string
}

// PageContent ist das Ergebnis der Auswertung eines Content Streams.
type PageContent struct {
	// Marked sind die Folgen mit MCID in Reihenfolge des Streams.
	Marked []Marked
	// ArtifactText ist der Text in Artefakten.
	ArtifactText []string
	// Unmarked zählt zeichnende Operatoren außerhalb jeder Markierung,
	// Nested zählt Artefakte in Inhalt und Inhalt in Artefakten.
	Unmarked, Nested int
}

// painting sind die Operatoren, die sichtbaren Inhalt erzeugen.
var painting = map[Operator]bool{
	"Tj": true, "TJ": true, "'": true, "\"": true, "f": true, "F": true, "f*": true,
	"B": true, "B*": true, "b": true, "b*": true, "S": true, "s": true, "Do": true, "sh": true, "BI": true,
}

// Content wertet den Inhalt einer Seite aus.
func (f *File) Content(page Dict) (*PageContent, error) {
	var data []byte
	switch v := f.Resolve(page["Contents"]).(type) {
	case *Stream:
		b, err := f.Decode(v)
		if err != nil {
			return nil, err
		}
		data = b
	case Array:
		for _, o := range v {
			if s, ok := f.Resolve(o).(*Stream); ok {
				b, err := f.Decode(s)
				if err != nil {
					return nil, err
				}
				data = append(append(data, b...), '\n')
			}
		}
	}
	res := f.Dict(page["Resources"])
	fonts := f.Dict(res["Font"])
	props := f.Dict(res["Properties"])
	loaded := map[Name]*font{}
	pc := &PageContent{}
	type frame struct {
		artifact bool
		marked   int // Index in pc.Marked oder -1
		// actual: Ersatztext gilt, der gezeichnete Text entfällt.
		actual bool
	}
	var stack []frame
	var cur *font
	l := &lexer{b: data}
	var ops []Object
	inArtifact := func() bool {
		for _, fr := range stack {
			if fr.artifact {
				return true
			}
		}
		return false
	}
	target := func() int {
		for i := len(stack) - 1; i >= 0; i-- {
			if stack[i].marked >= 0 {
				return stack[i].marked
			}
		}
		return -1
	}
	replaced := func() bool {
		for _, fr := range stack {
			if fr.actual {
				return true
			}
		}
		return false
	}
	text := func(s string) {
		switch {
		case replaced():
		case inArtifact():
			pc.ArtifactText = append(pc.ArtifactText, s)
		case target() >= 0:
			pc.Marked[target()].Text += s
		}
	}
	for {
		t, err := l.object(false)
		if err != nil {
			break
		}
		op, isOp := t.(Operator)
		if !isOp {
			ops = append(ops, t)
			continue
		}
		if painting[op] && len(stack) == 0 {
			pc.Unmarked++
		}
		switch op {
		case "BI":
			// Inline-Bild bis EI überspringen.
			for {
				t, err := l.token()
				if err != nil || t == Operator("EI") {
					break
				}
			}
		case "BMC", "BDC":
			tag := ""
			if len(ops) > 0 {
				if n, ok := ops[0].(Name); ok {
					tag = string(n)
				}
			}
			fr := frame{artifact: tag == "Artifact", marked: -1}
			if len(stack) > 0 && fr.artifact != inArtifact() {
				pc.Nested++
			}
			if op == "BDC" && len(ops) > 1 {
				var d Dict
				switch v := ops[1].(type) {
				case Dict:
					d = v
				case Name:
					d = f.Dict(props[v])
				}
				if mcid, ok := f.Int(d["MCID"]); ok && !fr.artifact {
					pc.Marked = append(pc.Marked, Marked{Tag: tag, MCID: mcid})
					fr.marked = len(pc.Marked) - 1
				}
				if at, ok := f.Resolve(d["ActualText"]).(String); ok && !replaced() {
					text(textString(at))
					fr.actual = true
				}
			}
			stack = append(stack, fr)
		case "EMC":
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case "Tf":
			if len(ops) > 0 {
				if n, ok := ops[0].(Name); ok {
					if loaded[n] == nil {
						loaded[n] = f.loadFont(f.Dict(fonts[n]))
					}
					cur = loaded[n]
				}
			}
		case "Tj", "'", "\"":
			if cur != nil && len(ops) > 0 {
				if s, ok := ops[len(ops)-1].(String); ok {
					text(cur.decode(s))
				}
			}
		case "TJ":
			if cur != nil && len(ops) > 0 {
				if a, ok := ops[0].(Array); ok {
					var b strings.Builder
					for _, o := range a {
						switch v := o.(type) {
						case String:
							b.WriteString(cur.decode(v))
						case int64, float64:
							// Großer Abstand gilt als Wortzwischenraum, wenn
							// dort nicht schon ein Leerzeichen steht.
							if num(v) < -250 && b.Len() > 0 && !strings.HasSuffix(b.String(), " ") {
								b.WriteByte(' ')
							}
						}
					}
					text(b.String())
				}
			}
		}
		ops = ops[:0]
	}
	return pc, nil
}
