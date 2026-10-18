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
	// Trace erhält, wenn gesetzt, je Seite die Operatorbereiche mit der
	// Vorlagen-ID der Bereiche. Die Ausgabe bleibt unverändert.
	Trace *Trace
	// Tagged schreibt Tagged PDF mit Strukturbaum. Profile, die Tagging
	// verlangen (PDF/A Stufe A, PDF/UA), schalten es immer ein.
	Tagged bool
}

// Trace ordnet Operatoren der Inhaltsströme den Vorlagenelementen zu, damit
// Befunde von veraPDF (Kontext pages[i]/contentStream[0]/operators[n]) auf
// Elemente zeigen.
type Trace struct {
	Pages []PageTrace
}

// PageTrace sind die Operatorbereiche einer Seite in Zeichenreihenfolge.
// Innere Bereiche folgen nach dem Bereich ihres Containers.
type PageTrace struct {
	Ops []OpRange
}

// OpRange sind die Operatoren First bis ausschließlich Last eines Bereichs.
type OpRange struct {
	First, Last int
	ID          string
}

// IDAt liefert die Vorlagen-ID zum Operator op auf Seite page (ab 0), den
// kleinsten Bereich, der ihn enthält. Leer, wenn keiner passt.
func (t *Trace) IDAt(page, op int) string {
	if t == nil || page < 0 || page >= len(t.Pages) {
		return ""
	}
	id, size := "", -1
	for _, r := range t.Pages[page].Ops {
		if op >= r.First && op < r.Last && (size < 0 || r.Last-r.First <= size) {
			id, size = r.ID, r.Last-r.First
		}
	}
	return id
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

	pageRefs []pdf.Ref
	pageIdx  int

	// tag plant den Strukturbaum, nil ohne Tagged PDF.
	tag *tagger
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
	if c := profile.ClaimOf(opts.Profile); opts.Tagged || c.Level == "A" || c.UA > 0 {
		r.tag = newTagger(doc)
		r.tag.plan()
	}
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
	// Seitenobjekte vorab reservieren, damit Verweise auf spätere Seiten zeigen
	// können.
	r.pageRefs = make([]pdf.Ref, len(r.doc.Pages))
	for i := range r.doc.Pages {
		r.pageRefs[i] = r.d.Reserve()
	}
	var kids pdf.Array
	for i, p := range r.doc.Pages {
		r.pageIdx = i
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
	if r.tag != nil {
		r.tag.write(r)
	}
	r.info = r.infoDict()
	if len(r.doc.Bookmarks) > 0 {
		outlines, err := r.outlines()
		if err != nil {
			return err
		}
		r.catalog.Set("Outlines", outlines)
	}
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
	if r.opts.Trace != nil {
		pc.trace = &PageTrace{}
	}
	for _, a := range p.Areas {
		if err := pc.area(a); err != nil {
			return pdf.Ref{}, err
		}
	}
	if pc.trace != nil {
		r.opts.Trace.Pages = append(r.opts.Trace.Pages, *pc.trace)
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
	if len(pc.annots) > 0 {
		page.Set("Annots", pc.annots)
	}
	if r.tag != nil {
		if len(r.tag.pages[r.pageIdx]) > 0 {
			page.Set("StructParents", pdf.Int(r.pageIdx))
		}
		// Tabulatorreihenfolge nach Struktur (ISO 14289-1 7.18.3).
		page.Set("Tabs", pdf.Name("S"))
	}
	ref := r.pageRefs[r.pageIdx]
	r.d.Set(ref, page)
	return ref, nil
}
