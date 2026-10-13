// Package render schreibt einen Area Tree als PDF. Es übersetzt Bereiche in
// Content Streams, bettet Schriften und Bilder ein und lässt das
// Konformitätsprofil Pflichtobjekte wie Output Intent und XMP ergänzen.
package render

import (
	"fmt"
	"time"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/pdf"
	"github.com/tim-riep/pdx/pdf/profile"
)

// Options steuert die Ausgabe.
type Options struct {
	// Profile ist das Konformitätsprofil. nil bedeutet pdf-1.7.
	Profile profile.Profile
	// CreationDate wird als Erstellungs- und Änderungsdatum geschrieben. Für
	// reproduzierbare Ausgabe muss es gesetzt sein.
	CreationDate time.Time
	// Producer erscheint im Info-Dictionary und im XMP.
	Producer string
}

// Render schreibt das Dokument als PDF-Bytes.
func Render(doc *areatree.Document, opts Options) ([]byte, error) {
	r, err := newRenderer(doc, opts)
	if err != nil {
		return nil, err
	}
	if err := r.build(); err != nil {
		return nil, err
	}
	return r.d.Bytes()
}

type renderer struct {
	doc  *areatree.Document
	opts Options
	d    *pdf.Document

	pages   pdf.Ref
	catalog *pdf.Dict
	info    *pdf.Dict

	fonts  *fontSet
	images *imageSet
	gs     map[string]pdf.Ref
}

func newRenderer(doc *areatree.Document, opts Options) (*renderer, error) {
	if opts.CreationDate.IsZero() {
		return nil, fmt.Errorf("render: CreationDate fehlt")
	}
	if opts.Profile == nil {
		opts.Profile = profile.PDF17{}
	}
	if opts.Producer == "" {
		opts.Producer = "pdx"
	}
	r := &renderer{doc: doc, opts: opts, d: pdf.NewDocument(opts.Profile), gs: map[string]pdf.Ref{}}
	r.fonts = newFontSet(r)
	r.images = newImageSet(r)
	return r, nil
}

// pageRes sammelt die Ressourcen einer Seite.
type pageRes struct {
	fonts    *pdf.Dict
	xobjects *pdf.Dict
	gstates  *pdf.Dict
}

func (r *renderer) build() error {
	if len(r.doc.Pages) == 0 {
		return fmt.Errorf("render: Dokument hat keine Seiten")
	}
	r.pages = r.d.Reserve()
	if err := r.fonts.collect(); err != nil {
		return err
	}
	if err := r.fonts.embed(); err != nil {
		return err
	}
	var kids pdf.Array
	for _, p := range r.doc.Pages {
		ref, err := r.page(p)
		if err != nil {
			return fmt.Errorf("Seite %d: %w", p.Number, err)
		}
		kids = append(kids, ref)
	}
	r.d.Set(r.pages, pdf.D("Type", pdf.Name("Pages"), "Kids", kids, "Count", pdf.Int(len(kids))))

	r.catalog = pdf.D("Type", pdf.Name("Catalog"), "Pages", r.pages)
	if r.doc.Meta.Lang != "" {
		r.catalog.Set("Lang", pdf.TextString(r.doc.Meta.Lang))
	}
	r.info = r.infoDict()
	if err := r.attachments(); err != nil {
		return err
	}
	err := r.opts.Profile.Prepare(&profile.Context{
		Doc: r.d, Catalog: r.catalog, Info: r.info, Meta: r.doc.Meta,
		Date: r.opts.CreationDate, Producer: r.opts.Producer,
	})
	if err != nil {
		return err
	}
	r.d.Root = r.d.Add(r.catalog)
	r.d.Info = r.d.Add(r.info)
	return nil
}

func (r *renderer) infoDict() *pdf.Dict {
	m := r.doc.Meta
	date := pdf.Date(r.opts.CreationDate)
	info := pdf.D("Producer", pdf.TextString(r.opts.Producer), "CreationDate", date, "ModDate", date)
	for _, kv := range [][2]string{{"Title", m.Title}, {"Author", m.Author}, {"Subject", m.Subject},
		{"Keywords", m.Keywords}, {"Creator", m.Creator}} {
		if kv[1] != "" {
			info.Set(pdf.Name(kv[0]), pdf.TextString(kv[1]))
		}
	}
	return info
}

func (r *renderer) page(p *areatree.Page) (pdf.Ref, error) {
	res := &pageRes{fonts: pdf.NewDict(), xobjects: pdf.NewDict(), gstates: pdf.NewDict()}
	pc := &pageCtx{r: r, res: res, h: p.Height.Pt()}
	for _, a := range p.Areas {
		if err := pc.area(a); err != nil {
			return pdf.Ref{}, err
		}
	}
	resources := pdf.NewDict()
	if res.fonts.Len() > 0 {
		resources.Set("Font", res.fonts)
	}
	if res.xobjects.Len() > 0 {
		resources.Set("XObject", res.xobjects)
	}
	if res.gstates.Len() > 0 {
		resources.Set("ExtGState", res.gstates)
	}
	contents := r.d.Add(pdf.NewFlateStream(nil, pc.c.Bytes()))
	page := pdf.D(
		"Type", pdf.Name("Page"),
		"Parent", r.pages,
		"MediaBox", pdf.Array{pdf.Int(0), pdf.Int(0), pdf.Real(p.Width.Pt()), pdf.Real(p.Height.Pt())},
		"Resources", resources,
		"Contents", contents,
	)
	if p.TrimBox != nil {
		page.Set("TrimBox", pc.box(*p.TrimBox))
	}
	if p.BleedBox != nil {
		page.Set("BleedBox", pc.box(*p.BleedBox))
	}
	return r.d.Add(page), nil
}
