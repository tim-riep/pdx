//go:build ignore

// mkico writes an ICO file with PNG entries (Windows Vista and newer).
//
//	go run mkico.go output.ico 16.png 32.png 256.png
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image/png"
	"os"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Aufruf: go run mkico.go ausgabe.ico bild.png ...")
		os.Exit(2)
	}
	var images [][]byte
	var dims [][2]int
	for _, p := range os.Args[2:] {
		b, err := os.ReadFile(p)
		if err != nil {
			fail(err)
		}
		cfg, err := png.DecodeConfig(bytes.NewReader(b))
		if err != nil {
			fail(fmt.Errorf("%s: %w", p, err))
		}
		if cfg.Width != cfg.Height || cfg.Width > 256 {
			fail(fmt.Errorf("%s: quadratisch bis 256 Pixel verlangt, hat %dx%d", p, cfg.Width, cfg.Height))
		}
		images = append(images, b)
		dims = append(dims, [2]int{cfg.Width, cfg.Height})
	}
	var out bytes.Buffer
	// ICONDIR: reserved, type 1 (icon), count.
	binary.Write(&out, binary.LittleEndian, [3]uint16{0, 1, uint16(len(images))})
	offset := 6 + 16*len(images)
	for i, img := range images {
		w, h := dims[i][0], dims[i][1]
		// 256 is entered as 0.
		entry := struct {
			W, H, Colors, Reserved uint8
			Planes, Bits           uint16
			Size, Offset           uint32
		}{uint8(w % 256), uint8(h % 256), 0, 0, 1, 32, uint32(len(img)), uint32(offset)}
		binary.Write(&out, binary.LittleEndian, entry)
		offset += len(img)
	}
	for _, img := range images {
		out.Write(img)
	}
	if err := os.WriteFile(os.Args[1], out.Bytes(), 0o644); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "mkico:", err)
	os.Exit(1)
}
