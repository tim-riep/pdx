// Package image lädt Rasterbilder für die PDF-Ausgabe. JPEG-Daten werden
// unverändert durchgereicht (DCTDecode), PNG wird dekodiert und als
// Flate-Stream mit getrenntem Alphakanal bereitgestellt.
package image

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"image/color"
	"image/png"

	"github.com/tim-riep/pdx/areatree"
)

// Format ist das Quellformat eines Bildes.
type Format uint8

const (
	// JPEG wird unverändert als DCTDecode eingebettet.
	JPEG Format = iota + 1
	// PNG wird dekodiert und mit Flate komprimiert.
	PNG
)

// ColorSpace ist der Farbraum der Pixeldaten.
type ColorSpace uint8

const (
	// Gray ist DeviceGray.
	Gray ColorSpace = iota + 1
	// RGB ist DeviceRGB.
	RGB
	// CMYK ist DeviceCMYK.
	CMYK
)

// Image ist ein geladenes Bild.
type Image struct {
	key    string
	Format Format
	Width  int
	Height int
	Space  ColorSpace
	// BitsPerComponent ist bei PNG immer 8.
	BitsPerComponent int
	// Data enthält bei JPEG die Originaldatei, bei PNG die unkomprimierten
	// Pixel Zeile für Zeile.
	Data []byte
	// Alpha enthält bei PNG mit Transparenz einen Kanal je Pixel, sonst nil.
	Alpha []byte
	// InvertCMYK ist bei Adobe-CMYK-JPEGs gesetzt (invertierte Werte).
	InvertCMYK bool
	// DPIX und DPIY sind die Auflösung aus der Datei, 0 wenn unbekannt.
	DPIX, DPIY float64
}

// Key identifiziert das Bild über einen Hash der Quelldaten.
func (im *Image) Key() string { return im.key }

// PixelSize liefert Breite und Höhe in Pixel und erfüllt areatree.ImageSource.
func (im *Image) PixelSize() (int, int) { return im.Width, im.Height }

// IntrinsicSize liefert die natürliche Größe. Ohne Auflösungsangabe gilt
// wie bei Apache FOP 72 dpi, also ein Pixel je Punkt.
func (im *Image) IntrinsicSize() (w, h areatree.Length) {
	dx, dy := im.DPIX, im.DPIY
	if dx <= 0 || dy <= 0 {
		dx, dy = 72, 72
	}
	return areatree.Points(float64(im.Width) * 72 / dx), areatree.Points(float64(im.Height) * 72 / dy)
}

// Load erkennt das Format an den ersten Bytes und lädt das Bild.
func Load(data []byte) (*Image, error) {
	sum := sha256.Sum256(data)
	key := "sha256:" + hex.EncodeToString(sum[:8])
	switch {
	case bytes.HasPrefix(data, []byte{0xff, 0xd8, 0xff}):
		im, err := loadJPEG(data)
		if err != nil {
			return nil, err
		}
		im.key = key
		return im, nil
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		im, err := loadPNG(data)
		if err != nil {
			return nil, err
		}
		im.key = key
		return im, nil
	}
	return nil, errors.New("Bildformat nicht unterstützt, erwartet JPEG oder PNG")
}

// loadJPEG liest Größe, Komponenten und Auflösung aus den Markern.
func loadJPEG(data []byte) (*Image, error) {
	im := &Image{Format: JPEG, Data: data}
	p := 2
	for p+4 <= len(data) {
		if data[p] != 0xff {
			return nil, fmt.Errorf("JPEG: Marker erwartet an Position %d", p)
		}
		marker := data[p+1]
		if marker == 0xff { // Füllbyte
			p++
			continue
		}
		if marker == 0xd8 || (marker >= 0xd0 && marker <= 0xd7) || marker == 0x01 {
			p += 2
			continue
		}
		seg := int(binary.BigEndian.Uint16(data[p+2:]))
		if seg < 2 || p+2+seg > len(data) {
			return nil, errors.New("JPEG: Segment abgeschnitten")
		}
		body := data[p+4 : p+2+seg]
		switch {
		case marker == 0xe0 && len(body) >= 12 && string(body[:5]) == "JFIF\x00":
			unit := body[7]
			x, y := float64(binary.BigEndian.Uint16(body[8:])), float64(binary.BigEndian.Uint16(body[10:]))
			switch unit {
			case 1:
				im.DPIX, im.DPIY = x, y
			case 2:
				im.DPIX, im.DPIY = x*2.54, y*2.54
			}
		case marker == 0xee && len(body) >= 5 && string(body[:5]) == "Adobe":
			im.InvertCMYK = true
		case marker >= 0xc0 && marker <= 0xcf && marker != 0xc4 && marker != 0xc8 && marker != 0xcc:
			if marker == 0xc3 || marker == 0xc7 || marker == 0xcb || marker == 0xcf {
				return nil, errors.New("JPEG: verlustfreies JPEG wird von PDF nicht unterstützt")
			}
			if len(body) < 6 {
				return nil, errors.New("JPEG: SOF-Segment zu kurz")
			}
			im.BitsPerComponent = int(body[0])
			im.Height = int(binary.BigEndian.Uint16(body[1:]))
			im.Width = int(binary.BigEndian.Uint16(body[3:]))
			switch body[5] {
			case 1:
				im.Space = Gray
			case 3:
				im.Space = RGB
			case 4:
				im.Space = CMYK
			default:
				return nil, fmt.Errorf("JPEG: %d Farbkomponenten werden nicht unterstützt", body[5])
			}
			if im.BitsPerComponent != 8 {
				return nil, fmt.Errorf("JPEG: %d Bit je Komponente werden nicht unterstützt", im.BitsPerComponent)
			}
			if im.Width == 0 || im.Height == 0 {
				return nil, errors.New("JPEG: Breite oder Höhe ist 0")
			}
			if im.Space != CMYK {
				im.InvertCMYK = false
			}
			return im, nil
		case marker == 0xda || marker == 0xd9:
			return nil, errors.New("JPEG: kein SOF-Segment vor den Bilddaten")
		}
		p += 2 + seg
	}
	return nil, errors.New("JPEG: kein SOF-Segment gefunden")
}

// loadPNG dekodiert ein PNG in 8 Bit Grau oder RGB plus optionalem Alpha.
func loadPNG(data []byte) (*Image, error) {
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("PNG: %w", err)
	}
	if cfg.Width*cfg.Height > 100_000_000 {
		return nil, fmt.Errorf("PNG: %dx%d Pixel sind zu groß", cfg.Width, cfg.Height)
	}
	src, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("PNG: %w", err)
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	im := &Image{Format: PNG, Width: w, Height: h, BitsPerComponent: 8}
	im.DPIX, im.DPIY = pngDPI(data)

	gray := isGrayModel(src.ColorModel())
	if gray {
		im.Space = Gray
		im.Data = make([]byte, 0, w*h)
	} else {
		im.Space = RGB
		im.Data = make([]byte, 0, 3*w*h)
	}
	alpha := make([]byte, 0, w*h)
	opaque := true
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := color.NRGBA64Model.Convert(src.At(x, y)).(color.NRGBA64)
			if gray {
				im.Data = append(im.Data, byte(c.R>>8))
			} else {
				im.Data = append(im.Data, byte(c.R>>8), byte(c.G>>8), byte(c.B>>8))
			}
			a := byte(c.A >> 8)
			if a != 0xff {
				opaque = false
			}
			alpha = append(alpha, a)
		}
	}
	if !opaque {
		im.Alpha = alpha
	}
	return im, nil
}

func isGrayModel(m color.Model) bool {
	return m == color.GrayModel || m == color.Gray16Model
}

// pngDPI liest den pHYs-Chunk, falls vorhanden und in Metern angegeben.
func pngDPI(data []byte) (float64, float64) {
	p := 8
	for p+12 <= len(data) {
		n := int(binary.BigEndian.Uint32(data[p:]))
		typ := string(data[p+4 : p+8])
		if n < 0 || p+12+n > len(data) {
			break
		}
		if typ == "pHYs" && n == 9 && data[p+16] == 1 {
			x := float64(binary.BigEndian.Uint32(data[p+8:]))
			y := float64(binary.BigEndian.Uint32(data[p+12:]))
			return x * 0.0254, y * 0.0254
		}
		if typ == "IDAT" {
			break
		}
		p += 12 + n
	}
	return 0, 0
}
