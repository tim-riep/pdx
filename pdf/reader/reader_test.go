package reader

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestLexer(t *testing.T) {
	l := &lexer{b: []byte(`<< /Na#20me (a\(b\)\101\
c) /H <48 65 6C6> /A [1 -2.5 true null 3 0 R] /D << /X /Y >> >>`)}
	o, err := l.object(true)
	if err != nil {
		t.Fatal(err)
	}
	want := Dict{
		"Na me": String("a(b)Ac"),
		"H":     String("Hel`"),
		"A":     Array{int64(1), -2.5, true, nil, Ref{3, 0}},
		"D":     Dict{"X": Name("Y")},
	}
	if !reflect.DeepEqual(o, want) {
		t.Errorf("%#v", o)
	}
}

func deflate(b []byte) []byte {
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	w.Write(b)
	w.Close()
	return buf.Bytes()
}

// TestXrefStream liest eine Datei mit Xref-Stream (PNG-Prädiktor Up) und
// Objektstream, wie sie andere Werkzeuge schreiben.
func TestXrefStream(t *testing.T) {
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n")
	offs := map[int]int{}
	obj := func(n int, body string, stream []byte) {
		offs[n] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s", n, body)
		if stream != nil {
			b.WriteString("\nstream\n")
			b.Write(stream)
			b.WriteString("\nendstream")
		}
		b.WriteString("\nendobj\n")
	}
	// Objekt 2 (Katalog) und 3 (Seiten) liegen im Objektstream 4.
	objs := "<< /Type /Catalog /Pages 3 0 R >> << /Type /Pages /Kids [] /Count 0 >>"
	head := fmt.Sprintf("2 0 3 %d ", len("<< /Type /Catalog /Pages 3 0 R >> "))
	body := deflate([]byte(head + objs))
	obj(4, fmt.Sprintf("<< /Type /ObjStm /N 2 /First %d /Filter /FlateDecode /Length %d >>", len(head), len(body)), body)
	// Xref-Stream: W [1 2 1], Zeilen mit Prädiktor Up.
	rows := [][4]byte{
		{0, 0, 0, 0},
		{2, 0, 4, 0}, // 1 frei (Platzhalter, Typ 2 auf Stream 4 Index 0)
		{2, 0, 4, 0},
		{2, 0, 4, 1},
		{1, byte(offs[4] >> 8), byte(offs[4]), 0},
		{1, 0, 0, 0},
	}
	rows[1] = [4]byte{0, 0, 0, 0}
	xoff := b.Len()
	rows[5] = [4]byte{1, byte(xoff >> 8), byte(xoff), 0}
	var raw []byte
	prev := [4]byte{}
	for _, r := range rows {
		raw = append(raw, 2)
		for i := range r {
			raw = append(raw, r[i]-prev[i])
		}
		prev = r
	}
	xs := deflate(raw)
	obj(5, fmt.Sprintf("<< /Type /XRef /Size 6 /W [1 2 1] /Root 2 0 R /Filter /FlateDecode /DecodeParms << /Predictor 12 /Columns 4 >> /Length %d >>", len(xs)), xs)
	fmt.Fprintf(&b, "startxref\n%d\n%%%%EOF\n", xoff)

	f, err := Open(b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	root := f.Root()
	if root["Type"] != Name("Catalog") {
		t.Fatalf("Katalog nicht gelesen: %v", root)
	}
	if p := f.Dict(root["Pages"]); p["Count"] != int64(0) {
		t.Errorf("Seiten nicht gelesen: %v", p)
	}
}

// TestStructureProblems erkennt unmarkierten Inhalt, fehlende MCIDs und
// Inhalt ohne Element.
func TestStructureProblems(t *testing.T) {
	content := "/P <</MCID 0>> BDC 0 0 1 1 re f EMC 0 0 1 1 re f /P <</MCID 1>> BDC EMC"
	pdf := minimal(content, "<< /Type /StructElem /S /P /P 5 0 R /Pg 3 0 R /K [0 7] >>")
	f, err := Open(pdf)
	if err != nil {
		t.Fatal(err)
	}
	st, err := f.ReadStructure()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"Seite 1: 1 zeichnende Operatoren ohne Markierung",
		"P verweist auf Seite 1 MCID 7, die es nicht gibt",
		"Seite 1 MCID 1 gehört zu keinem Element",
	}
	if !reflect.DeepEqual(st.Problems, want) {
		t.Errorf("Befunde:\n%q\nerwartet:\n%q", st.Problems, want)
	}
}

// minimal baut eine einseitige Datei mit Xref-Tabelle und einem
// Strukturelement unter Document.
func minimal(content, elem string) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n")
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R /StructTreeRoot 4 0 R /MarkInfo << /Marked true >> >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 10 10] /Contents 7 0 R /StructParents 0 >>",
		"<< /Type /StructTreeRoot /K 5 0 R /ParentTree << /Nums [0 [6 0 R 6 0 R]] >> >>",
		"<< /Type /StructElem /S /Document /K [6 0 R] >>",
		elem,
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
	}
	offs := make([]int, len(objs))
	for i, o := range objs {
		offs[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	x := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f\r\n", len(objs)+1)
	for _, o := range offs {
		fmt.Fprintf(&b, "%010d 00000 n\r\n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, x)
	return b.Bytes()
}

// Zyklische Verweise führen zu einem Fehler statt zu endloser Rekursion.
func TestCyclicLength(t *testing.T) {
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n")
	o1 := b.Len()
	b.WriteString("1 0 obj\n<< /Length 1 0 R >>\nstream\nabc\nendstream\nendobj\n")
	x := b.Len()
	fmt.Fprintf(&b, "xref\n0 2\n0000000000 65535 f\r\n%010d 00000 n\r\ntrailer\n<< /Size 2 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", o1, x)
	f, err := Open(b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { f.Get(Ref{1, 0}); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("zyklische Länge hängt")
	}
}
