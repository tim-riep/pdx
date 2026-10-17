package image

import (
	"bytes"
	goimage "image"
	"image/color"
	"image/gif"
	"os"
	"testing"
)

func load(t *testing.T, name string) *Image {
	t.Helper()
	data, err := os.ReadFile("../testdata/images/" + name)
	if err != nil {
		t.Fatal(err)
	}
	im, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	return im
}

func TestJPEG(t *testing.T) {
	im := load(t, "foto.jpg")
	if im.Format != JPEG || im.Width != 320 || im.Height != 200 || im.Space != RGB || im.BitsPerComponent != 8 {
		t.Errorf("JPEG falsch gelesen: %+v", im)
	}
	if w, h := im.IntrinsicSize(); w != 320000 || h != 200000 {
		t.Errorf("Größe %v x %v", w, h)
	}
}

func TestPNGAlpha(t *testing.T) {
	im := load(t, "logo-alpha.png")
	if im.Format != PNG || im.Space != RGB || len(im.Data) != 128*128*3 || len(im.Alpha) != 128*128 {
		t.Errorf("PNG falsch gelesen: Format %d Space %d Daten %d Alpha %d", im.Format, im.Space, len(im.Data), len(im.Alpha))
	}
}

func TestPNGGray(t *testing.T) {
	im := load(t, "verlauf-grau.png")
	if im.Space != Gray || im.Alpha != nil || len(im.Data) != 200*40 {
		t.Errorf("Graustufen-PNG falsch gelesen: %+v", im.Space)
	}
}

func TestRejects(t *testing.T) {
	for _, data := range [][]byte{nil, []byte("GIF89a"), {0xff, 0xd8, 0xff, 0xd9}, []byte("\x89PNG\r\n\x1a\nkaputt")} {
		if _, err := Load(data); err == nil {
			t.Errorf("Load(%q) ohne Fehler", data)
		}
	}
}

func TestGIF(t *testing.T) {
	pal := color.Palette{color.Transparent, color.RGBA{255, 0, 0, 255}}
	src := goimage.NewPaletted(goimage.Rect(0, 0, 2, 1), pal)
	src.SetColorIndex(1, 0, 1)
	var b bytes.Buffer
	if err := gif.Encode(&b, src, nil); err != nil {
		t.Fatal(err)
	}
	im, err := Load(b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if im.Format != PNG || im.Space != RGB || im.Width != 2 || len(im.Alpha) != 2 || im.Alpha[0] != 0 || im.Alpha[1] != 255 {
		t.Fatalf("%+v", im)
	}
}
