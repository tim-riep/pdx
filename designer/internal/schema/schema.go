// Package schema prepares the data XSD of a template package for the designer:
// data tree with types and cardinalities, prefixes of the template, context
// of bindings and placeholder data for the design view.
package schema

import (
	"encoding/xml"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/tim-riep/pdx/dataschema"
	"github.com/tim-riep/pdx/template"
	"github.com/tim-riep/pdx/template/pkg"
)

// Node is a node of the data tree.
type Node struct {
	// Path is the steps from the root, for example inv:invoice/inv:items.
	Path string `json:"path"`
	Name string `json:"name"` // with prefix, attributes with @
	// Type is the built-in base type (string, decimal, date ...), empty for
	// elements without text content.
	Type     string   `json:"type"`
	Min      int      `json:"min"`
	Max      int      `json:"max"` // -1 for any number
	Attr     bool     `json:"attr"`
	Enum     []string `json:"enum,omitempty"`
	Doc      string   `json:"doc,omitempty"`
	Children []*Node  `json:"children"`
	// Recursive marks a node whose content repeats itself (the
	// tree ends there).
	Recursive bool `json:"recursive,omitempty"`
	// Open reports xs:any, where the content is unknown.
	Open bool `json:"open,omitempty"`

	decl *dataschema.Decl
}

// Repeated reports a repeated node.
func (n *Node) Repeated() bool { return n.Max == -1 || n.Max > 1 }

// Schema is the prepared data XSD.
type Schema struct {
	Main  string  `json:"main"`
	Roots []*Node `json:"roots"`
	// Prefixes maps namespaces to prefixes, from the template or new.
	Prefixes map[string]string `json:"prefixes"`
	// Missing are namespaces that are not declared in the template yet.
	Missing map[string]string `json:"missing"`

	model dataschema.Model
	index map[string]*Node
}

// maxDepth limits the tree for very deep schemas.
const maxDepth = 24

// Load reads the data XSD of a package. Without a schema it returns nil.
func Load(p *pkg.Package) (*Schema, error) {
	main, files := p.Schemas()
	if main == "" {
		return nil, nil
	}
	m, err := dataschema.FromXSD(main, files)
	if err != nil {
		return nil, err
	}
	s := &Schema{Main: main, Prefixes: map[string]string{}, Missing: map[string]string{}, model: m, index: map[string]*Node{}}
	if ns := p.Template.Root.First("namespaces"); ns != nil {
		for _, n := range ns.Elements("ns") {
			uri, prefix := n.Get("uri", ""), n.Get("prefix", "")
			if _, ok := s.Prefixes[uri]; !ok && prefix != "" {
				s.Prefixes[uri] = prefix
			}
		}
	}
	root := p.Template.Root.Get("dataRoot", "")
	for _, d := range m.Roots() {
		if root != "" && s.name(d) != root {
			continue
		}
		s.Roots = append(s.Roots, s.build(d, "", nil, 0))
	}
	if len(s.Roots) == 0 && root != "" {
		return nil, fmt.Errorf("Datenwurzel %s nicht im Daten-XSD %s", root, main)
	}
	return s, nil
}

// name returns the name with prefix. A namespace without a prefix in the
// template gets a new one.
func (s *Schema) name(d *dataschema.Decl) string {
	local := d.Name.Local
	if d.Attr {
		local = "@" + local
	}
	if d.Name.Space == "" {
		return local
	}
	p, ok := s.Prefixes[d.Name.Space]
	if !ok {
		p = fmt.Sprintf("ns%d", len(s.Missing)+1)
		s.Prefixes[d.Name.Space] = p
		s.Missing[d.Name.Space] = p
	}
	if d.Attr {
		return "@" + p + ":" + d.Name.Local
	}
	return p + ":" + local
}

func (s *Schema) build(d *dataschema.Decl, parent string, seen []xml.Name, depth int) *Node {
	name := s.name(d)
	path := name
	if parent != "" {
		path = parent + "/" + name
	}
	n := &Node{Path: path, Name: name, Type: d.Type, Min: d.Min, Max: d.Max, Attr: d.Attr, Enum: d.Enum, Children: []*Node{}, decl: d}
	if !d.Simple && d.Type == "" {
		n.Type = ""
	}
	s.index[path] = n
	if d.Attr {
		return n
	}
	for _, x := range seen {
		if x == d.Name {
			n.Recursive = true
			return n
		}
	}
	if depth >= maxDepth {
		n.Recursive = true
		return n
	}
	kids, open := s.model.Children(d)
	n.Open = open
	// Attributes first, then elements in the order of the schema.
	sort.SliceStable(kids, func(i, j int) bool { return kids[i].Attr && !kids[j].Attr })
	for _, k := range kids {
		n.Children = append(n.Children, s.build(k, path, append(seen, d.Name), depth+1))
	}
	return n
}

// Find returns a node by its path.
func (s *Schema) Find(path string) *Node { return s.index[path] }

// Context determines the data context of a template element as a path: the
// data root, extended by the bindings of the enclosing subforms and
// tables.
func Context(t *template.Template, n *template.Node) (string, error) {
	var chain []*template.Node
	for x := n; x != nil; x = x.Parent {
		chain = append(chain, x)
	}
	ctx := t.Root.Get("dataRoot", "")
	for i := len(chain) - 1; i >= 1; i-- {
		x := chain[i]
		bind := x.Get("bind", "")
		if bind == "" {
			continue
		}
		switch {
		case x.Name == "subform":
			ctx = join(ctx, bind)
		case x.Name == "table":
			// The table context applies in rows, header and footer also refer
			// to the individual items.
			ctx = join(ctx, bind)
		}
	}
	if ctx == "" {
		return "", errors.New("die Vorlage hat keine Datenwurzel (dataRoot)")
	}
	return ctx, nil
}

// join joins a context with a relative path and resolves .. steps.
func join(ctx, rel string) string {
	if strings.HasPrefix(rel, "/") {
		return strings.TrimPrefix(rel, "/")
	}
	steps := strings.Split(ctx, "/")
	for _, p := range strings.Split(rel, "/") {
		switch p {
		case "", ".":
		case "..":
			if len(steps) > 0 {
				steps = steps[:len(steps)-1]
			}
		default:
			steps = append(steps, p)
		}
	}
	return strings.Join(steps, "/")
}

// Relative returns the XPath from ctx to target, with .. for steps outside
// the context.
func Relative(ctx, target string) string {
	a, b := strings.Split(ctx, "/"), strings.Split(target, "/")
	i := 0
	for i < len(a) && i < len(b) && a[i] == b[i] {
		i++
	}
	var out []string
	for range a[i:] {
		out = append(out, "..")
	}
	out = append(out, b[i:]...)
	if len(out) == 0 {
		return "."
	}
	return strings.Join(out, "/")
}

// Join joins a context with a relative path and resolves .. steps.
func Join(ctx, rel string) string { return join(ctx, rel) }

// Nodes returns all nodes of the data tree in document order.
func (s *Schema) Nodes() []*Node {
	var out []*Node
	var walk func(n *Node)
	walk = func(n *Node) {
		out = append(out, n)
		for _, c := range n.Children {
			walk(c)
		}
	}
	for _, r := range s.Roots {
		walk(r)
	}
	return out
}
