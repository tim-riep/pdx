package pdf

import (
	"bytes"
	"compress/zlib"
)

// Stream ist ein Stream-Objekt. Data enthält die bereits kodierten Bytes,
// Dict den Stream-Dictionary ohne /Length, das beim Schreiben gesetzt wird.
type Stream struct {
	Dict *Dict
	Data []byte
}

// NewStream legt einen Stream mit bereits kodierten Daten an. Ein eventueller
// Filter muss im Dictionary stehen.
func NewStream(dict *Dict, data []byte) *Stream {
	if dict == nil {
		dict = NewDict()
	}
	return &Stream{Dict: dict, Data: data}
}

// NewFlateStream komprimiert raw mit Flate und setzt /Filter /FlateDecode.
// Die Kompression ist für eine Go-Version deterministisch.
func NewFlateStream(dict *Dict, raw []byte) *Stream {
	if dict == nil {
		dict = NewDict()
	}
	dict.Set("Filter", Name("FlateDecode"))
	return &Stream{Dict: dict, Data: Deflate(raw)}
}

// Deflate komprimiert Daten im zlib-Format, wie FlateDecode es erwartet.
func Deflate(raw []byte) []byte {
	var b bytes.Buffer
	w, _ := zlib.NewWriterLevel(&b, zlib.BestCompression)
	w.Write(raw)
	w.Close()
	return b.Bytes()
}

func (s *Stream) writePDF(b *bytes.Buffer) {
	s.Dict.Set("Length", Int(len(s.Data)))
	s.Dict.writePDF(b)
	b.WriteString("\nstream\n")
	b.Write(s.Data)
	b.WriteString("\nendstream")
}
