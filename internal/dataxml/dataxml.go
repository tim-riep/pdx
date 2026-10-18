// Package dataxml ist ein kleines XML-Baummodell für Daten. Es liest ein
// Dokument, erlaubt Werte zu setzen und Elemente einzufügen und schreibt es
// mit den ursprünglichen Präfixen wieder. pdx extract-data schreibt damit
// Formulareingaben in die Daten zurück.
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

// Kind ist die Art eines Knotens.
type Kind int

const (
	Element Kind = iota
	Text
	Comment
	ProcInst
)

// Node ist ein Knoten. Name ist bei Elementen der aufgelöste Name, Attrs
// enthalten auch die Namespace-Deklarationen (Name.Space "xmlns" oder Local
// "xmlns" ohne Space).
type Node struct {
	Kind     Kind
	Name     xml.Name
	Attrs    []xml.Attr
	Children []*Node
	Parent   *Node
	Data     string // Text, Kommentar oder Inhalt einer Verarbeitungsanweisung
	Target   string // Ziel einer Verarbeitungsanweisung
}

// Document ist ein Dokument mit genau einem Wurzelelement.
type Document struct {
	Root *Node
	// Before sind Kommentare und Verarbeitungsanweisungen vor der Wurzel
	// (ohne die XML-Deklaration).
	Before []*Node
}

// Parse liest ein Dokument.
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

// RawToken liefert Präfixe statt Namespaces. resolve ersetzt sie durch die
// deklarierten Namespaces, behält die Präfixe aber in prefix.
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

// Elements liefert die Kindelemente mit dem Namen, alle bei leerem Local.
func (n *Node) Elements(name xml.Name) []*Node {
	var out []*Node
	for _, c := range n.Children {
		if c.Kind == Element && (name.Local == "" || c.Name == name) {
			out = append(out, c)
		}
	}
	return out
}

// Attr liefert ein Attribut.
func (n *Node) Attr(name xml.Name) (string, bool) {
	for _, a := range n.Attrs {
		if a.Name == name {
			return a.Value, true
		}
	}
	return "", false
}

// SetAttr setzt ein Attribut.
func (n *Node) SetAttr(name xml.Name, v string) {
	for i, a := range n.Attrs {
		if a.Name == name {
			n.Attrs[i].Value = v
			return
		}
	}
	n.Attrs = append(n.Attrs, xml.Attr{Name: name, Value: v})
}

// SetText ersetzt den Inhalt eines Elements durch Text.
func (n *Node) SetText(v string) {
	n.Children = nil
	if v != "" {
		n.Children = []*Node{{Kind: Text, Data: v, Parent: n}}
	}
}

// Text liefert den Textinhalt.
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

// Insert fügt ein Kindelement an Position i der Kindelemente ein (nicht der
// Knoten), am Ende bei i außerhalb.
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

// Bytes schreibt das Dokument als UTF-8 mit XML-Deklaration. Elemente und
// Attribute bekommen das im Gültigkeitsbereich deklarierte Präfix ihres
// Namespace. Fehlt eine Deklaration, wird sie mit prefixes ergänzt, einer
// Zuordnung Namespace zu bevorzugtem Präfix.
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

// writeNode schreibt einen Knoten. scope ordnet Präfixe Namespaces zu.
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
	// prefix liefert das Präfix für einen Namespace, deklariert es bei Bedarf.
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

// escapeText maskiert &, < und > und lässt Zeilenwechsel stehen, anders als
// xml.EscapeText, das sie als &#xA; schreibt.
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
