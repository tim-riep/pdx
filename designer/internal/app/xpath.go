package app

import (
	"errors"
	"strings"

	"github.com/tim-riep/pdx/designer/internal/model"
	"github.com/tim-riep/pdx/designer/internal/schema"
	"github.com/tim-riep/pdx/designer/internal/xpath"
)

// XPathQuery describes an expression on a template element. Cursor is
// the position in characters (UTF-16 as in the browser) for completion.
type XPathQuery struct {
	Ref    string `json:"ref"`
	Expr   string `json:"expr"`
	Cursor int    `json:"cursor"`
}

// XPathInfo is the result of checking an expression.
type XPathInfo struct {
	// Context is the data context of the element, empty without data root.
	Context string `json:"context"`
	// Data names the data of the evaluation: the path of the sample data or
	// empty for placeholders.
	Data     string          `json:"data"`
	Problems []xpath.Problem `json:"problems"`
	// Target is the data node of a simple path.
	Target *Field        `json:"target"`
	Result *xpath.Result `json:"result"`
}

// Field is a data node, relative to the context of an element.
type Field struct {
	Path     string `json:"path"` // relative XPath
	Type     string `json:"type"`
	Repeated bool   `json:"repeated"`
	Optional bool   `json:"optional"`
}

// xpathInput collects under the lock what check and completion
// need.
type xpathInput struct {
	sc     *schema.Schema
	ctx    string
	ns     map[string]string
	data   []byte
	sample string
}

func (s *TemplateService) xpathInput(ref string) (xpathInput, error) {
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	d := s.c.doc
	if d == nil {
		return xpathInput{}, errors.New("keine Vorlage geöffnet")
	}
	t := d.Pkg.Template
	n, err := model.Resolve(t, ref)
	if err != nil {
		return xpathInput{}, err
	}
	in := xpathInput{ns: map[string]string{}}
	in.sc, _ = s.c.schemaLocked()
	in.ctx, _ = schema.Context(t, n)
	if ns := t.Root.First("namespaces"); ns != nil {
		for _, x := range ns.Elements("ns") {
			in.ns[x.Get("prefix", "")] = x.Get("uri", "")
		}
	}
	if in.sc != nil {
		for uri, p := range in.sc.Missing {
			in.ns[p] = uri
		}
	}
	// Sample data as in the preview, otherwise placeholders.
	in.data, in.sample = s.c.previewDataLocked()
	return in, nil
}

// CheckXPath checks an expression: paths against the data XSD, syntax when
// compiling and value with sample data in the first occurrence of the context.
func (s *TemplateService) CheckXPath(q XPathQuery) (XPathInfo, error) {
	in, err := s.xpathInput(q.Ref)
	if err != nil {
		return XPathInfo{}, err
	}
	info := XPathInfo{Context: in.ctx, Data: in.sample, Problems: []xpath.Problem{}}
	if strings.TrimSpace(q.Expr) == "" || in.ctx == "" {
		return info, nil
	}
	probs, target := xpath.Paths(in.sc, in.ctx, q.Expr)
	for _, p := range probs {
		p.Pos, p.End = utf16Index(q.Expr, p.Pos), utf16Index(q.Expr, p.End)
		info.Problems = append(info.Problems, p)
	}
	if target != "" {
		if n := in.sc.Find(target); n != nil {
			info.Target = &Field{Path: q.Expr, Type: n.Type, Repeated: n.Repeated(), Optional: n.Min == 0}
		}
	}
	if in.data != nil {
		r := xpath.Eval(q.Expr, in.ctx, in.ns, in.data)
		info.Result = &r
	}
	return info, nil
}

// CompleteXPath returns suggestions for the position Cursor.
func (s *TemplateService) CompleteXPath(q XPathQuery) (xpath.Completion, error) {
	in, err := s.xpathInput(q.Ref)
	if err != nil {
		return xpath.Completion{}, err
	}
	c := xpath.Complete(in.sc, in.ctx, q.Expr, byteIndex(q.Expr, q.Cursor))
	c.From, c.To = utf16Index(q.Expr, c.From), utf16Index(q.Expr, c.To)
	return c, nil
}

// XPathFields lists the data nodes under the context of an element with
// relative path, for example for the selection in presence conditions.
func (s *TemplateService) XPathFields(ref string) ([]Field, error) {
	in, err := s.xpathInput(ref)
	if err != nil {
		return nil, err
	}
	out := []Field{}
	if in.sc == nil || in.ctx == "" {
		return out, nil
	}
	root := in.sc.Find(in.ctx)
	if root == nil {
		return out, nil
	}
	var walk func(n *schema.Node, depth int)
	walk = func(n *schema.Node, depth int) {
		for _, k := range n.Children {
			out = append(out, Field{Path: schema.Relative(in.ctx, k.Path), Type: k.Type, Repeated: k.Repeated(), Optional: k.Min == 0})
			// Not into repeated nodes: there the path is ambiguous.
			if depth < 3 && !k.Repeated() {
				walk(k, depth+1)
			}
		}
	}
	walk(root, 0)
	return out, nil
}

// utf16Index converts a byte position into a position in UTF-16 units,
// as JavaScript counts them.
func utf16Index(s string, b int) int {
	n := 0
	for i, r := range s {
		if i >= b {
			break
		}
		n++
		if r >= 0x10000 {
			n++
		}
	}
	return n
}

// byteIndex is the inverse of utf16Index.
func byteIndex(s string, u int) int {
	n := 0
	for i, r := range s {
		if n >= u {
			return i
		}
		n++
		if r >= 0x10000 {
			n++
		}
	}
	return len(s)
}

// RelativeBind returns the XPath to a data node (path in the data tree)
// relative to the context of an element, for example for a field that is
// dragged into a text.
func (s *TemplateService) RelativeBind(ref, path string) (string, error) {
	in, err := s.xpathInput(ref)
	if err != nil {
		return "", err
	}
	if in.ctx == "" {
		return "", errors.New("die Vorlage hat keine Datenwurzel")
	}
	return schema.Relative(in.ctx, path), nil
}
