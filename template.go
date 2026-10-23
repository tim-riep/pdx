package pdx

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	"github.com/tim-riep/go-xslt/engine"
	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/compile"
	"github.com/tim-riep/pdx/dataschema"
	"github.com/tim-riep/pdx/einvoice"
	"github.com/tim-riep/pdx/fo/tree"
	"github.com/tim-riep/pdx/font"
	"github.com/tim-riep/pdx/pdf/profile"
	"github.com/tim-riep/pdx/pdf/render"
	"github.com/tim-riep/pdx/template"
	"github.com/tim-riep/pdx/template/pkg"
)

// TemplateOptions controls the typesetting of a template.
type TemplateOptions struct {
	Options
	// Lang selects text resources and format patterns, empty means the
	// language of the template.
	Lang string
	// Strict validates the data against the package's data XSD before typesetting.
	Strict bool
}

// Template is a compiled template. It can be used for many data records,
// also concurrently.
type Template struct {
	Package  *pkg.Package
	Compiled *compile.Result
	lang     string
	fonts    []*font.Face
}

// CompileTemplate compiles the template of a package for a language (empty
// means the language of the template).
func CompileTemplate(p *pkg.Package, lang string) (*Template, error) {
	res, err := compile.Compile(p.Template, compileOptions(p, lang))
	if err != nil {
		return nil, err
	}
	fonts, err := packageFonts(p)
	if err != nil {
		return nil, err
	}
	return &Template{Package: p, Compiled: res, lang: lang, fonts: fonts}, nil
}

// compileOptions assembles the language and the facets from the data XSD
// for the compiler.
func compileOptions(p *pkg.Package, lang string) compile.Options {
	f := dataschema.PackageFacets(p)
	return compile.Options{Lang: lang, Enums: f.Enums, MaxLengths: f.MaxLengths}
}

// packageFonts loads the fonts under assets/ of a package.
func packageFonts(p *pkg.Package) ([]*font.Face, error) {
	var out []*font.Face
	for _, name := range p.Names() {
		ext := strings.ToLower(path.Ext(name))
		if !strings.HasPrefix(name, "assets/") || ext != ".ttf" && ext != ".otf" && ext != ".ttc" {
			continue
		}
		data, _ := p.File(name)
		f, err := font.Load(data, 0)
		if err != nil {
			return nil, &template.Error{Msg: fmt.Sprintf("Schrift %s: %v", name, err)}
		}
		out = append(out, f)
	}
	return out, nil
}

// RenderTemplate compiles the template and typesets the data. For many
// data records, CompileTemplate with Template.Render is cheaper.
func RenderTemplate(p *pkg.Package, data []byte, opts TemplateOptions) (*Result, error) {
	t, err := CompileTemplate(p, opts.Lang)
	if err != nil {
		return nil, err
	}
	return t.Render(data, opts)
}

// Render typesets a data record. opts.Lang is ignored, the language is
// fixed at compile time.
func (t *Template) Render(data []byte, opts TemplateOptions) (*Result, error) {
	fo, o, err := t.prepare(data, opts, true)
	if err != nil {
		return nil, err
	}
	if o.Profile == "" {
		o.Profile = t.Profile()
	}
	if err := t.checkEInvoice(o.Profile); err != nil {
		return nil, err
	}
	res, err := Render(bytes.NewReader(fo), o)
	if err != nil {
		return nil, mapError(fo, err)
	}
	return res, nil
}

// checkEInvoice requires PDF/A-3 for an e-invoice. Factur-X and ZUGFeRD
// only describe PDF/A-3 (Factur-X 1.0 chapter 6, ZUGFeRD 2 part A).
func (t *Template) checkEInvoice(spec string) error {
	e := t.Compiled.EInvoice
	if e == nil {
		return nil
	}
	p, err := profile.Lookup(spec)
	if err != nil {
		return err
	}
	if c := profile.ClaimOf(p); c.PDFA != 3 {
		return &template.Error{ID: e.ID, Msg: fmt.Sprintf("E-Rechnung verlangt PDF/A-3 (pdfa-3b, pdfa-3u oder pdfa-3a,pdfua-1), angegeben ist %s", spec)}
	}
	return nil
}

// Profile returns the profiles of the template (template/@profile), or the
// default profile if none is specified.
func (t *Template) Profile() string {
	return t.Package.Template.Root.Get("profile", DefaultProfile)
}

// Layout typesets a data record and returns only the area tree, without PDF.
// Profile and creation date are not needed.
func (t *Template) Layout(data []byte, opts TemplateOptions) (*areatree.Document, error) {
	fo, o, err := t.prepare(data, opts, false)
	if err != nil {
		return nil, err
	}
	doc, err := Layout(bytes.NewReader(fo), o)
	if err != nil {
		return nil, mapError(fo, err)
	}
	return doc, nil
}

// prepare validates the data if requested, transforms it and assembles the
// options for the package. pdf reports that a PDF is produced: only then is
// an e-invoice detected and embedded. The layout preview does not need this
// and also works with placeholder data.
func (t *Template) prepare(data []byte, opts TemplateOptions, pdf bool) ([]byte, Options, error) {
	if opts.Strict {
		if err := t.Validate(data); err != nil {
			return nil, Options{}, err
		}
	}
	if opts.Context != nil {
		if err := opts.Context.Err(); err != nil {
			return nil, Options{}, err
		}
	}
	// An e-invoice is detected before the transformation so that wrong
	// data (for example UBL) gets a precise message.
	var info *einvoice.Info
	if e := t.Compiled.EInvoice; e != nil && pdf {
		var err error
		if info, err = einvoice.Detect(data); err != nil {
			return nil, Options{}, &template.Error{ID: e.ID, Msg: err.Error()}
		}
	}
	fo, err := t.Compiled.Transform(data)
	if err != nil {
		return nil, Options{}, err
	}
	o := opts.Options
	if info != nil {
		o.Attachments = append(o.Attachments, info.Attachment(data, areatree.Relationship(t.Compiled.EInvoice.Relationship)))
		o.XMP = append(o.XMP, info.XMP())
	}
	if len(t.fonts) > 0 {
		// Fonts of the package take precedence over the built-in ones. Add
		// registers each font only once per registry.
		if o.Fonts == nil {
			o.Fonts = font.NewRegistry()
		}
		for _, f := range t.fonts {
			o.Fonts.Add(f)
		}
	}
	o.Resolver = packageResolver{t.Package}
	o.BaseDir = ""
	for _, a := range t.Compiled.Attachments {
		o.Attachments = append(o.Attachments, areatree.Attachment{
			Name: a.Name, MIMEType: a.MIMEType, Relationship: areatree.Relationship(a.Relationship),
			Description: a.Description, Data: data,
		})
	}
	return fo, o, nil
}

// mapError translates typesetting errors into errors with a template ID.
// They refer to a position in the FO, the element there carries the ID.
func mapError(fo []byte, err error) error {
	var ce render.CheckErrors
	if errors.As(err, &ce) {
		es := make(template.Errors, len(ce))
		for i, e := range ce {
			es[i] = &template.Error{ID: e.ID, Msg: e.Msg}
		}
		return es
	}
	var te *tree.Error
	if errors.As(err, &te) {
		return &template.Error{ID: foIDAt(fo, te.Line, te.Col), Msg: te.Msg}
	}
	return err
}

// Validate validates data against the package's data XSD.
func (t *Template) Validate(data []byte) error {
	main, files := t.Package.Schemas()
	if main == "" {
		return fmt.Errorf("die Vorlage nennt kein Daten-XSD (template/@schema), strenge Prüfung nicht möglich")
	}
	docs := []string{string(files[main])}
	var names []string
	for n := range files {
		if n != main {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	for _, n := range names {
		docs = append(docs, string(files[n]))
	}
	res := engine.Validate(engine.ValidateRequest{Schemas: docs, Instance: string(data)})
	if res.Valid {
		return nil
	}
	var msgs []string
	for _, d := range res.Diagnostics {
		if d.Severity != engine.SeverityError {
			continue
		}
		if d.Line > 0 {
			msgs = append(msgs, fmt.Sprintf("%d:%d: %s", d.Line, d.Col, d.Message))
		} else {
			msgs = append(msgs, d.Message)
		}
	}
	if len(msgs) == 0 {
		msgs = append(msgs, "ungültig")
	}
	return fmt.Errorf("Daten entsprechen nicht %s:\n%s", main, strings.Join(msgs, "\n"))
}

// packageResolver reads images and attachments from the template package.
type packageResolver struct{ p *pkg.Package }

func (r packageResolver) Read(uri string) ([]byte, error) {
	name := path.Clean(strings.TrimPrefix(uri, "./"))
	if b, ok := r.p.File(name); ok {
		return b, nil
	}
	return nil, fmt.Errorf("%s ist nicht im Vorlagenpaket", uri)
}

// foIDAt returns the pdx:id of the FO element that starts at the position
// or that started most recently before it.
func foIDAt(fo []byte, line, col int) string {
	dec := xml.NewDecoder(bytes.NewReader(fo))
	id := ""
	for {
		l, c := dec.InputPos()
		if l > line || l == line && c > col {
			return id
		}
		tok, err := dec.Token()
		if err == io.EOF || err != nil {
			return id
		}
		if se, ok := tok.(xml.StartElement); ok {
			for _, a := range se.Attr {
				if a.Name.Space == tree.NSPDX && a.Name.Local == "id" {
					id = a.Value
				}
			}
		}
	}
}
