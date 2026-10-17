package fopcompare

import (
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Symbol ist ein von zbar erkannter Barcode.
type Symbol struct {
	Type string
	Data string
}

type zbarXML struct {
	Sources []struct {
		Indexes []struct {
			Symbols []struct {
				Type string `xml:"type,attr"`
				Data string `xml:"data"`
			} `xml:"symbol"`
		} `xml:"index"`
	} `xml:"source"`
}

// Decode rastert die erste Seite mit 300 dpi und dekodiert alle Barcodes mit
// zbar (QR-Code und Code 128, kein DataMatrix).
func Decode(ctx context.Context, pdf []byte) ([]Symbol, error) {
	dir, err := poppler(ctx, pdf, "sh", "-c",
		"pdftoppm -r 300 -gray -png -f 1 -l 1 in.pdf seite && zbarimg -q --xml seite-1.png > out.xml || true")
	if dir != "" {
		defer os.RemoveAll(dir)
	}
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, "out.xml"))
	if err != nil {
		return nil, err
	}
	var z zbarXML
	if err := xml.Unmarshal(data, &z); err != nil {
		return nil, fmt.Errorf("zbar-Ausgabe: %w", err)
	}
	var out []Symbol
	for _, s := range z.Sources {
		for _, ix := range s.Indexes {
			for _, sym := range ix.Symbols {
				out = append(out, Symbol{Type: sym.Type, Data: sym.Data})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Type+out[i].Data < out[j].Type+out[j].Data })
	return out, nil
}
