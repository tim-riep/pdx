// Package icc generates the sRGB color profile for the output intent.
//
// The profile is computed from the parameters of the standard IEC 61966-2-1
// (primaries adapted to D50 with Bradford, tone curve as a table with
// 1024 entries) and is an ICC profile of version 2.1 of the class
// monitor. It is original work of this project, no third-party license is
// needed. The generation is deterministic.
package icc

import (
	"bytes"
	"encoding/binary"
	"math"
	"sync"
)

// SRGBDescription is the profile description and also the
// OutputConditionIdentifier in the output intent.
const SRGBDescription = "sRGB IEC61966-2.1"

var (
	once sync.Once
	srgb []byte
)

// SRGB returns the sRGB profile. The bytes must not be modified.
func SRGB() []byte {
	once.Do(func() { srgb = buildSRGB() })
	return srgb
}

type tag struct {
	sig  string
	data []byte
}

func buildSRGB() []byte {
	trc := curve()
	tags := []tag{
		{"desc", textDescription(SRGBDescription)},
		{"cprt", text("Erzeugt von pdx aus IEC 61966-2-1, frei verwendbar")},
		{"wtpt", xyz(0.9642, 1.0, 0.8249)},
		{"rXYZ", xyz(0.4360747, 0.2225045, 0.0139322)},
		{"gXYZ", xyz(0.3850649, 0.7168786, 0.0971045)},
		{"bXYZ", xyz(0.1430804, 0.0606169, 0.7141733)},
		{"rTRC", trc},
		{"gTRC", trc},
		{"bTRC", trc},
	}

	// Arrange tag data. Identical data (the three curves) is shared.
	tableLen := 4 + 12*len(tags)
	offset := 128 + tableLen
	var data bytes.Buffer
	type entry struct{ off, size int }
	placed := map[string]entry{}
	entries := make([]entry, len(tags))
	for i, t := range tags {
		if e, ok := placed[string(t.data)]; ok {
			entries[i] = e
			continue
		}
		for (offset+data.Len())%4 != 0 {
			data.WriteByte(0)
		}
		e := entry{off: offset + data.Len(), size: len(t.data)}
		data.Write(t.data)
		placed[string(t.data)] = e
		entries[i] = e
	}
	for (offset+data.Len())%4 != 0 {
		data.WriteByte(0)
	}
	size := offset + data.Len()

	var b bytes.Buffer
	be := func(v any) { binary.Write(&b, binary.BigEndian, v) }
	// Header (128 bytes) according to ICC.1:2001-04.
	be(uint32(size))
	b.WriteString("\x00\x00\x00\x00") // preferred CMM
	be(uint32(0x02100000))            // Version 2.1
	b.WriteString("mntr")
	b.WriteString("RGB ")
	b.WriteString("XYZ ")
	be([6]uint16{2026, 1, 1, 0, 0, 0}) // creation date, fixed for reproducibility
	b.WriteString("acsp")
	b.WriteString("\x00\x00\x00\x00") // platform
	be(uint32(0))                     // Flags
	b.WriteString("\x00\x00\x00\x00") // device manufacturer
	be(uint32(0))                     // device model
	be(uint64(0))                     // device attributes
	be(uint32(0))                     // rendering intent perceptual
	b.Write(xyzNum(0.9642, 1.0, 0.8249))
	b.WriteString("\x00\x00\x00\x00") // creator
	b.Write(make([]byte, 128-b.Len()))

	be(uint32(len(tags)))
	for i, t := range tags {
		b.WriteString(t.sig)
		be(uint32(entries[i].off))
		be(uint32(entries[i].size))
	}
	b.Write(data.Bytes())
	return b.Bytes()
}

func s15(v float64) []byte {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], uint32(int32(math.Round(v*65536))))
	return b[:]
}

func xyzNum(x, y, z float64) []byte {
	return append(append(s15(x), s15(y)...), s15(z)...)
}

func xyz(x, y, z float64) []byte {
	return append([]byte("XYZ \x00\x00\x00\x00"), xyzNum(x, y, z)...)
}

func text(s string) []byte {
	return append(append([]byte("text\x00\x00\x00\x00"), s...), 0)
}

// textDescription builds a textDescriptionType with an ASCII part and empty
// Unicode and ScriptCode parts.
func textDescription(s string) []byte {
	var b bytes.Buffer
	b.WriteString("desc\x00\x00\x00\x00")
	binary.Write(&b, binary.BigEndian, uint32(len(s)+1))
	b.WriteString(s)
	b.WriteByte(0)
	binary.Write(&b, binary.BigEndian, uint32(0)) // Unicode language code
	binary.Write(&b, binary.BigEndian, uint32(0)) // Unicode length
	binary.Write(&b, binary.BigEndian, uint16(0)) // ScriptCode code
	b.WriteByte(0)                                // ScriptCode length
	b.Write(make([]byte, 67))                     // ScriptCode data
	return b.Bytes()
}

// curve returns the sRGB tone curve as a curveType with 1024 entries.
func curve() []byte {
	const n = 1024
	var b bytes.Buffer
	b.WriteString("curv\x00\x00\x00\x00")
	binary.Write(&b, binary.BigEndian, uint32(n))
	for i := 0; i < n; i++ {
		x := float64(i) / (n - 1)
		var y float64
		if x <= 0.04045 {
			y = x / 12.92
		} else {
			y = math.Pow((x+0.055)/1.055, 2.4)
		}
		binary.Write(&b, binary.BigEndian, uint16(math.Round(y*65535)))
	}
	return b.Bytes()
}
