package pdx

import (
	"encoding/xml"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/tim-riep/pdx/dataschema"
	"github.com/tim-riep/pdx/internal/dataxml"
	"github.com/tim-riep/pdx/pdf/reader"
	"github.com/tim-riep/pdx/template"
)

// ExtractError reports that the extracted data does not conform to the data
// XSD. Data still contains it so that it can be inspected.
type ExtractError struct {
	Data []byte
	Err  error
}

func (e *ExtractError) Error() string { return e.Err.Error() }
func (e *ExtractError) Unwrap() error { return e.Err }

// ExtractData reads the values of a filled-in form and writes them into the
// data. The basis is base, otherwise the data embedded in the PDF
// (attachment with source="data"), otherwise an empty root element. Each
// field finds its data node via TM: the path of the context node (fn:path)
// and the template ID whose binding leads from there to the value. Missing
// elements are created in the order of the data XSD. Afterwards the data is
// validated against the XSD, reporting all errors at once. Since forms
// contain no JavaScript, this is the actual validation of the input.
func (t *Template) ExtractData(pdf, base []byte) ([]byte, error) {
	f, err := reader.Open(pdf)
	if err != nil {
		return nil, err
	}
	fields := f.Fields()
	if len(fields) == 0 {
		return nil, fmt.Errorf("das PDF enthält keine Formularfelder")
	}
	if base == nil {
		base, err = t.embeddedData(f)
		if err != nil {
			return nil, err
		}
	}
	ex, err := t.newExtractor(base)
	if err != nil {
		return nil, err
	}
	var errs template.Errors
	for _, fl := range fields {
		if fl.TM == "" || fl.Type == "Sig" {
			continue
		}
		// TM is contextpath|templateID, the ID contains no |.
		i := strings.LastIndexByte(fl.TM, '|')
		if i < 0 {
			errs = append(errs, &template.Error{Msg: fmt.Sprintf("Feld %s: TM %q ist unbekannt aufgebaut", fl.Name, fl.TM)})
			continue
		}
		ctx, id := fl.TM[:i], fl.TM[i+1:]
		if err := ex.set(ctx, id, fl); err != nil {
			errs = append(errs, &template.Error{ID: id, Msg: fmt.Sprintf("Feld %s: %v", fl.Name, err)})
		}
	}
	if len(errs) > 0 {
		return nil, errs
	}
	out := ex.doc.Bytes(ex.prefixes)
	if main, _ := t.Package.Schemas(); main != "" {
		if err := t.Validate(out); err != nil {
			return out, &ExtractError{Data: out, Err: err}
		}
	}
	return out, nil
}

// embeddedData looks for the data that the template embeds as an attachment,
// or returns an empty root element according to template/@dataRoot.
func (t *Template) embeddedData(f *reader.File) ([]byte, error) {
	files, err := f.EmbeddedFiles()
	if err != nil {
		return nil, err
	}
	for _, a := range t.Compiled.Attachments {
		if data, ok := files[a.Name]; ok {
			return data, nil
		}
	}
	if e := t.Compiled.EInvoice; e != nil {
		for _, name := range []string{"factur-x.xml", "xrechnung.xml"} {
			if data, ok := files[name]; ok {
				return data, nil
			}
		}
	}
	root := t.Package.Template.Root
	dr := root.Get("dataRoot", "")
	if dr == "" {
		return nil, fmt.Errorf("weder eingebettete Daten noch template/@dataRoot, die Daten lassen sich nicht aufbauen")
	}
	prefix, local, ok := strings.Cut(dr, ":")
	if !ok {
		return []byte("<" + dr + "/>"), nil
	}
	uri := t.namespaces()[prefix]
	var b strings.Builder
	b.WriteString("<" + prefix + ":" + local + ` xmlns:` + prefix + `="`)
	xml.EscapeText(&b, []byte(uri))
	b.WriteString(`"/>`)
	return []byte(b.String()), nil
}

// namespaces returns the prefixes of the template.
func (t *Template) namespaces() map[string]string {
	ns := map[string]string{}
	if nss := t.Package.Template.Root.First("namespaces"); nss != nil {
		for _, n := range nss.Elements("ns") {
			ns[n.Get("prefix", "")] = n.Get("uri", "")
		}
	}
	return ns
}

// extractor writes values into a data document.
type extractor struct {
	doc      *dataxml.Document
	ns       map[string]string
	prefixes map[string]string // namespace to template prefix
	nodes    map[string]*template.Node
	model    dataschema.Model
	decls    map[*dataxml.Node]*dataschema.Decl
}

func (t *Template) newExtractor(base []byte) (*extractor, error) {
	doc, err := dataxml.Parse(base)
	if err != nil {
		return nil, err
	}
	ex := &extractor{doc: doc, ns: t.namespaces(), prefixes: map[string]string{},
		nodes: map[string]*template.Node{}, decls: map[*dataxml.Node]*dataschema.Decl{}}
	for p, u := range ex.ns {
		ex.prefixes[u] = p
	}
	var walk func(n *template.Node)
	walk = func(n *template.Node) {
		if id := n.ID(); id != "" {
			ex.nodes[id] = n
		}
		for _, c := range n.Elements("") {
			walk(c)
		}
	}
	walk(t.Package.Template.Root)
	if main, files := t.Package.Schemas(); main != "" {
		if m, err := dataschema.FromXSD(main, files); err == nil {
			ex.model = m
		}
	}
	return ex, nil
}

// set writes the value of a field.
func (ex *extractor) set(ctx, id string, fl reader.Field) error {
	n := ex.nodes[id]
	if n == nil {
		return fmt.Errorf("Vorlagenelement %s fehlt in der Vorlage", id)
	}
	var v string
	switch n.Name {
	case "textField", "comboBox", "listBox":
		v = fl.Value
	case "checkBox":
		v = n.Get("offValue", "false")
		if fl.Value != "" && fl.Value != "Off" {
			v = n.Get("onValue", "true")
		}
	case "radioGroup":
		if fl.Value == "Off" {
			return nil // no selection, the data stays as it is
		}
		v = fl.Value
	default:
		return fmt.Errorf("%s ist kein Formularfeld", n.Name)
	}
	// An empty field does not create missing nodes, an empty element
	// would be invalid for optional typed nodes.
	if v == "" && !ex.exists(ctx, n.Get("bind", "")) {
		return nil
	}
	cur, err := ex.context(ctx)
	if err != nil {
		return err
	}
	steps := strings.Split(strings.TrimSpace(n.Get("bind", "")), "/")
	for i, st := range steps {
		st = strings.TrimSpace(st)
		last := i == len(steps)-1
		switch {
		case st == "" || st == ".":
		case st == "..":
			if cur.Parent == nil {
				return fmt.Errorf("Bindung führt über die Wurzel der Daten hinaus")
			}
			cur = cur.Parent
		case strings.HasPrefix(st, "@"):
			if !last {
				return fmt.Errorf("Attribut %s ist nicht der letzte Schritt der Bindung", st)
			}
			name, err := ex.qname(st[1:], true)
			if err != nil {
				return err
			}
			cur.SetAttr(name, v)
			return nil
		default:
			name, err := ex.qname(st, false)
			if err != nil {
				return err
			}
			cur = ex.child(cur, name, 1)
		}
	}
	cur.SetText(v)
	return nil
}

// exists reports whether the data node for context path and binding already
// exists.
func (ex *extractor) exists(ctx, bind string) bool {
	steps, err := parsePath(ctx)
	if err != nil || len(steps) == 0 || ex.doc.Root.Name != steps[0].name {
		return false
	}
	cur := ex.doc.Root
	for _, st := range steps[1:] {
		kids := cur.Elements(st.name)
		if len(kids) < st.pos {
			return false
		}
		cur = kids[st.pos-1]
	}
	for _, st := range strings.Split(strings.TrimSpace(bind), "/") {
		st = strings.TrimSpace(st)
		switch {
		case st == "" || st == ".":
		case st == "..":
			if cur.Parent == nil {
				return false
			}
			cur = cur.Parent
		case strings.HasPrefix(st, "@"):
			name, err := ex.qname(st[1:], true)
			if err != nil {
				return false
			}
			_, ok := cur.Attr(name)
			return ok
		default:
			name, err := ex.qname(st, false)
			if err != nil {
				return false
			}
			kids := cur.Elements(name)
			if len(kids) == 0 {
				return false
			}
			cur = kids[0]
		}
	}
	return true
}

// qname resolves a step with a template prefix.
func (ex *extractor) qname(s string, attr bool) (xml.Name, error) {
	if strings.ContainsAny(s, "[]()*") {
		return xml.Name{}, fmt.Errorf("Schritt %q ist kein einfacher Name", s)
	}
	p, local, ok := strings.Cut(s, ":")
	if !ok {
		return xml.Name{Local: s}, nil
	}
	uri, ok := ex.ns[p]
	if !ok {
		return xml.Name{}, fmt.Errorf("Präfix %s ist in der Vorlage nicht deklariert", p)
	}
	return xml.Name{Space: uri, Local: local}, nil
}

// context finds the element for the path from fn:path, for example
// /Q{uri}antrag[1]/Q{uri}person[2], and creates missing elements.
func (ex *extractor) context(path string) (*dataxml.Node, error) {
	steps, err := parsePath(path)
	if err != nil {
		return nil, err
	}
	if len(steps) == 0 {
		return nil, fmt.Errorf("Kontextpfad %q ist leer", path)
	}
	root := ex.doc.Root
	if root.Name != steps[0].name {
		return nil, fmt.Errorf("Kontextpfad %q passt nicht zur Wurzel {%s}%s der Daten", path, root.Name.Space, root.Name.Local)
	}
	cur := root
	for _, st := range steps[1:] {
		cur = ex.child(cur, st.name, st.pos)
	}
	return cur, nil
}

type pathStep struct {
	name xml.Name
	pos  int
}

// parsePath splits a path from fn:path (XPath 3.1 section 14.6.2).
func parsePath(p string) ([]pathStep, error) {
	var out []pathStep
	bad := fmt.Errorf("Kontextpfad %q ist kein Pfad aus fn:path", p)
	for len(p) > 0 {
		if p[0] != '/' {
			return nil, bad
		}
		p = p[1:]
		var name xml.Name
		if strings.HasPrefix(p, "Q{") {
			end := strings.IndexByte(p, '}')
			if end < 0 {
				return nil, bad
			}
			name.Space = p[2:end]
			p = p[end+1:]
		}
		open := strings.IndexByte(p, '[')
		close := strings.IndexByte(p, ']')
		if open <= 0 || close < open {
			return nil, bad
		}
		name.Local = p[:open]
		pos, err := strconv.Atoi(p[open+1 : close])
		if err != nil || pos < 1 {
			return nil, bad
		}
		out = append(out, pathStep{name, pos})
		p = p[close+1:]
	}
	return out, nil
}

// child returns the pos-th child element with the name and creates missing
// ones in the order of the data XSD.
func (ex *extractor) child(parent *dataxml.Node, name xml.Name, pos int) *dataxml.Node {
	kids := parent.Elements(name)
	for len(kids) < pos {
		n := &dataxml.Node{Kind: dataxml.Element, Name: name}
		parent.Insert(n, ex.insertAt(parent, name))
		kids = parent.Elements(name)
	}
	return kids[pos-1]
}

// insertAt determines the position among the child elements where a new
// element belongs according to the content model: after the last siblings
// with the same or an earlier name. Without a model, at the end.
func (ex *extractor) insertAt(parent *dataxml.Node, name xml.Name) int {
	order := ex.order(parent)
	rank, ok := order[name]
	elems := parent.Elements(xml.Name{})
	if !ok {
		return len(elems)
	}
	at := 0
	for i, e := range elems {
		if r, ok := order[e.Name]; ok && r <= rank {
			at = i + 1
		}
	}
	return at
}

// order returns the order of the child elements of a data element according
// to the data XSD.
func (ex *extractor) order(n *dataxml.Node) map[xml.Name]int {
	d := ex.decl(n)
	out := map[xml.Name]int{}
	if d == nil {
		return out
	}
	kids, _ := ex.model.Children(d)
	for i, k := range kids {
		if !k.Attr {
			out[k.Name] = i
		}
	}
	return out
}

// decl assigns a data element its declaration.
func (ex *extractor) decl(n *dataxml.Node) *dataschema.Decl {
	if ex.model == nil {
		return nil
	}
	if d, ok := ex.decls[n]; ok {
		return d
	}
	var d *dataschema.Decl
	if n.Parent == nil {
		for _, r := range ex.model.Roots() {
			if r.Name == n.Name {
				d = r
			}
		}
	} else if pd := ex.decl(n.Parent); pd != nil {
		kids, _ := ex.model.Children(pd)
		for _, k := range kids {
			if !k.Attr && k.Name == n.Name {
				d = k
			}
		}
	}
	ex.decls[n] = d
	return d
}

// IsExtractError reports whether err reports invalid but extracted data.
func IsExtractError(err error) (*ExtractError, bool) {
	var e *ExtractError
	ok := errors.As(err, &e)
	return e, ok
}

// Flatten converts a filled-in form into a static PDF: it reads the input
// like ExtractData and renders the template again with it, with the fields
// as fixed content. Profile and options come from opts, without a profile
// the template's profile applies. Invalid data is not flattened.
func (t *Template) Flatten(pdf, base []byte, opts TemplateOptions) (*Result, error) {
	data, err := t.ExtractData(pdf, base)
	if err != nil {
		return nil, err
	}
	opts.Flatten = true
	return t.Render(data, opts)
}
