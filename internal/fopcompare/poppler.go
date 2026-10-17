package fopcompare

import (
	"context"
	"encoding/xml"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Rasterize rastert alle Seiten in Graustufen.
func Rasterize(ctx context.Context, pdf []byte, dpi int) ([]*image.Gray, error) {
	dir, err := poppler(ctx, pdf, "pdftoppm", "-r", strconv.Itoa(dpi), "-gray", "-png", "in.pdf", "seite")
	if dir != "" {
		defer os.RemoveAll(dir)
	}
	if err != nil {
		return nil, err
	}
	files, _ := filepath.Glob(filepath.Join(dir, "seite-*.png"))
	sort.Slice(files, func(i, j int) bool { return pageNum(files[i]) < pageNum(files[j]) })
	var pages []*image.Gray
	for _, f := range files {
		r, err := os.Open(f)
		if err != nil {
			return nil, err
		}
		img, err := png.Decode(r)
		r.Close()
		if err != nil {
			return nil, err
		}
		g := image.NewGray(img.Bounds())
		draw.Draw(g, g.Bounds(), img, img.Bounds().Min, draw.Src)
		pages = append(pages, g)
	}
	return pages, nil
}

func pageNum(f string) int {
	base := strings.TrimSuffix(filepath.Base(f), ".png")
	n, _ := strconv.Atoi(base[strings.LastIndex(base, "-")+1:])
	return n
}

// Line ist eine Textzeile mit Lage in Punkt (Ursprung oben links).
type Line struct {
	Page       int
	Text       string
	X, Y, W, H float64
}

type bboxDoc struct {
	Pages []struct {
		Flows []struct {
			Blocks []struct {
				Lines []struct {
					XMin  float64 `xml:"xMin,attr"`
					YMin  float64 `xml:"yMin,attr"`
					XMax  float64 `xml:"xMax,attr"`
					YMax  float64 `xml:"yMax,attr"`
					Words []struct {
						XMin float64 `xml:"xMin,attr"`
						Text string  `xml:",chardata"`
					} `xml:"word"`
				} `xml:"line"`
			} `xml:"block"`
		} `xml:"flow"`
	} `xml:"body>doc>page"`
}

// Lines extrahiert die Textzeilen mit pdftotext -bbox-layout.
func Lines(ctx context.Context, pdf []byte) ([]Line, int, error) {
	dir, err := poppler(ctx, pdf, "pdftotext", "-bbox-layout", "-enc", "UTF-8", "in.pdf", "out.html")
	if dir != "" {
		defer os.RemoveAll(dir)
	}
	if err != nil {
		return nil, 0, err
	}
	data, err := os.ReadFile(filepath.Join(dir, "out.html"))
	if err != nil {
		return nil, 0, err
	}
	return ParseBBox(data)
}

// ParseBBox liest die Ausgabe von pdftotext -bbox-layout.
func ParseBBox(data []byte) ([]Line, int, error) {
	var doc bboxDoc
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	dec.Strict = false
	dec.Entity = xml.HTMLEntity
	if err := dec.Decode(&doc); err != nil {
		return nil, 0, fmt.Errorf("pdftotext-Ausgabe: %w", err)
	}
	var out []Line
	for pi, p := range doc.Pages {
		for _, f := range p.Flows {
			for _, b := range f.Blocks {
				for _, l := range b.Lines {
					// Poppler übergeht beim xMin der Zeile Wörter, die genau bei
					// x = 0 beginnen. Der Anfang ist daher das kleinste xMin der
					// Wörter.
					x := l.XMin
					words := make([]string, len(l.Words))
					for i, w := range l.Words {
						words[i] = w.Text
						x = min(x, w.XMin)
					}
					out = append(out, Line{Page: pi + 1, Text: strings.Join(words, " "),
						X: x, Y: l.YMin, W: l.XMax - x, H: l.YMax - l.YMin})
				}
			}
		}
	}
	return out, len(doc.Pages), nil
}
