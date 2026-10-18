package model

import (
	"sort"
	"sync"

	"github.com/tim-riep/go-xslt/xsd"
	"github.com/tim-riep/pdx/template"
)

// Property ist ein Attribut eines Vorlagenelements, abgeleitet aus dem XSD
// des Vorlagenformats.
type Property struct {
	Name string `json:"name"`
	// Group ordnet die Eigenschaft im Inspektor ein: general, access
	// (Barrierefreiheit), position, text, border, spacing, breaks, binding
	// oder other.
	Group string `json:"group"`
	// Kind wählt den Editor: id, ref, length, lengths, color, font (Verweis
	// auf ein Schrift-Token), xpath, enum, bool, number, profile
	// (Konformitätsprofile) oder text.
	Kind     string   `json:"kind"`
	Enum     []string `json:"enum,omitempty"`
	Default  string   `json:"default,omitempty"`
	Required bool     `json:"required,omitempty"`
}

// Child ist ein erlaubtes Kindelement.
type Child struct {
	Name string `json:"name"`
	Key  string `json:"key"`
	Min  int    `json:"min"`
	Max  int    `json:"max"` // -1 für beliebig viele
}

// ElementType beschreibt ein Element des Vorlagenformats.
type ElementType struct {
	Key        string     `json:"key"`
	Name       string     `json:"name"`
	Properties []Property `json:"properties"`
	Children   []Child    `json:"children"`
	Mixed      bool       `json:"mixed"`
	// Text meldet Textinhalt: gemischt oder nur Text (etwa string).
	Text bool `json:"text"`
}

// Catalog sind alle Elementtypen. Schlüssel ist bei globalen Elementen der
// Name, bei lokalen "eltern/name" (etwa table/header).
type Catalog map[string]*ElementType

var (
	catalogOnce sync.Once
	catalog     Catalog
	catalogErr  error
)

// LoadCatalog liest den Katalog einmal aus template.Schema.
func LoadCatalog() (Catalog, error) {
	catalogOnce.Do(func() { catalog, catalogErr = buildCatalog() })
	return catalog, catalogErr
}

// TypeOf liefert den Elementtyp eines Knotens.
func (c Catalog) TypeOf(n *template.Node) *ElementType {
	if n.Parent != nil {
		if t, ok := c[n.Parent.Name+"/"+n.Name]; ok {
			return t
		}
	}
	return c[n.Name]
}

// Property liefert eine Eigenschaft nach Namen.
func (t *ElementType) Property(name string) (Property, bool) {
	for _, p := range t.Properties {
		if p.Name == name {
			return p, true
		}
	}
	return Property{}, false
}

func buildCatalog() (Catalog, error) {
	s, diags := xsd.Compile(xsd.Request{Schemas: []string{template.Schema}})
	if s == nil {
		return nil, diagError(diags)
	}
	global := map[string]*xsd.Element{}
	for _, e := range s.GlobalElements() {
		global[e.Name().Local] = e
	}
	cat := Catalog{}
	// Lokale Elemente mit einem Namen, den es mehrfach gibt, erhalten den
	// Elternnamen als Präfix.
	var visit func(e *xsd.Element, key string)
	visit = func(e *xsd.Element, key string) {
		if _, done := cat[key]; done {
			return
		}
		et := &ElementType{Key: key, Name: e.Name().Local, Properties: []Property{}, Children: []Child{}}
		cat[key] = et
		ct, ok := e.Type().(*xsd.ComplexType)
		if !ok {
			return
		}
		et.Mixed = ct.Mixed()
		et.Text = et.Mixed || ct.ContentKind() == xsd.ContentSimple
		for _, a := range ct.Attributes() {
			if a.Use == xsd.UseProhibited {
				continue
			}
			et.Properties = append(et.Properties, property(a))
		}
		sort.SliceStable(et.Properties, func(i, j int) bool { return groupOrder[et.Properties[i].Group] < groupOrder[et.Properties[j].Group] })
		seen := map[string]int{} // Schlüssel zu Index in et.Children
		var walk func(p *xsd.Particle, lo, hi int)
		walk = func(p *xsd.Particle, lo, hi int) {
			lo, hi = lo*p.MinOccurs, mulMax(hi, p.MaxOccurs)
			switch t := p.Term.(type) {
			case *xsd.Element:
				name := t.Name().Local
				ck := name
				if g, ok := global[name]; !ok || g != t {
					ck = et.Name + "/" + name
				}
				if i, ok := seen[ck]; ok {
					et.Children[i].Min += lo
					et.Children[i].Max = addMax(et.Children[i].Max, hi)
				} else {
					seen[ck] = len(et.Children)
					et.Children = append(et.Children, Child{Name: name, Key: ck, Min: lo, Max: hi})
				}
				visit(t, ck)
			case *xsd.ModelGroup:
				for _, q := range t.Particles {
					if t.Compositor == xsd.CompositorChoice && len(t.Particles) > 1 {
						walk(q, 0, hi)
					} else {
						walk(q, lo, hi)
					}
				}
			}
		}
		for _, p := range ct.ContentParticles() {
			walk(p, 1, 1)
		}
	}
	visit(global["template"], "template")
	for name, e := range global {
		visit(e, name)
	}
	return cat, nil
}

// property leitet Editor und Gruppe einer Eigenschaft ab.
func property(a *xsd.AttributeUse) Property {
	p := Property{Name: a.Name.Local, Group: groupOf(a.Name.Local), Required: a.Use == xsd.UseRequired}
	if a.HasDefault {
		p.Default = a.Default
	}
	p.Kind = "text"
	if a.Type != nil {
		p.Enum = a.Type.Enumeration()
		switch a.Type.Name().Local {
		case "Length", "ColumnWidth":
			p.Kind = "length"
		case "Lengths":
			p.Kind = "lengths"
		case "Color":
			p.Kind = "color"
		case "XPath":
			p.Kind = "xpath"
		case "TokenRef":
			p.Kind = "font"
		case "ProfileList":
			p.Kind = "profile"
		default:
			switch a.Type.BuiltinBase().Local {
			case "ID":
				p.Kind = "id"
			case "IDREF":
				p.Kind = "ref"
			case "boolean":
				p.Kind = "bool"
			case "integer", "positiveInteger", "nonNegativeInteger", "decimal", "double", "int":
				p.Kind = "number"
			}
		}
		if len(p.Enum) > 0 {
			p.Kind = "enum"
		}
	}
	return p
}

var groupOrder = map[string]int{"general": 0, "binding": 1, "access": 2, "position": 3, "text": 4, "spacing": 5, "border": 6, "breaks": 7, "other": 8}

var groups = map[string]string{
	"id": "general", "style": "general", "presence": "general", "title": "general",
	"parent": "general", "use": "general", "type": "general", "name": "general",
	"profile": "general", "afRelationship": "general",
	"role": "access", "alt": "access", "altBind": "access", "lang": "access", "readingOrder": "access",
	"artifact": "access", "scope": "access",
	"bind": "binding", "format": "binding", "default": "binding", "repeat": "binding", "by": "binding",
	"of": "binding", "value": "binding", "ref": "binding", "src": "binding", "source": "binding",
	"dataRoot": "binding", "schema": "binding", "pattern": "binding", "href": "binding",
	"x": "position", "y": "position", "w": "position", "h": "position", "anchor": "position",
	"layout": "position", "width": "position", "height": "position", "margin": "position",
	"size": "position", "orientation": "position", "gap": "position", "colspan": "position",
	"rowspan": "position", "verticalAlign": "position", "moduleWidth": "position", "barHeight": "position",
	"font": "text", "family": "text", "fontFamily": "text", "fontSize": "text", "fontWeight": "text", "fontStyle": "text", "color": "text",
	"textAlign": "text", "lineHeight": "text", "hyphenate": "text",
	"spaceBefore": "spacing", "spaceAfter": "spacing", "padding": "spacing",
	"backgroundColor": "border", "border": "border", "borderTop": "border", "borderBottom": "border",
	"borderLeft": "border", "borderRight": "border", "borders": "border", "fill": "border",
	"thickness": "border", "lineStyle": "border",
	"breakBefore": "breaks", "breakAfter": "breaks", "keepTogether": "breaks", "keepWithNext": "breaks",
	"keepWithPrevious": "breaks",
}

func groupOf(name string) string {
	if g, ok := groups[name]; ok {
		return g
	}
	return "other"
}

func mulMax(a, b int) int {
	if a == -1 || b == -1 {
		if a == 0 || b == 0 {
			return 0
		}
		return -1
	}
	return a * b
}

func addMax(a, b int) int {
	if a == -1 || b == -1 {
		return -1
	}
	return a + b
}

type diagErr []xsd.Diagnostic

func (d diagErr) Error() string {
	if len(d) == 0 {
		return "XSD des Vorlagenformats nicht lesbar"
	}
	return "XSD des Vorlagenformats: " + d[0].Message
}

func diagError(d []xsd.Diagnostic) error { return diagErr(d) }
