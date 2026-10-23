// Package dataxml is a small XML tree model for data. It reads a document,
// allows setting values and inserting elements and writes it back with the
// original prefixes. pdx extract-data uses it to write form input back
// into the data.
package dataxml

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Kind is the kind of a node.
type Kind int

const (
	Element Kind = iota
	Text
	Comment
	ProcInst
)

// Node is a node. For elements, Name is the resolved name, Attrs also
// contain the namespace declarations (Name.Space "xmlns" or Local
// "xmlns" without Space).
type Node struct {
	Kind     Kind
	Name     xml.Name
	Attrs    []xml.Attr
	Children []*Node
	Parent   *Node
	Data     string // text, comment or content of a processing instruction
	Target   string // target of a processing instruction
}

// Document is a document with exactly one root element.
type Document struct {
	Root *Node
	// Before holds comments and processing instructions before the root
	// (without the XML declaration).
	Before []*Node
}

// Parse reads a document.
func Parse(data []byte) (*Document, error) {
	d := xml.NewDecoder(bytes.NewReader(data))
	doc := &Document{}
	var stack []*Node
	for {
		tok, err := d.RawToken()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("Daten sind kein wohlgeformtes XML: %v", err)
		}
		add := func(n *Node) {
			if len(stack) == 0 {
				if n.Kind == Element {
					doc.Root = n
				} else if n.Kind != Text {
					doc.Before = append(doc.Before, n)
				}
				return
			}
			p := stack[len(stack)-1]
			n.Parent = p
			p.Children = append(p.Children, n)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := &Node{Kind: Element, Name: t.Name, Attrs: append([]xml.Attr(nil), t.Attr...)}
			if len(stack) == 0 && doc.Root != nil {
				return nil, fmt.Errorf("Daten haben mehr als ein Wurzelelement")
			}
			add(n)
			stack = append(stack, n)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) > 0 {
				add(&Node{Kind: Text, Data: string(t)})
			}
		case xml.Comment:
			add(&Node{Kind: Comment, Data: string(t)})
		case xml.ProcInst:
			if t.Target != "xml" {
				add(&Node{Kind: ProcInst, Target: t.Target, Data: string(t.Inst)})
			}
		}
	}
	if doc.Root == nil {
		return nil, fmt.Errorf("Daten ohne Wurzelelement")
	}
	doc.resolve()
	return doc, nil
}

// RawToken returns prefixes instead of namespaces. resolve replaces them with
// the declared namespaces, but keeps the prefixes in prefix.
func (doc *Document) resolve() {
	var walk func(n *Node, scope map[string]string)
	walk = func(n *Node, scope map[string]string) {
		if n.Kind != Element {
			return
		}
		local := scope
		copied := false
		for _, a := range n.Attrs {
			if a.Name.Space == "xmlns" || (a.Name.Space == "" && a.Name.Local == "xmlns") {
				if !copied {
					local, copied = copyMap(scope), true
				}
				p := a.Name.Local
				if a.Name.Space == "" {
					p = ""
				}
				local[p] = a.Value
			}
		}
		n.Name.Space = local[n.Name.Space]
		for i, a := range n.Attrs {
			if a.Name.Space != "" && a.Name.Space != "xmlns" && a.Name.Space != "xml" {
				n.Attrs[i].Name.Space = local[a.Name.Space]
			}
		}
		for _, c := range n.Children {
			walk(c, local)
		}
	}
	walk(doc.Root, map[string]string{"": ""})
}

func copyMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m)+2)
	for k, v := range m {
		out[k] = v
	}
	return out
}

// Elements returns the child elements with the name, all of them for an empty Local.
func (n *Node) Elements(name xml.Name) []*Node {
	var out []*Node
	for _, c := range n.Children {
		if c.Kind == Element && (name.Local == "" || c.Name == name) {
			out = append(out, c)
		}
	}
	return out
}

// Attr returns an attribute.
func (n *Node) Attr(name xml.Name) (string, bool) {
	for _, a := range n.Attrs {
		if a.Name == name {
			return a.Value, true
		}
	}
	return "", false
}

// SetAttr sets an attribute.
func (n *Node) SetAttr(name xml.Name, v string) {
	for i, a := range n.Attrs {
		if a.Name == name {
			n.Attrs[i].Value = v
			return
		}
	}
	n.Attrs = append(n.Attrs, xml.Attr{Name: name, Value: v})
}

// SetText replaces the content of an element with text.
func (n *Node) SetText(v string) {
	n.Children = nil
	if v != "" {
		n.Children = []*Node{{Kind: Text, Data: v, Parent: n}}
	}
}

// Text returns the text content.
func (n *Node) Text() string {
	var b strings.Builder
	for _, c := range n.Children {
		switch c.Kind {
		case Text:
			b.WriteString(c.Data)
		case Element:
			b.WriteString(c.Text())
		}
	}
	return b.String()
}

// Insert inserts a child element at position i of the child elements (not
// of the nodes), at the end if i is out of range.
func (n *Node) Insert(c *Node, i int) {
	c.Parent = n
	elems := 0
	for k, ch := range n.Children {
		if ch.Kind != Element {
			continue
		}
		if elems == i {
			n.Children = append(n.Children[:k], append([]*Node{c}, n.Children[k:]...)...)
			return
		}
		elems++
	}
	n.Children = append(n.Children, c)
}

// Bytes writes the document as UTF-8 with an XML declaration. Elements and
// attributes get the prefix of their namespace declared in scope. If a
// declaration is missing, it is added using prefixes, a mapping from
// namespace to preferred prefix.
func (doc *Document) Bytes(prefixes map[string]string) []byte {
	var b bytes.Buffer
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	for _, n := range doc.Before {
		writeNode(&b, n, nil, prefixes)
		b.WriteByte('\n')
	}
	writeNode(&b, doc.Root, map[string]string{"": ""}, prefixes)
	b.WriteByte('\n')
	return b.Bytes()
}

// writeNode writes a node. scope maps prefixes to namespaces.
func writeNode(b *bytes.Buffer, n *Node, scope map[string]string, prefixes map[string]string) {
	switch n.Kind {
	case Text:
		escapeText(b, n.Data)
		return
	case Comment:
		b.WriteString("<!--" + n.Data + "-->")
		return
	case ProcInst:
		b.WriteString("<?" + n.Target + " " + n.Data + "?>")
		return
	}
	local := copyMap(scope)
	var decls []xml.Attr
	for _, a := range n.Attrs {
		switch {
		case a.Name.Space == "xmlns":
			local[a.Name.Local] = a.Value
			decls = append(decls, a)
		case a.Name.Space == "" && a.Name.Local == "xmlns":
			local[""] = a.Value
			decls = append(decls, a)
		}
	}
	// prefix returns the prefix for a namespace and declares it if needed.
	prefix := func(uri string, attr bool) string {
		if uri == "" {
			return ""
		}
		if uri == "http://www.w3.org/XML/1998/namespace" {
			return "xml"
		}
		if !attr && local[""] == uri {
			return ""
		}
		var cands []string
		for p, u := range local {
			if u == uri && p != "" {
				cands = append(cands, p)
			}
		}
		if len(cands) > 0 {
			sort.Strings(cands)
			return cands[0]
		}
		p := prefixes[uri]
		if p == "" {
			p = "ns"
		}
		for local[p] != "" {
			p += "x"
		}
		local[p] = uri
		decls = append(decls, xml.Attr{Name: xml.Name{Space: "xmlns", Local: p}, Value: uri})
		return p
	}
	qn := func(name xml.Name, attr bool) string {
		if p := prefix(name.Space, attr); p != "" {
			return p + ":" + name.Local
		}
		return name.Local
	}
	elem := qn(n.Name, false)
	var attrs []string
	for _, a := range n.Attrs {
		if a.Name.Space == "xmlns" || (a.Name.Space == "" && a.Name.Local == "xmlns") {
			continue
		}
		var v bytes.Buffer
		xml.EscapeText(&v, []byte(a.Value))
		attrs = append(attrs, qn(a.Name, true)+`="`+strings.ReplaceAll(v.String(), `"`, "&quot;")+`"`)
	}
	b.WriteString("<" + elem)
	for _, d := range decls {
		name := "xmlns"
		if d.Name.Space == "xmlns" {
			name += ":" + d.Name.Local
		}
		var v bytes.Buffer
		xml.EscapeText(&v, []byte(d.Value))
		b.WriteString(" " + name + `="` + v.String() + `"`)
	}
	for _, a := range attrs {
		b.WriteString(" " + a)
	}
	if len(n.Children) == 0 {
		b.WriteString("/>")
		return
	}
	b.WriteByte('>')
	for _, c := range n.Children {
		writeNode(b, c, local, prefixes)
	}
	b.WriteString("</" + elem + ">")
}

// escapeText escapes &, < and > and leaves line breaks as they are, unlike
// xml.EscapeText, which writes them as &#xA;.
func escapeText(b *bytes.Buffer, s string) {
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		default:
			b.WriteRune(r)
		}
	}
}
