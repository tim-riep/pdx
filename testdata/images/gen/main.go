// Erzeugt die Testbilder für testdata/images.
package main

import (
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

func main() {
	dir := os.Args[1]
	// Foto: Farbverlauf mit Kreis.
	ph := image.NewRGBA(image.Rect(0, 0, 320, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 320; x++ {
			r := uint8(40 + x*180/320)
			g := uint8(90 + y*120/200)
			b := uint8(200 - x*100/320)
			if math.Hypot(float64(x-220), float64(y-90)) < 50 {
				r, g, b = 250, 210, 60
			}
			ph.Set(x, y, color.RGBA{r, g, b, 255})
		}
	}
	f, _ := os.Create(filepath.Join(dir, "foto.jpg"))
	jpeg.Encode(f, ph, &jpeg.Options{Quality: 85})
	f.Close()
	// Logo mit Alpha: weicher Kreis.
	lg := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			d := math.Hypot(float64(x-64), float64(y-64))
			a := 0.0
			if d < 56 {
				a = 1
			} else if d < 62 {
				a = (62 - d) / 6
			}
			lg.Set(x, y, color.NRGBA{20, 90, 170, uint8(a * 255)})
		}
	}
	f, _ = os.Create(filepath.Join(dir, "logo-alpha.png"))
	png.Encode(f, lg)
	f.Close()
	// Graustufen ohne Alpha.
	gr := image.NewGray(image.Rect(0, 0, 200, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 200; x++ {
			gr.SetGray(x, y, color.Gray{uint8(x * 255 / 199)})
		}
	}
	f, _ = os.Create(filepath.Join(dir, "verlauf-grau.png"))
	png.Encode(f, gr)
	f.Close()
}
