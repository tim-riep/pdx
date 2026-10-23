package testutil

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
)

var (
	reStartXref = regexp.MustCompile(`startxref\n(\d+)\n%%EOF\n$`)
	reXrefHead  = regexp.MustCompile(`^xref\n0 (\d+)\n`)
	reEntry     = regexp.MustCompile(`^(\d{10}) (\d{5}) ([nf])\r\n$`)
)

// CheckStructure checks the header, xref table and trailer of a PDF file.
// Every xref entry must point to "N 0 obj". The function returns the number of
// indirect objects.
func CheckStructure(pdf []byte) (int, error) {
	if !bytes.HasPrefix(pdf, []byte("%PDF-1.7\n%")) && !bytes.HasPrefix(pdf, []byte("%PDF-2.0\n%")) {
		return 0, fmt.Errorf("Kopf fehlt")
	}
	line2 := pdf[9 : bytes.IndexByte(pdf[9:], '\n')+9]
	high := 0
	for _, c := range line2[1:] {
		if c > 127 {
			high++
		}
	}
	if high < 4 {
		return 0, fmt.Errorf("Binärkommentar hat nur %d Bytes über 127", high)
	}
	m := reStartXref.FindSubmatch(pdf)
	if m == nil {
		return 0, fmt.Errorf("startxref oder %%%%EOF fehlt am Dateiende")
	}
	off, _ := strconv.Atoi(string(m[1]))
	if off >= len(pdf) {
		return 0, fmt.Errorf("startxref zeigt hinter das Dateiende")
	}
	x := pdf[off:]
	h := reXrefHead.FindSubmatch(x)
	if h == nil {
		return 0, fmt.Errorf("Xref-Tabelle nicht an Position %d", off)
	}
	n, _ := strconv.Atoi(string(h[1]))
	x = x[len(h[0]):]
	for i := 0; i < n; i++ {
		if len(x) < 20 {
			return 0, fmt.Errorf("Xref-Tabelle zu kurz")
		}
		e := reEntry.FindSubmatch(x[:20])
		if e == nil {
			return 0, fmt.Errorf("Xref-Eintrag %d hat kein gültiges Format: %q", i, x[:20])
		}
		if i > 0 {
			p, _ := strconv.Atoi(string(e[1]))
			want := fmt.Sprintf("%d 0 obj\n", i)
			if p+len(want) > len(pdf) || string(pdf[p:p+len(want)]) != want {
				return 0, fmt.Errorf("Xref-Eintrag %d zeigt nicht auf das Objekt", i)
			}
		}
		x = x[20:]
	}
	if !bytes.HasPrefix(x, []byte("trailer\n<<")) {
		return 0, fmt.Errorf("Trailer folgt nicht auf die Xref-Tabelle")
	}
	return n - 1, nil
}
