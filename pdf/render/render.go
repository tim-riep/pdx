// Package render writes an area tree as PDF. It translates areas into
// content streams, embeds fonts and images and lets the
// conformance profile add required objects such as output intent and XMP.
package render

import (
	"fmt"
	"time"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/pdf"
	"github.com/tim-riep/pdx/pdf/profile"
)

// Options controls the output.
type Options struct {
	// Profile is the conformance profile. nil means pdf-1.7.
	Profile profile.Profile
	// CreationDate is written as creation and modification date. For
	// reproducible output it must be set.
	CreationDate time.Time
	// Producer appears in the info dictionary and in the XMP.
	Producer string
	// Trace receives, if set, the operator ranges per page with the
	// template ID of the areas. The output stays unchanged.
	Trace *Trace
	// Tagged writes Tagged PDF with a structure tree. Profiles that require
	// tagging (PDF/A level A, PDF/UA) always switch it on.
	Tagged bool
	// Flatten draws form fields as fixed page content instead of as
	// fillable fields (pdx flatten).
	Flatten bool
}

// Trace maps operators of the content streams to the template elements so
// that findings of veraPDF (context pages[i]/contentStream[0]/operators[n])
// point to elements.
type Trace struct {
	Pages []PageTrace
}

// PageTrace are the operator ranges of a page in drawing order.
// Inner ranges follow after the range of their container.
type PageTrace struct {
	Ops []OpRange
}

// OpRange are the operators First up to but excluding Last of an area.
type OpRange struct {
	First, Last int
	ID          string
}

// IDAt returns the template ID for the operator op on page page (from 0), the
// smallest range that contains it. Empty if none matches.
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

// Render writes the document as PDF bytes.
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

	// tag plans the structure tree, nil without Tagged PDF.
	tag *tagger
	// claim is the claim of the profile with PDF version.
	claim profile.Claim
	// form collects the form fields, nil without fields. flat counts the
	// flattened fields for resource names.
	form *formState
	flat int
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
	r.claim = profile.ClaimOf(opts.Profile)
	r.d.Version = r.claim.Version
	r.fonts = newFontSet(r)
	r.images = newImageSet(r)
	c := r.claim
	if strict := c.Level == "A" || c.UA > 0; opts.Tagged || strict {
		r.tag = newTagger(doc)
		r.tag.pdf2 = c.Version == "2.0"
		r.tag.flatten = opts.Flatten
		r.tag.plan()
		if strict {
			if err := r.tag.check(c); err != nil {
				return nil, err
			}
		}
	}
	return r, nil
}

// pageRes collects the resources of a page.
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
	// Reserve page objects in advance so that references can point to later
	// pages.
	r.pageRefs = make([]pdf.Ref, len(r.doc.Pages))
	for i := range r.doc.Pages {
		r.pageRefs[i] = r.d.Reserve()
	}
	if r.tag != nil {
		r.tag.prepare(r)
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
	r.writeForm()
	if r.tag != nil {
		r.tag.write(r)
	}
	// PDF 2.0 replaces the info dictionary with XMP (ISO 32000-2 14.3.3),
	// PDF/A-4 forbids it without PieceInfo (ISO 19005-4 6.1.3).
	if r.claim.Version != "2.0" {
		r.info = r.infoDict()
	}
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
	if r.info != nil {
		r.d.Info = r.d.Add(r.info)
	}
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
		// Tab order by structure (ISO 14289-1 7.18.3).
		page.Set("Tabs", pdf.Name("S"))
	}
	ref := r.pageRefs[r.pageIdx]
	r.d.Set(ref, page)
	return ref, nil
}
