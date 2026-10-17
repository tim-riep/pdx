// Package schema bereitet das Daten-XSD eines Vorlagenpakets für den Designer
// auf: Datenbaum mit Typen und Kardinalitäten, Präfixe der Vorlage, Kontext
// von Bindungen und Platzhalterdaten für die Entwurfsansicht.
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

// Node ist ein Knoten des Datenbaums.
type Node struct {
	// Path sind die Schritte von der Wurzel, etwa inv:invoice/inv:items.
	Path string `json:"path"`
	Name string `json:"name"` // mit Präfix, Attribute mit @
	// Type ist der eingebaute Basistyp (string, decimal, date ...), leer bei
	// Elementen ohne Textinhalt.
	Type     string   `json:"type"`
	Min      int      `json:"min"`
	Max      int      `json:"max"` // -1 für beliebig viele
	Attr     bool     `json:"attr"`
	Enum     []string `json:"enum,omitempty"`
	Doc      string   `json:"doc,omitempty"`
	Children []*Node  `json:"children"`
	// Recursive markiert einen Knoten, dessen Inhalt sich wiederholt (der
	// Baum endet dort).
	Recursive bool `json:"recursive,omitempty"`
	// Open meldet xs:any, dort ist der Inhalt unbekannt.
	Open bool `json:"open,omitempty"`

	decl *dataschema.Decl
}

// Repeated meldet einen wiederholten Knoten.
func (n *Node) Repeated() bool { return n.Max == -1 || n.Max > 1 }

// Schema ist das aufbereitete Daten-XSD.
type Schema struct {
	Main  string  `json:"main"`
	Roots []*Node `json:"roots"`
	// Prefixes ordnet Namespaces Präfixen zu, aus der Vorlage oder neu.
	Prefixes map[string]string `json:"prefixes"`
	// Missing sind Namespaces, die in der Vorlage noch nicht deklariert sind.
	Missing map[string]string `json:"missing"`

	model dataschema.Model
	index map[string]*Node
}

// maxDepth begrenzt den Baum bei sehr tiefen Schemas.
const maxDepth = 24

// Load liest das Daten-XSD eines Pakets. Ohne Schema liefert es nil.
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

// name liefert den Namen mit Präfix. Ein Namespace ohne Präfix in der
// Vorlage erhält einen neuen.
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
	// Attribute zuerst, dann Elemente in der Reihenfolge des Schemas.
	sort.SliceStable(kids, func(i, j int) bool { return kids[i].Attr && !kids[j].Attr })
	for _, k := range kids {
		n.Children = append(n.Children, s.build(k, path, append(seen, d.Name), depth+1))
	}
	return n
}

// Find liefert einen Knoten über seinen Pfad.
func (s *Schema) Find(path string) *Node { return s.index[path] }

// Context bestimmt den Datenkontext eines Vorlagenelements als Pfad: die
// Datenwurzel, erweitert um die Bindungen der umgebenden Teilformulare und
// Tabellen.
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
			// Der Tabellenkontext gilt in Zeilen, Kopf und Fuß beziehen sich
			// ebenfalls auf die einzelnen Positionen.
			ctx = join(ctx, bind)
		}
	}
	if ctx == "" {
		return "", errors.New("die Vorlage hat keine Datenwurzel (dataRoot)")
	}
	return ctx, nil
}

// join verbindet einen Kontext mit einem relativen Pfad und löst .. auf.
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

// Relative liefert den XPath von ctx zu target, mit .. für Schritte außerhalb
// des Kontexts.
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

// Join verbindet einen Kontext mit einem relativen Pfad und löst .. auf.
func Join(ctx, rel string) string { return join(ctx, rel) }

// Nodes liefert alle Knoten des Datenbaums in Dokumentreihenfolge.
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
