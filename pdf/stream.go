package pdf

import (
	"bytes"
	"compress/zlib"
)

// Stream is a stream object. Data contains the already encoded bytes,
// Dict the stream dictionary without /Length, which is set when writing.
type Stream struct {
	Dict *Dict
	Data []byte
}

// NewStream creates a stream with already encoded data. Any
// filter must be in the dictionary.
func NewStream(dict *Dict, data []byte) *Stream {
	if dict == nil {
		dict = NewDict()
	}
	return &Stream{Dict: dict, Data: data}
}

// NewFlateStream compresses raw with Flate and sets /Filter /FlateDecode.
// The compression is deterministic for a given Go version.
func NewFlateStream(dict *Dict, raw []byte) *Stream {
	if dict == nil {
		dict = NewDict()
	}
	dict.Set("Filter", Name("FlateDecode"))
	return &Stream{Dict: dict, Data: Deflate(raw)}
}

// Deflate compresses data in zlib format, as FlateDecode expects it.
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
