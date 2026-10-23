// Package dataschema checks the bindings of a template statically against
// the data schema: path present, type matching the format, cardinality
// matching the element. Findings carry the ID of the template element.
//
// The data schema is hidden behind the interface Model. FromXSD builds it
// from XSD files with the schema component model of go-xslt.
package dataschema

import (
	"encoding/xml"
	"fmt"
	"sort"
	"strings"

	"github.com/tim-riep/go-xslt/xsd"
)

// Unbounded is Max for maxOccurs="unbounded".
const Unbounded = -1

// Decl is an element or attribute in the data schema.
type Decl struct {
	Name xml.Name
	Attr bool
	// Type is the local name of the built-in base type, such as decimal, date
	// or string. Empty if the element has no simple content or the type is
	// not unique (union).
	Type string
	// Simple is set if the element contains text (simple or mixed content).
	// Attributes are always simple.
	Simple bool
	// Min and Max are the cardinality relative to the parent element.
	Min, Max int
	// Enum are the allowed values of a simple type with an enumeration.
	Enum []string
	// MaxLength is the effective facet maxLength (or length), 0 if absent.
	MaxLength int

	impl any
}

// Model describes the structure of the data.
type Model interface {
	// Roots returns the global elements.
	Roots() []*Decl
	// Children returns child elements and attributes of an element. open
	// reports wildcards (xs:any), where the content is not known.
	Children(d *Decl) (kids []*Decl, open bool)
}

// FromXSD compiles the data schema. main is the path of the main XSD in
// files, the other XSD files are available for xs:import and xs:include.
func FromXSD(main string, files map[string][]byte) (Model, error) {
	src, ok := files[main]
	if !ok {
		return nil, fmt.Errorf("Datenschema %s fehlt", main)
	}
	docs := []string{string(src)}
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
	s, diags := xsd.Compile(xsd.Request{Schemas: docs})
	if s == nil {
		var msgs []string
		for _, d := range diags {
			msgs = append(msgs, d.Message)
		}
		return nil, fmt.Errorf("Datenschema %s: %s", main, strings.Join(msgs, ", "))
	}
	return &xsdModel{s: s}, nil
}

type xsdModel struct {
	s *xsd.Schema
}

func (m *xsdModel) Roots() []*Decl {
	var out []*Decl
	for _, el := range m.s.GlobalElements() {
		if el.Abstract() {
			continue
		}
		out = append(out, m.decl(el, 1, 1))
	}
	return out
}

func (m *xsdModel) decl(el *xsd.Element, lo, hi int) *Decl {
	d := &Decl{Name: el.Name(), Min: lo, Max: hi, impl: el}
	d.Type, d.Simple = typeInfo(el.Type())
	d.Enum = enumOf(el.Type())
	d.MaxLength = maxLengthOf(simpleOf(el.Type()))
	return d
}

// typeInfo determines the base type and whether text is allowed.
func typeInfo(t xsd.TypeDefinition) (string, bool) {
	switch t := t.(type) {
	case *xsd.SimpleType:
		return simpleBase(t), true
	case *xsd.ComplexType:
		switch t.ContentKind() {
		case xsd.ContentSimple:
			if st := t.SimpleContent(); st != nil {
				return simpleBase(st), true
			}
			return "", true
		case xsd.ContentMixed:
			return "string", true
		}
	}
	return "", false
}

// simpleOf returns the simple type of the content, nil without simple content.
func simpleOf(t xsd.TypeDefinition) *xsd.SimpleType {
	switch t := t.(type) {
	case *xsd.SimpleType:
		return t
	case *xsd.ComplexType:
		if t.ContentKind() == xsd.ContentSimple {
			return t.SimpleContent()
		}
	}
	return nil
}

// enumOf returns the enumeration of the simple content, if present.
func enumOf(t xsd.TypeDefinition) []string {
	if st := simpleOf(t); st != nil {
		return st.Enumeration()
	}
	return nil
}

// maxLengthOf returns the effective maximum length of a simple type from the
// facets maxLength or length (XSD 1.1 Part 2 4.3.1, 4.3.3), 0 if absent.
func maxLengthOf(st *xsd.SimpleType) int {
	if st == nil {
		return 0
	}
	f := st.Facets()
	switch {
	case f.Length != nil:
		return *f.Length
	case f.MaxLength != nil:
		return *f.MaxLength
	}
	return 0
}

func simpleBase(t *xsd.SimpleType) string {
	if t == nil || t.Variety() == xsd.VarietyUnion {
		return ""
	}
	if t.Variety() == xsd.VarietyList {
		return "list"
	}
	return t.BuiltinBase().Local
}

func (m *xsdModel) Children(d *Decl) ([]*Decl, bool) {
	el, ok := d.impl.(*xsd.Element)
	if !ok {
		return nil, false
	}
	ct, ok := el.Type().(*xsd.ComplexType)
	if !ok {
		return nil, false
	}
	var out []*Decl
	index := map[xml.Name]*Decl{}
	open := false
	add := func(e *xsd.Element, lo, hi int) {
		if old, ok := index[e.Name()]; ok {
			// Same name at several places in the content model.
			old.Min += lo
			old.Max = addMax(old.Max, hi)
			return
		}
		nd := m.decl(e, lo, hi)
		index[e.Name()] = nd
		out = append(out, nd)
	}
	var walk func(p *xsd.Particle, lo, hi int)
	walk = func(p *xsd.Particle, lo, hi int) {
		lo, hi = lo*p.MinOccurs, mulMax(hi, p.MaxOccurs)
		switch t := p.Term.(type) {
		case *xsd.Element:
			if !t.Abstract() {
				add(t, lo, hi)
			}
			for _, sub := range m.s.SubstitutionMembers(t.Name()) {
				if !sub.Abstract() {
					// Members of a substitution group are optional.
					add(sub, 0, hi)
				}
			}
		case *xsd.ModelGroup:
			for _, q := range t.Particles {
				if t.Compositor == xsd.CompositorChoice && len(t.Particles) > 1 {
					walk(q, 0, hi)
				} else {
					walk(q, lo, hi)
				}
			}
		case *xsd.Wildcard:
			open = true
		}
	}
	for _, p := range ct.ContentParticles() {
		walk(p, 1, 1)
	}
	for _, a := range ct.Attributes() {
		if a.Use == xsd.UseProhibited {
			continue
		}
		lo := 0
		if a.Use == xsd.UseRequired {
			lo = 1
		}
		var enum []string
		if a.Type != nil {
			enum = a.Type.Enumeration()
		}
		out = append(out, &Decl{Name: a.Name, Attr: true, Type: simpleBase(a.Type), Simple: true, Min: lo, Max: 1, Enum: enum, MaxLength: maxLengthOf(a.Type)})
	}
	if ct.AttributeWildcard() != nil {
		open = true
	}
	return out, open
}

func mulMax(a, b int) int {
	if a == Unbounded || b == Unbounded {
		if a == 0 || b == 0 {
			return 0
		}
		return Unbounded
	}
	return a * b
}

func addMax(a, b int) int {
	if a == Unbounded || b == Unbounded {
		return Unbounded
	}
	return a + b
}
