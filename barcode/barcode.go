// Package barcode generates barcodes as module matrices for vector
// output: QR code, DataMatrix (ECC 200), Code 128 and GiroCode (EPC QR).
// Encoding is done by github.com/boombuler/barcode (MIT), this package
// returns the modules including the quiet zone and validates the input.
package barcode

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"unicode/utf8"

	bc "github.com/boombuler/barcode"
	"github.com/boombuler/barcode/code128"
	"github.com/boombuler/barcode/datamatrix"
	"github.com/boombuler/barcode/qr"
)

// Matrix is a barcode as a grid of dark modules. For Code 128 Rows is 1,
// each column is then a bar over the full height.
type Matrix struct {
	Cols, Rows int
	// Quiet is the quiet zone in modules on each side (for Code 128 only
	// left and right).
	Quiet  int
	dark   []bool
	Linear bool
}

// Dark reports whether the module in column x and row y is dark.
func (m *Matrix) Dark(x, y int) bool { return m.dark[y*m.Cols+x] }

func fromImage(b bc.Barcode, quiet int, linear bool) *Matrix {
	r := b.Bounds()
	m := &Matrix{Cols: r.Dx(), Rows: r.Dy(), Quiet: quiet, Linear: linear}
	if linear {
		m.Rows = 1
	}
	m.dark = make([]bool, m.Cols*m.Rows)
	for y := 0; y < m.Rows; y++ {
		for x := 0; x < m.Cols; x++ {
			c, _, _, _ := b.At(r.Min.X+x, r.Min.Y+y).RGBA()
			m.dark[y*m.Cols+x] = c < 0x8000
		}
	}
	return m
}

// Spec describes a barcode.
type Spec struct {
	// Type is qr, datamatrix, code128 or girocode.
	Type string
	// Value is the content for qr, datamatrix and code128.
	Value string
	// ECC is the error correction L, M, Q or H for qr (default M).
	ECC string
	// GiroCode describes a SEPA credit transfer for type="girocode".
	Giro Giro
}

// Giro holds the fields of a GiroCode according to EPC069-12, version 002.
type Giro struct {
	Name      string // beneficiary, at most 70 characters
	IBAN      string
	BIC       string // optional from version 002
	Amount    string // Betrag in Euro, etwa "1190.00", optional
	Purpose   string // purpose code, 4 characters, optional
	Reference string // structured reference (RF), optional
	Text      string // remittance information, at most 140 characters
}

// Generate generates the module matrix.
func Generate(s Spec) (*Matrix, error) {
	switch s.Type {
	case "qr":
		if s.Value == "" {
			return nil, fmt.Errorf("QR-Code ohne Inhalt")
		}
		lvl, err := eccLevel(s.ECC)
		if err != nil {
			return nil, err
		}
		b, err := qr.Encode(s.Value, lvl, qr.Unicode)
		if err != nil {
			return nil, fmt.Errorf("QR-Code: %v", err)
		}
		return fromImage(b, 4, false), nil
	case "girocode":
		payload, err := s.Giro.Payload()
		if err != nil {
			return nil, err
		}
		b, err := qr.Encode(payload, qr.M, qr.Unicode)
		if err != nil {
			return nil, fmt.Errorf("GiroCode: %v", err)
		}
		return fromImage(b, 4, false), nil
	case "datamatrix":
		if s.Value == "" {
			return nil, fmt.Errorf("DataMatrix ohne Inhalt")
		}
		b, err := datamatrix.Encode(s.Value)
		if err != nil {
			return nil, fmt.Errorf("DataMatrix: %v", err)
		}
		return fromImage(b, 1, false), nil
	case "code128":
		if s.Value == "" {
			return nil, fmt.Errorf("Code 128 ohne Inhalt")
		}
		b, err := code128.Encode(s.Value)
		if err != nil {
			return nil, fmt.Errorf("Code 128: %v", err)
		}
		return fromImage(b, 10, true), nil
	}
	return nil, fmt.Errorf("Barcodetyp %q unbekannt, erlaubt sind qr, datamatrix, code128 und girocode", s.Type)
}

func eccLevel(s string) (qr.ErrorCorrectionLevel, error) {
	switch strings.ToUpper(s) {
	case "L":
		return qr.L, nil
	case "", "M":
		return qr.M, nil
	case "Q":
		return qr.Q, nil
	case "H":
		return qr.H, nil
	}
	return qr.M, fmt.Errorf("Fehlerkorrektur %q unbekannt, erlaubt sind L, M, Q und H", s)
}

// Payload builds the payload of a GiroCode according to EPC069-12 version 002
// (character set UTF-8, identifier SCT) and validates IBAN and amount.
func (g Giro) Payload() (string, error) {
	iban := strings.ToUpper(strings.ReplaceAll(g.IBAN, " ", ""))
	if !validIBAN(iban) {
		return "", fmt.Errorf("GiroCode: IBAN %q ist ungültig", g.IBAN)
	}
	if g.Name == "" || utf8.RuneCountInString(g.Name) > 70 {
		return "", fmt.Errorf("GiroCode: Empfängername fehlt oder ist länger als 70 Zeichen")
	}
	amount := ""
	if g.Amount != "" {
		v, err := strconv.ParseFloat(g.Amount, 64)
		if err != nil || v < 0.01 || v > 999999999.99 || strings.Contains(g.Amount, ",") {
			return "", fmt.Errorf("GiroCode: Betrag %q ist ungültig, erwartet etwa 1190.00", g.Amount)
		}
		amount = "EUR" + strconv.FormatFloat(v, 'f', 2, 64)
	}
	if g.Reference != "" && g.Text != "" {
		return "", fmt.Errorf("GiroCode: Referenz und Verwendungszweck schließen sich aus")
	}
	if utf8.RuneCountInString(g.Text) > 140 {
		return "", fmt.Errorf("GiroCode: Verwendungszweck ist länger als 140 Zeichen")
	}
	lines := []string{"BCD", "002", "1", "SCT", strings.ToUpper(strings.ReplaceAll(g.BIC, " ", "")),
		g.Name, iban, amount, g.Purpose, g.Reference, g.Text}
	p := strings.TrimRight(strings.Join(lines, "\n"), "\n")
	if len(p) > 331 {
		return "", fmt.Errorf("GiroCode: Nutzlast mit %d Bytes ist länger als 331", len(p))
	}
	return p, nil
}

// validIBAN checks format and check digits (ISO 13616, modulo 97).
func validIBAN(iban string) bool {
	if len(iban) < 15 || len(iban) > 34 {
		return false
	}
	var digits strings.Builder
	for _, r := range iban[4:] + iban[:4] {
		switch {
		case r >= '0' && r <= '9':
			digits.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			digits.WriteString(strconv.Itoa(int(r-'A') + 10))
		default:
			return false
		}
	}
	n, ok := new(big.Int).SetString(digits.String(), 10)
	return ok && new(big.Int).Mod(n, big.NewInt(97)).Int64() == 1
}

// Rects merges the dark modules row by row into rectangles.
// Coordinates in modules without quiet zone, origin top left.
func (m *Matrix) Rects() [][4]int {
	var out [][4]int
	for y := 0; y < m.Rows; y++ {
		x := 0
		for x < m.Cols {
			if !m.Dark(x, y) {
				x++
				continue
			}
			start := x
			for x < m.Cols && m.Dark(x, y) {
				x++
			}
			out = append(out, [4]int{start, y, x - start, 1})
		}
	}
	return out
}
