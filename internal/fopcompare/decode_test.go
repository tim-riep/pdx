package fopcompare_test

import (
	"context"
	"testing"
	"time"

	"github.com/tim-riep/pdx"
	"github.com/tim-riep/pdx/internal/fopcompare"
)

// TestBarcodesDecode checks that the vector barcodes of the corpus are read
// by zbar and carry the expected content.
func TestBarcodesDecode(t *testing.T) {
	if !fopcompare.Enabled() {
		t.Skip("PDX_FOP=1 setzen, um Barcodes mit zbar zu dekodieren")
	}
	res, err := pdx.RenderFile("../../testdata/fo/barcodes.fo", pdx.Options{CreationDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	syms, err := fopcompare.Decode(context.Background(), res.PDF)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"CODE-128|RE-2026-0001":                             false,
		"QR-Code|https://pdx.example/rechnung/RE-2026-0001": false,
		"QR-Code|BCD\n002\n1\nSCT\nBYLADEM1001\nBeispiel GmbH\nDE02120300000000202051\nEUR1190.00\n\n\nRE-2026-0001": false,
	}
	for _, s := range syms {
		k := s.Type + "|" + s.Data
		if _, ok := want[k]; ok {
			want[k] = true
		} else {
			t.Logf("zusätzlich erkannt: %q", k)
		}
	}
	for k, found := range want {
		if !found {
			t.Errorf("nicht erkannt: %q (erkannt: %+v)", k, syms)
		}
	}
}
