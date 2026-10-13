package icc

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/tim-riep/pdx/internal/testutil"
)

func TestSRGBHeader(t *testing.T) {
	p := SRGB()
	if int(binary.BigEndian.Uint32(p)) != len(p) {
		t.Fatal("Größe im Kopf stimmt nicht")
	}
	if string(p[12:16]) != "mntr" || string(p[16:20]) != "RGB " || string(p[20:24]) != "XYZ " || string(p[36:40]) != "acsp" {
		t.Fatal("Kopffelder falsch")
	}
	if binary.BigEndian.Uint32(p[8:12]) != 0x02100000 {
		t.Fatal("Version ist nicht 2.1")
	}
	n := int(binary.BigEndian.Uint32(p[128:]))
	for i := 0; i < n; i++ {
		e := p[132+12*i:]
		off, size := binary.BigEndian.Uint32(e[4:]), binary.BigEndian.Uint32(e[8:])
		if off%4 != 0 || int(off+size) > len(p) {
			t.Errorf("Tag %s liegt falsch", e[:4])
		}
	}
	if !bytes.Contains(p, []byte(SRGBDescription)) {
		t.Error("Beschreibung fehlt")
	}
	testutil.Golden(t, "testdata/srgb.icc", p)
}
