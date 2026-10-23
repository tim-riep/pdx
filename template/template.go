// Package template reads, validates and writes the pdx template format 1
// (template.xml). The format is described by schema/template-v1.xsd and is
// validated with the XSD validator of go-xslt.
//
// The template is held as an ordered node tree so that the designer can edit
// it losslessly and Write can write it back canonically: fixed indentation,
// id as the first attribute, the others in alphabetical order, mixed content
// unchanged. This keeps Git diffs readable.
package template

import (
	"bytes"
	_ "embed"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"

	"github.com/tim-riep/go-xslt/engine"
)

// NS is the namespace of template format 1.
const NS = "https://github.com/tim-riep/pdx/ns/template/1"

// LegacyNS is the namespace of format 0 up to pdx 0.1.0. Format 1 has the
// same content, the parser reads old templates and lifts them to format 1
// (namespace and version), pdx fmt rewrites them.
const LegacyNS = "https://pdx.example/ns/template/0"

// Version is the value of template/@version in format 1.
const Version = "1"

// Schema is the XSD of template format 1.
//
//go:embed schema/template-v1.xsd
var Schema string

// Kind distinguishes elements and text.
type Kind uint8

const (
	// Element is an element node.
	Element Kind = iota
	// Text is a text node (only in mixed content).
	Text
)

// Attr is an attribute.
type Attr struct {
	Name, Value string
}

// Node is a node of the template.
type Node struct {
	Kind     Kind
	Name     string // local name, the namespace is always NS
	Attrs    []Attr
	Children []*Node
	Parent   *Node
	Data     string // content of a text node
	Line     int
	Col      int
}

// Attr returns an attribute.
func (n *Node) Attr(name string) (string, bool) {
	for _, a := range n.Attrs {
		if a.Name == name {
			return a.Value, true
		}
	}
	return "", false
}

// Get returns an attribute or def.
func (n *Node) Get(name, def string) string {
	if v, ok := n.Attr(name); ok {
		return v
	}
	return def
}

// SetAttr sets or replaces an attribute.
func (n *Node) SetAttr(name, value string) {
	for i := range n.Attrs {
		if n.Attrs[i].Name == name {
			n.Attrs[i].Value = value
			return
		}
	}
	n.Attrs = append(n.Attrs, Attr{name, value})
}

// RemoveAttr removes an attribute and returns its previous value.
func (n *Node) RemoveAttr(name string) (string, bool) {
	for i, a := range n.Attrs {
		if a.Name == name {
			n.Attrs = append(n.Attrs[:i:i], n.Attrs[i+1:]...)
			return a.Value, true
		}
	}
	return "", false
}

// NewElement creates an element without a parent.
func NewElement(name string, attrs ...Attr) *Node {
	return &Node{Kind: Element, Name: name, Attrs: attrs}
}

// NewText creates a text node.
func NewText(data string) *Node { return &Node{Kind: Text, Data: data} }

// InsertChild inserts c at position i under n. An i outside the children
// appends c. c must not have a parent node.
func (n *Node) InsertChild(i int, c *Node) {
	if i < 0 || i > len(n.Children) {
		i = len(n.Children)
	}
	n.Children = slices.Insert(n.Children, i, c)
	c.Parent = n
}

// RemoveChild detaches c from n and returns its previous position, -1 if c
// is not a child of n.
func (n *Node) RemoveChild(c *Node) int {
	i := slices.Index(n.Children, c)
	if i < 0 {
		return -1
	}
	n.Children = slices.Delete(n.Children, i, i+1)
	c.Parent = nil
	return i
}

// Index returns the position under the parent node, -1 without a parent.
func (n *Node) Index() int {
	if n.Parent == nil {
		return -1
	}
	return slices.Index(n.Parent.Children, n)
}

// Clone copies the node with all descendants, without the parent node.
func (n *Node) Clone() *Node {
	c := &Node{Kind: n.Kind, Name: n.Name, Attrs: slices.Clone(n.Attrs), Data: n.Data, Line: n.Line, Col: n.Col}
	for _, ch := range n.Children {
		cc := ch.Clone()
		cc.Parent = c
		c.Children = append(c.Children, cc)
	}
	return c
}

// ID returns the ID of the element.
func (n *Node) ID() string { return n.Get("id", "") }

// Elements returns the element children, optionally only those with a name.
func (n *Node) Elements(name string) []*Node {
	var out []*Node
	for _, c := range n.Children {
		if c.Kind == Element && (name == "" || c.Name == name) {
			out = append(out, c)
		}
	}
	return out
}

// First returns the first element child with the name, or nil.
func (n *Node) First(name string) *Node {
	for _, c := range n.Children {
		if c.Kind == Element && c.Name == name {
			return c
		}
	}
	return nil
}

// NearestID returns the ID of the element or of the nearest ancestor with an ID.
func (n *Node) NearestID() string {
	for x := n; x != nil; x = x.Parent {
		if id := x.ID(); id != "" {
			return id
		}
	}
	return ""
}

// Template is a loaded template.
type Template struct {
	Root *Node
	ids  map[string]*Node
}

// ByID returns the element with an ID.
func (t *Template) ByID(id string) *Node { return t.ids[id] }

// Reindex rebuilds the index of IDs. Required after changes to the node
// tree, before ByID or Validate are used.
func (t *Template) Reindex() { t.index() }

// Error is an error in a template with the template element ID, if it can
// be assigned, and the position.
type Error struct {
	ID        string
	Line, Col int
	Msg       string
}

func (e *Error) Error() string {
	var b strings.Builder
	if e.ID != "" {
		fmt.Fprintf(&b, "[%s] ", e.ID)
	}
	if e.Line > 0 {
		fmt.Fprintf(&b, "%d:%d: ", e.Line, e.Col)
	}
	b.WriteString(e.Msg)
	return b.String()
}

// Errors collects several errors.
type Errors []*Error

func (es Errors) Error() string {
	parts := make([]string, len(es))
	for i, e := range es {
		parts[i] = e.Error()
	}
	return strings.Join(parts, "\n")
}

// mixed are elements with mixed content whose text is preserved.
var mixed = map[string]bool{"text": true, "span": true, "string": true}

// Parse reads a template and validates it against the XSD.
func Parse(data []byte) (*Template, error) {
	t, err := parseTree(data)
	if err != nil {
		return nil, err
	}
	if err := t.Validate(); err != nil {
		return nil, err
	}
	return t, nil
}

// ParseTree reads a well-formed template without validation against the
// XSD, such as an intermediate state in the designer. Validate performs the
// validation later.
func ParseTree(data []byte) (*Template, error) { return parseTree(data) }

// parseTree reads the node tree without schema validation.
func parseTree(data []byte) (*Template, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = true
	var root *Node
	var stack []*Node
	count := 0
	legacy := false
	for {
		line, col := dec.InputPos()
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, &Error{Line: line, Col: col, Msg: "XML nicht wohlgeformt: " + err.Error()}
		}
		count++
		if count > 2_000_000 {
			return nil, &Error{Line: line, Col: col, Msg: "Vorlage hat zu viele Knoten"}
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Space == LegacyNS {
				legacy = true
			} else if t.Name.Space != NS {
				return nil, &Error{Line: line, Col: col, Msg: fmt.Sprintf("Element %s gehört nicht zum Namespace %s", t.Name.Local, NS)}
			}
			if len(stack) >= 128 {
				return nil, &Error{Line: line, Col: col, Msg: "Vorlage ist zu tief verschachtelt"}
			}
			n := &Node{Kind: Element, Name: t.Name.Local, Line: line, Col: col}
			for _, a := range t.Attr {
				if a.Name.Space == "xmlns" || a.Name.Space == "" && a.Name.Local == "xmlns" {
					continue
				}
				if a.Name.Space != "" {
					return nil, &Error{Line: line, Col: col, Msg: fmt.Sprintf("Attribut %s:%s ist nicht erlaubt", a.Name.Space, a.Name.Local)}
				}
				n.Attrs = append(n.Attrs, Attr{a.Name.Local, a.Value})
			}
			if len(stack) == 0 {
				if root != nil {
					return nil, &Error{Line: line, Col: col, Msg: "mehr als ein Wurzelelement"}
				}
				root = n
			} else {
				p := stack[len(stack)-1]
				n.Parent = p
				p.Children = append(p.Children, n)
			}
			stack = append(stack, n)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) == 0 {
				continue
			}
			p := stack[len(stack)-1]
			s := string(t)
			if !mixed[p.Name] {
				if strings.TrimSpace(s) != "" {
					return nil, &Error{ID: p.NearestID(), Line: line, Col: col, Msg: fmt.Sprintf("%s darf keinen Text enthalten", p.Name)}
				}
				continue
			}
			if k := len(p.Children); k > 0 && p.Children[k-1].Kind == Text {
				p.Children[k-1].Data += s
				continue
			}
			p.Children = append(p.Children, &Node{Kind: Text, Data: s, Parent: p, Line: line, Col: col})
		}
	}
	if root == nil {
		return nil, &Error{Line: 1, Col: 1, Msg: "Vorlage ist leer"}
	}
	if root.Name != "template" {
		return nil, &Error{Line: root.Line, Col: root.Col, Msg: "Wurzelelement muss template sein"}
	}
	// Format 0 (up to pdx 0.1.0) has the same content, only the namespace
	// and version change.
	if legacy && root.Get("version", "") == "0" {
		root.SetAttr("version", Version)
	}
	t := &Template{Root: root}
	t.index()
	return t, nil
}

func (t *Template) index() {
	t.ids = map[string]*Node{}
	var walk func(n *Node)
	walk = func(n *Node) {
		if n.Kind != Element {
			return
		}
		if id := n.ID(); id != "" {
			if _, dup := t.ids[id]; !dup {
				t.ids[id] = n
			}
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(t.Root)
}

// nodeAt returns the element that starts at a position in the source or is
// the last one to start before it.
func (t *Template) nodeAt(line, col int) *Node {
	var best *Node
	var walk func(n *Node)
	walk = func(n *Node) {
		if n.Kind != Element {
			return
		}
		if n.Line < line || n.Line == line && n.Col <= col {
			best = n
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(t.Root)
	return best
}

// Validate validates the template against the XSD. For this the template is
// written canonically, error positions refer to this version and are
// assigned to the element with an ID.
func (t *Template) Validate() error {
	canon := Write(t)
	res := engine.Validate(engine.ValidateRequest{Schemas: []string{Schema}, Instance: string(canon)})
	if res.Valid {
		return nil
	}
	ct, err := parseTree(canon)
	if err != nil {
		return err
	}
	var errs Errors
	for _, d := range res.Diagnostics {
		if d.Severity != engine.SeverityError {
			continue
		}
		e := &Error{Msg: "Schema: " + d.Message}
		if n := ct.nodeAt(d.Line, d.Col); n != nil {
			e.ID = n.NearestID()
			if orig := t.ids[e.ID]; orig != nil {
				e.Line, e.Col = orig.Line, orig.Col
			}
		}
		errs = append(errs, e)
	}
	if len(errs) == 0 {
		errs = append(errs, &Error{Msg: "Vorlage entspricht nicht dem Schema"})
	}
	return errs
}

// Write writes the template canonically.
func Write(t *Template) []byte {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	writeNode(&b, t.Root, 0, true)
	b.WriteByte('\n')
	return b.Bytes()
}

func sortedAttrs(n *Node) []Attr {
	as := append([]Attr(nil), n.Attrs...)
	sort.SliceStable(as, func(i, j int) bool {
		if as[i].Name == "id" || as[j].Name == "id" {
			return as[i].Name == "id" && as[j].Name != "id"
		}
		return as[i].Name < as[j].Name
	})
	return as
}

func writeNode(b *bytes.Buffer, n *Node, depth int, root bool) {
	if n.Kind == Text {
		xml.EscapeText(b, []byte(n.Data))
		return
	}
	b.WriteByte('<')
	b.WriteString(n.Name)
	if root {
		b.WriteString(` xmlns="` + NS + `"`)
	}
	for _, a := range sortedAttrs(n) {
		b.WriteByte(' ')
		b.WriteString(a.Name)
		b.WriteString(`="`)
		escapeAttr(b, a.Value)
		b.WriteByte('"')
	}
	if len(n.Children) == 0 {
		b.WriteString("/>")
		return
	}
	b.WriteByte('>')
	if mixed[n.Name] {
		for _, c := range n.Children {
			writeNode(b, c, depth+1, false)
		}
	} else {
		for _, c := range n.Children {
			b.WriteByte('\n')
			b.WriteString(strings.Repeat("  ", depth+1))
			writeNode(b, c, depth+1, false)
		}
		b.WriteByte('\n')
		b.WriteString(strings.Repeat("  ", depth))
	}
	b.WriteString("</")
	b.WriteString(n.Name)
	b.WriteByte('>')
}

func escapeAttr(b *bytes.Buffer, s string) {
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\n':
			b.WriteString("&#10;")
		case '\t':
			b.WriteString("&#9;")
		case '\r':
			b.WriteString("&#13;")
		default:
			b.WriteRune(r)
		}
	}
}
