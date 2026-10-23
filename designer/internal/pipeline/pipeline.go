// Package pipeline typesets the template for the canvas: compile (with a cache
// by content), transform, typeset without PDF, translate pages into the compact
// format and hash them. Only changed pages have to go to the UI.
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
	"github.com/tim-riep/pdx/pdf/profile"
	"github.com/tim-riep/pdx/pdf/render"
	"github.com/tim-riep/pdx/template"
	"github.com/tim-riep/pdx/template/pkg"
)

// Input is a state of the template with data.
type Input struct {
	// Canon is template.xml in canonical form, also the key of the
	// compiler cache.
	Canon []byte
	Files map[string][]byte
	Data  []byte
	Lang  string
}

// Result is the result of a run.
type Result struct {
	Pages    []PageRef `json:"pages"`
	Fonts    []FontRef `json:"fonts"`
	Problems []Problem `json:"problems"`
	// Millis is the duration of the run, Compiled reports whether it compiled.
	Millis   int64 `json:"millis"`
	Compiled bool  `json:"compiled"`
}

// PageRef refers to a page via the hash of its content.
type PageRef struct {
	N    int     `json:"n"`
	W    float64 `json:"w"`
	H    float64 `json:"h"`
	Hash string  `json:"hash"`
}

// FontRef is a font for the canvas.
type FontRef struct {
	Key    string `json:"key"`
	Family string `json:"family"`
}

// Problem is a finding during typesetting with template ID, as far as known.
type Problem struct {
	Level string `json:"level"`
	ID    string `json:"id"`
	Msg   string `json:"msg"`
}

// Pipeline holds the caches across many runs. It is safe for concurrent use.
type Pipeline struct {
	reg *font.Registry

	mu       sync.Mutex
	compiled struct {
		key string
		tpl *pdx.Template
	}
	pages  map[string][]byte // hash to JSON of the page
	order  []string          // order for evicting old pages
	fonts  map[string]*font.Face
	images map[string]*image.Image
}

// maxPages limits the page cache.
const maxPages = 2000

// New creates a pipeline with a shared font registry.
func New() *Pipeline {
	return &Pipeline{reg: font.NewRegistry(), pages: map[string][]byte{}, fonts: map[string]*font.Face{}, images: map[string]*image.Image{}}
}

// Run typesets a state. A canceled context returns ctx.Err().
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

// compile returns the compiled template, from the cache if the
// canonical content has not changed.
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

// PageJSON returns a page by its hash.
func (p *Pipeline) PageJSON(hash string) ([]byte, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	b, ok := p.pages[hash]
	return b, ok
}

// Font returns the bytes of a font by its key.
func (p *Pipeline) Font(key string) ([]byte, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	f, ok := p.fonts[key]
	if !ok {
		return nil, false
	}
	return f.Data(), true
}

// Image returns an image for the browser: JPEG unchanged, otherwise PNG.
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

// encodePNG re-encodes the uncompressed pixels of a PNG image.
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

// PDF generates the PDF of a state in the profile of the template and with its
// attachments. created is the creation date. trace receives, if set, the mapping of the
// operators to the template elements. Errors carry the template ID as in
// the findings of Run.
func (p *Pipeline) PDF(ctx context.Context, in Input, created time.Time, trace *render.Trace) ([]byte, []Problem, error) {
	tpl, _, err := p.compile(in)
	if err != nil {
		return nil, problems(err), nil
	}
	res, err := tpl.Render(in.Data, pdx.TemplateOptions{Options: pdx.Options{Fonts: p.reg, Context: ctx, Trace: trace, CreationDate: created}})
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil, nil, err
	}
	if err != nil {
		return nil, problems(err), nil
	}
	return res.PDF, nil, nil
}

// problems translates errors of the engine into findings with template ID.
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

// AccessProfile is the profile Access checks against when the profile of the
// template does not require accessibility.
const AccessProfile = "pdfa-3a,pdfua-1"

// Access is the accessibility analysis of a state.
type Access struct {
	// Profile is the checked profile. Assumed reports that the profile of the
	// template does not require accessibility and the check fell back to
	// AccessProfile.
	Profile string `json:"profile"`
	Assumed bool   `json:"assumed"`
	// Tree is the structure tree in reading order, nil if typesetting
	// failed.
	Tree *render.StructNode `json:"tree"`
	// Findings are the findings of the pre-check, Problems errors during typesetting.
	Findings []Problem `json:"findings"`
	Problems []Problem `json:"problems"`
}

// Access typesets a state and analyzes the structure tree and pre-check, without
// writing a PDF.
func (p *Pipeline) Access(ctx context.Context, in Input) (*Access, error) {
	res := &Access{Findings: []Problem{}, Problems: []Problem{}}
	tpl, _, err := p.compile(in)
	if err != nil {
		res.Problems = problems(err)
		return res, nil
	}
	res.Profile = tpl.Profile()
	prof, err := profile.Lookup(res.Profile)
	if err != nil {
		res.Problems = problems(err)
		return res, nil
	}
	c := profile.ClaimOf(prof)
	if c.Level != "A" && c.UA == 0 {
		res.Profile, res.Assumed = AccessProfile, true
		p, _ := profile.Lookup(AccessProfile)
		c = profile.ClaimOf(p)
	}
	doc, err := tpl.Layout(in.Data, pdx.TemplateOptions{Options: pdx.Options{Fonts: p.reg, Context: ctx}})
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil, err
	}
	if err != nil {
		res.Problems = problems(err)
		return res, nil
	}
	tree, errs := render.Analyze(doc, c)
	res.Tree = tree
	for _, e := range errs {
		res.Findings = append(res.Findings, Problem{Level: "Fehler", ID: e.ID, Msg: e.Msg})
	}
	return res, nil
}
