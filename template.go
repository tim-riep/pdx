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
	"github.com/tim-riep/pdx/fo/tree"
	"github.com/tim-riep/pdx/template"
	"github.com/tim-riep/pdx/template/pkg"
)

// TemplateOptions steuert das Setzen einer Vorlage.
type TemplateOptions struct {
	Options
	// Lang wählt Textressourcen und Formatmuster, leer bedeutet die Sprache
	// der Vorlage.
	Lang string
	// Strict prüft die Daten vor dem Setzen gegen das Daten-XSD des Pakets.
	Strict bool
}

// Template ist eine kompilierte Vorlage. Sie kann für viele Datensätze
// verwendet werden, auch nebenläufig.
type Template struct {
	Package  *pkg.Package
	Compiled *compile.Result
	lang     string
}

// CompileTemplate kompiliert die Vorlage eines Pakets für eine Sprache (leer
// bedeutet die Sprache der Vorlage).
func CompileTemplate(p *pkg.Package, lang string) (*Template, error) {
	res, err := compile.Compile(p.Template, compile.Options{Lang: lang})
	if err != nil {
		return nil, err
	}
	return &Template{Package: p, Compiled: res, lang: lang}, nil
}

// RenderTemplate kompiliert die Vorlage und setzt die Daten. Für viele
// Datensätze ist CompileTemplate mit Template.Render günstiger.
func RenderTemplate(p *pkg.Package, data []byte, opts TemplateOptions) (*Result, error) {
	t, err := CompileTemplate(p, opts.Lang)
	if err != nil {
		return nil, err
	}
	return t.Render(data, opts)
}

// Render setzt einen Datensatz. opts.Lang wird ignoriert, die Sprache steht
// beim Kompilieren fest.
func (t *Template) Render(data []byte, opts TemplateOptions) (*Result, error) {
	fo, o, err := t.prepare(data, opts)
	if err != nil {
		return nil, err
	}
	res, err := Render(bytes.NewReader(fo), o)
	if err != nil {
		return nil, mapError(fo, err)
	}
	return res, nil
}

// Layout setzt einen Datensatz und liefert nur den Area Tree, ohne PDF.
// Profil und Erstellungsdatum werden nicht gebraucht.
func (t *Template) Layout(data []byte, opts TemplateOptions) (*areatree.Document, error) {
	fo, o, err := t.prepare(data, opts)
	if err != nil {
		return nil, err
	}
	doc, err := Layout(bytes.NewReader(fo), o)
	if err != nil {
		return nil, mapError(fo, err)
	}
	return doc, nil
}

// prepare prüft die Daten auf Wunsch, transformiert sie und stellt die
// Optionen für das Paket zusammen.
func (t *Template) prepare(data []byte, opts TemplateOptions) ([]byte, Options, error) {
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
	fo, err := t.Compiled.Transform(data)
	if err != nil {
		return nil, Options{}, err
	}
	o := opts.Options
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

// mapError übersetzt Fehler beim Setzen in Fehler mit Vorlagen-ID. Sie
// beziehen sich auf eine Stelle im FO, das Element dort trägt die ID.
func mapError(fo []byte, err error) error {
	var te *tree.Error
	if errors.As(err, &te) {
		return &template.Error{ID: foIDAt(fo, te.Line, te.Col), Msg: te.Msg}
	}
	return err
}

// Validate prüft Daten gegen das Daten-XSD des Pakets.
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

// packageResolver liest Bilder und Anhänge aus dem Vorlagenpaket.
type packageResolver struct{ p *pkg.Package }

func (r packageResolver) Read(uri string) ([]byte, error) {
	name := path.Clean(strings.TrimPrefix(uri, "./"))
	if b, ok := r.p.File(name); ok {
		return b, nil
	}
	return nil, fmt.Errorf("%s ist nicht im Vorlagenpaket", uri)
}

// foIDAt liefert pdx:id des FO-Elements, das an der Position beginnt oder
// zuletzt davor begonnen hat.
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
