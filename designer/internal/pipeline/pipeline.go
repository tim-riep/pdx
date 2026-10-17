// Package pipeline setzt die Vorlage für den Canvas: kompilieren (mit Cache
// nach Inhalt), transformieren, setzen ohne PDF, Seiten in das kompakte
// Format übersetzen und hashen. Nur geänderte Seiten müssen zur Oberfläche.
package pipeline

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	stdimage "image"
	stdcolor "image/color"
	"image/png"
	"sync"
	"time"

	"github.com/tim-riep/pdx"
	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/font"
	"github.com/tim-riep/pdx/image"
	"github.com/tim-riep/pdx/template"
	"github.com/tim-riep/pdx/template/pkg"
)

// Input ist ein Stand der Vorlage mit Daten.
type Input struct {
	// Canon ist template.xml in kanonischer Form, zugleich Schlüssel des
	// Compiler-Caches.
	Canon []byte
	Files map[string][]byte
	Data  []byte
	Lang  string
}

// Result ist das Ergebnis eines Laufs.
type Result struct {
	Pages    []PageRef `json:"pages"`
	Fonts    []FontRef `json:"fonts"`
	Problems []Problem `json:"problems"`
	// Millis ist die Dauer des Laufs, Compiled meldet, ob kompiliert wurde.
	Millis   int64 `json:"millis"`
	Compiled bool  `json:"compiled"`
}

// PageRef verweist auf eine Seite über den Hash ihres Inhalts.
type PageRef struct {
	N    int     `json:"n"`
	W    float64 `json:"w"`
	H    float64 `json:"h"`
	Hash string  `json:"hash"`
}

// FontRef ist eine Schrift für den Canvas.
type FontRef struct {
	Key    string `json:"key"`
	Family string `json:"family"`
}

// Problem ist ein Befund beim Setzen mit Vorlagen-ID, soweit bekannt.
type Problem struct {
	Level string `json:"level"`
	ID    string `json:"id"`
	Msg   string `json:"msg"`
}

// Pipeline hält die Caches über viele Läufe. Sie ist nebenläufig nutzbar.
type Pipeline struct {
	reg *font.Registry

	mu       sync.Mutex
	compiled struct {
		key string
		tpl *pdx.Template
	}
	pages  map[string][]byte // Hash zu JSON der Seite
	order  []string          // Reihenfolge für das Verdrängen alter Seiten
	fonts  map[string]*font.Face
	images map[string]*image.Image
}

// maxPages begrenzt den Seiten-Cache.
const maxPages = 2000

// New legt eine Pipeline mit gemeinsamer Schrift-Registry an.
func New() *Pipeline {
	return &Pipeline{reg: font.NewRegistry(), pages: map[string][]byte{}, fonts: map[string]*font.Face{}, images: map[string]*image.Image{}}
}

// Run setzt einen Stand. Ein abgebrochener Kontext liefert ctx.Err().
func (p *Pipeline) Run(ctx context.Context, in Input) (*Result, error) {
	start := time.Now()
	res := &Result{Pages: []PageRef{}, Fonts: []FontRef{}, Problems: []Problem{}}
	tpl, compiled, err := p.compile(in)
	if err != nil {
		res.Problems = append(res.Problems, problems(err)...)
		res.Millis = time.Since(start).Milliseconds()
		return res, nil
	}
	res.Compiled = compiled
	doc, err := tpl.Layout(in.Data, pdx.TemplateOptions{Options: pdx.Options{Fonts: p.reg, Context: ctx}})
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil, err
	}
	if err != nil {
		res.Problems = append(res.Problems, problems(err)...)
		res.Millis = time.Since(start).Milliseconds()
		return res, nil
	}
	fontKeys := make([]string, len(doc.Fonts))
	p.mu.Lock()
	for i, f := range doc.Fonts {
		if face, ok := f.Source.(*font.Face); ok {
			k := face.Key()[:16]
			fontKeys[i] = k
			p.fonts[k] = face
			res.Fonts = append(res.Fonts, FontRef{Key: k, Family: "pdx-" + k})
		}
	}
	imageKeys := make([]string, len(doc.Images))
	for i, im := range doc.Images {
		if img, ok := im.Source.(*image.Image); ok {
			k := img.Key()[:16]
			imageKeys[i] = k
			p.images[k] = img
		}
	}
	p.mu.Unlock()
	for _, pg := range doc.Pages {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		page := convert(pg, func(id areatree.FontID) string { return fontKeys[id] },
			func(id areatree.ImageID) string { return imageKeys[id] })
		b, err := json.Marshal(page)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(b)
		h := hex.EncodeToString(sum[:12])
		p.storePage(h, b)
		res.Pages = append(res.Pages, PageRef{N: page.N, W: page.W, H: page.H, Hash: h})
	}
	res.Millis = time.Since(start).Milliseconds()
	return res, nil
}

// compile liefert die kompilierte Vorlage, aus dem Cache, wenn sich der
// kanonische Inhalt nicht geändert hat.
func (p *Pipeline) compile(in Input) (*pdx.Template, bool, error) {
	sum := sha256.Sum256(append(append([]byte(in.Lang), 0), in.Canon...))
	key := hex.EncodeToString(sum[:])
	p.mu.Lock()
	if p.compiled.key == key {
		tpl := p.compiled.tpl
		p.mu.Unlock()
		return tpl, false, nil
	}
	p.mu.Unlock()
	t, err := template.ParseTree(in.Canon)
	if err != nil {
		return nil, false, err
	}
	pk, err := pkg.New(t, in.Files)
	if err != nil {
		return nil, false, err
	}
	tpl, err := pdx.CompileTemplate(pk, in.Lang)
	if err != nil {
		return nil, false, err
	}
	p.mu.Lock()
	p.compiled.key, p.compiled.tpl = key, tpl
	p.mu.Unlock()
	return tpl, true, nil
}

func (p *Pipeline) storePage(h string, b []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.pages[h]; ok {
		return
	}
	p.pages[h] = b
	p.order = append(p.order, h)
	for len(p.order) > maxPages {
		delete(p.pages, p.order[0])
		p.order = p.order[1:]
	}
}

// PageJSON liefert eine Seite nach ihrem Hash.
func (p *Pipeline) PageJSON(hash string) ([]byte, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	b, ok := p.pages[hash]
	return b, ok
}

// Font liefert die Bytes einer Schrift nach ihrem Schlüssel.
func (p *Pipeline) Font(key string) ([]byte, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	f, ok := p.fonts[key]
	if !ok {
		return nil, false
	}
	return f.Data(), true
}

// Image liefert ein Bild für den Browser: JPEG unverändert, sonst PNG.
func (p *Pipeline) Image(key string) ([]byte, string, bool) {
	p.mu.Lock()
	im, ok := p.images[key]
	p.mu.Unlock()
	if !ok {
		return nil, "", false
	}
	if im.Format == image.JPEG {
		return im.Data, "image/jpeg", true
	}
	b, err := encodePNG(im)
	if err != nil {
		return nil, "", false
	}
	return b, "image/png", true
}

// encodePNG kodiert die unkomprimierten Pixel eines PNG-Bildes neu.
func encodePNG(im *image.Image) ([]byte, error) {
	r := stdimage.Rect(0, 0, im.Width, im.Height)
	out := stdimage.NewNRGBA(r)
	n := 3
	if im.Space == image.Gray {
		n = 1
	}
	for y := 0; y < im.Height; y++ {
		for x := 0; x < im.Width; x++ {
			i := (y*im.Width + x) * n
			if i+n > len(im.Data) {
				return nil, errors.New("Bilddaten unvollständig")
			}
			c := stdcolor.NRGBA{A: 255}
			if n == 1 {
				c.R, c.G, c.B = im.Data[i], im.Data[i], im.Data[i]
			} else {
				c.R, c.G, c.B = im.Data[i], im.Data[i+1], im.Data[i+2]
			}
			if im.Alpha != nil {
				c.A = im.Alpha[y*im.Width+x]
			}
			out.SetNRGBA(x, y, c)
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, out); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// problems übersetzt Fehler der Engine in Befunde mit Vorlagen-ID.
func problems(err error) []Problem {
	var es template.Errors
	var e *template.Error
	switch {
	case errors.As(err, &es):
		out := make([]Problem, 0, len(es))
		for _, e := range es {
			out = append(out, Problem{Level: "Fehler", ID: e.ID, Msg: e.Msg})
		}
		return out
	case errors.As(err, &e):
		return []Problem{{Level: "Fehler", ID: e.ID, Msg: e.Msg}}
	}
	return []Problem{{Level: "Fehler", Msg: err.Error()}}
}
