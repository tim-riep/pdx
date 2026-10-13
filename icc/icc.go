// Package icc erzeugt das sRGB-Farbprofil für das Output Intent.
//
// Das Profil wird aus den Parametern der Norm IEC 61966-2-1 berechnet
// (Primärfarben nach Bradford auf D50 adaptiert, Tonwertkurve als Tabelle mit
// 1024 Stützstellen) und ist ein ICC-Profil der Version 2.1 der Klasse
// Monitor. Es ist eigenes Werk dieses Projekts, eine Fremdlizenz ist nicht
// nötig. Die Erzeugung ist deterministisch.
package icc

import (
	"bytes"
	"encoding/binary"
	"math"
	"sync"
)

// SRGBDescription ist die Profilbeschreibung und zugleich die
// OutputConditionIdentifier im Output Intent.
const SRGBDescription = "sRGB IEC61966-2.1"

var (
	once sync.Once
	srgb []byte
)

// SRGB liefert das sRGB-Profil. Die Bytes dürfen nicht verändert werden.
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

	// Tag-Daten anordnen. Gleiche Daten (die drei Kurven) werden geteilt.
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
	// Kopf (128 Bytes) nach ICC.1:2001-04.
	be(uint32(size))
	b.WriteString("\x00\x00\x00\x00") // bevorzugtes CMM
	be(uint32(0x02100000))            // Version 2.1
	b.WriteString("mntr")
	b.WriteString("RGB ")
	b.WriteString("XYZ ")
	be([6]uint16{2026, 1, 1, 0, 0, 0}) // Erstellungsdatum, fest für Reproduzierbarkeit
	b.WriteString("acsp")
	b.WriteString("\x00\x00\x00\x00") // Plattform
	be(uint32(0))                     // Flags
	b.WriteString("\x00\x00\x00\x00") // Gerätehersteller
	be(uint32(0))                     // Gerätemodell
	be(uint64(0))                     // Geräteattribute
	be(uint32(0))                     // Rendering Intent perzeptiv
	b.Write(xyzNum(0.9642, 1.0, 0.8249))
	b.WriteString("\x00\x00\x00\x00") // Ersteller
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

// textDescription baut einen textDescriptionType mit ASCII-Teil und leeren
// Unicode- und ScriptCode-Teilen.
func textDescription(s string) []byte {
	var b bytes.Buffer
	b.WriteString("desc\x00\x00\x00\x00")
	binary.Write(&b, binary.BigEndian, uint32(len(s)+1))
	b.WriteString(s)
	b.WriteByte(0)
	binary.Write(&b, binary.BigEndian, uint32(0)) // Unicode-Sprachcode
	binary.Write(&b, binary.BigEndian, uint32(0)) // Unicode-Länge
	binary.Write(&b, binary.BigEndian, uint16(0)) // ScriptCode-Code
	b.WriteByte(0)                                // ScriptCode-Länge
	b.Write(make([]byte, 67))                     // ScriptCode-Daten
	return b.Bytes()
}

// curve liefert die sRGB-Tonwertkurve als curveType mit 1024 Stützstellen.
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
