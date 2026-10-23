package incr

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"testing"

	"github.com/tim-riep/pdx/pdf"
	"github.com/tim-riep/pdx/pdf/reader"
)

// xrefStreamPDF builds a small file with an xref stream.
func xrefStreamPDF() []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n")
	o1 := b.Len()
	b.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")
	o2 := b.Len()
	b.WriteString("2 0 obj\n<< /Type /Pages /Kids [] /Count 0 >>\nendobj\n")
	x := b.Len()
	var rows []byte
	for _, r := range [][3]int{{0, 0, 65535}, {1, o1, 0}, {1, o2, 0}, {1, x, 0}} {
		rows = append(rows, byte(r[0]), byte(r[1]>>8), byte(r[1]), byte(r[2]>>8), byte(r[2]))
	}
	var z bytes.Buffer
	w := zlib.NewWriter(&z)
	w.Write(rows)
	w.Close()
	fmt.Fprintf(&b, "3 0 obj\n<< /Type /XRef /Size 4 /W [1 2 2] /Root 1 0 R /Filter /FlateDecode /Length %d >>\nstream\n", z.Len())
	b.Write(z.Bytes())
	b.WriteString("\nendstream\nendobj\n")
	fmt.Fprintf(&b, "startxref\n%d\n%%%%EOF\n", x)
	return b.Bytes()
}

// A file with an xref stream gets an update with an xref stream.
func TestUpdateXRefStream(t *testing.T) {
	u, err := Open(xrefStreamPDF())
	if err != nil {
		t.Fatal(err)
	}
	cat, err := u.Dict(u.Root())
	if err != nil {
		t.Fatal(err)
	}
	extra := u.Add(pdf.D("Note", pdf.String("neu")))
	cat.Set("Extra", extra)
	u.Set(u.Root(), cat)
	res, err := u.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(res.Data[len(xrefStreamPDF()):], []byte("\nxref\n")) {
		t.Error("Update schreibt eine Xref-Tabelle statt eines Xref-Streams")
	}
	f, err := reader.Open(res.Data)
	if err != nil {
		t.Fatal(err)
	}
	if f.Dict(f.Root()["Extra"])["Note"] == nil || !f.XRefStream() {
		t.Errorf("neues Objekt nicht lesbar: %v", f.Root())
	}
	if f.Dict(f.Root()["Pages"])["Type"] != reader.Name("Pages") {
		t.Error("altes Objekt über Prev nicht lesbar")
	}
	// An empty update is an error.
	u2, _ := Open(xrefStreamPDF())
	if _, err := u2.Bytes(); err == nil {
		t.Error("leeres Update angenommen")
	}
}
