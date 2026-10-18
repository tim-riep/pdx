package xmp

import (
	"bytes"
	"encoding/xml"
	"testing"
	"time"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/internal/testutil"
)

func sample() *Packet {
	d := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return &Packet{
		PDFAPart: 3, PDFAConformance: "B",
		Title: "Rechnung <2026> & Co", Author: "Tim", Subject: "Test", Keywords: "a, b",
		Creator: "pdx-Test", Producer: "pdx", Created: d, Modified: d,
		Extension: true,
		Schemas: []areatree.XMPSchema{{NamespaceURI: "https://github.com/tim-riep/pdx/ns/test#", Prefix: "pdxt", Description: "Testschema",
			Properties: []areatree.XMPProperty{{Name: "Version", Value: "1.0", Description: "Version"}}}},
	}
}

func TestGolden(t *testing.T) {
	b, err := sample().Bytes()
	if err != nil {
		t.Fatal(err)
	}
	// Wohlgeformt?
	dec := xml.NewDecoder(bytes.NewReader(b))
	for {
		if _, err := dec.Token(); err != nil {
			if err.Error() != "EOF" {
				t.Fatalf("XMP nicht wohlgeformt: %v", err)
			}
			break
		}
	}
	testutil.Golden(t, "testdata/sample.xmp", b)
}

func TestRejectsReservedPrefix(t *testing.T) {
	p := sample()
	p.Schemas[0].Prefix = "dc"
	if _, err := p.Bytes(); err == nil {
		t.Fatal("reserviertes Präfix nicht erkannt")
	}
}
