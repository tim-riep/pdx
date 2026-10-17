// Package template liest, prüft und schreibt das Vorlagenformat v0 von pdx
// (template.xml). Das Format beschreibt schema/template-v0.xsd, geprüft wird
// mit dem XSD-Validator von go-xslt.
//
// Die Vorlage wird als geordneter Knotenbaum gehalten, damit der Designer sie
// verlustfrei bearbeiten und Write sie kanonisch zurückschreiben kann: feste
// Einrückung, id als erstes Attribut, die übrigen alphabetisch, gemischter
// Inhalt unverändert. So bleiben Git-Diffs lesbar.
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

// NS ist der Namespace des Vorlagenformats v0. Die Domain pdx.example ist ein
// Platzhalter (RFC 2606).
const NS = "https://pdx.example/ns/template/0"

// Schema ist das XSD des Vorlagenformats v0.
//
//go:embed schema/template-v0.xsd
var Schema string

// Kind unterscheidet Elemente und Text.
type Kind uint8

const (
	// Element ist ein Elementknoten.
	Element Kind = iota
	// Text ist ein Textknoten (nur in gemischtem Inhalt).
	Text
)

// Attr ist ein Attribut.
type Attr struct {
	Name, Value string
}

// Node ist ein Knoten der Vorlage.
type Node struct {
	Kind     Kind
	Name     string // lokaler Name, der Namespace ist immer NS
	Attrs    []Attr
	Children []*Node
	Parent   *Node
	Data     string // Inhalt eines Textknotens
	Line     int
	Col      int
}

// Attr liefert ein Attribut.
func (n *Node) Attr(name string) (string, bool) {
	for _, a := range n.Attrs {
		if a.Name == name {
			return a.Value, true
		}
	}
	return "", false
}

// Get liefert ein Attribut oder def.
func (n *Node) Get(name, def string) string {
	if v, ok := n.Attr(name); ok {
		return v
	}
	return def
}

// SetAttr setzt oder ersetzt ein Attribut.
func (n *Node) SetAttr(name, value string) {
	for i := range n.Attrs {
		if n.Attrs[i].Name == name {
			n.Attrs[i].Value = value
			return
		}
	}
	n.Attrs = append(n.Attrs, Attr{name, value})
}

// RemoveAttr entfernt ein Attribut und liefert seinen bisherigen Wert.
func (n *Node) RemoveAttr(name string) (string, bool) {
	for i, a := range n.Attrs {
		if a.Name == name {
			n.Attrs = append(n.Attrs[:i:i], n.Attrs[i+1:]...)
			return a.Value, true
		}
	}
	return "", false
}

// NewElement legt ein Element ohne Eltern an.
func NewElement(name string, attrs ...Attr) *Node {
	return &Node{Kind: Element, Name: name, Attrs: attrs}
}

// NewText legt einen Textknoten an.
func NewText(data string) *Node { return &Node{Kind: Text, Data: data} }

// InsertChild fügt c an Position i unter n ein. Ein i außerhalb der Kinder
// hängt c an. c darf keinen Elternknoten haben.
func (n *Node) InsertChild(i int, c *Node) {
	if i < 0 || i > len(n.Children) {
		i = len(n.Children)
	}
	n.Children = slices.Insert(n.Children, i, c)
	c.Parent = n
}

// RemoveChild löst c von n und liefert seine bisherige Position, -1 wenn c
// kein Kind von n ist.
func (n *Node) RemoveChild(c *Node) int {
	i := slices.Index(n.Children, c)
	if i < 0 {
		return -1
	}
	n.Children = slices.Delete(n.Children, i, i+1)
	c.Parent = nil
	return i
}

// Index liefert die Position unter dem Elternknoten, -1 ohne Eltern.
func (n *Node) Index() int {
	if n.Parent == nil {
		return -1
	}
	return slices.Index(n.Parent.Children, n)
}

// Clone kopiert den Knoten mit allen Nachfahren, ohne Elternknoten.
func (n *Node) Clone() *Node {
	c := &Node{Kind: n.Kind, Name: n.Name, Attrs: slices.Clone(n.Attrs), Data: n.Data, Line: n.Line, Col: n.Col}
	for _, ch := range n.Children {
		cc := ch.Clone()
		cc.Parent = c
		c.Children = append(c.Children, cc)
	}
	return c
}

// ID liefert die ID des Elements.
func (n *Node) ID() string { return n.Get("id", "") }

// Elements liefert die Elementkinder, auf Wunsch nur mit einem Namen.
func (n *Node) Elements(name string) []*Node {
	var out []*Node
	for _, c := range n.Children {
		if c.Kind == Element && (name == "" || c.Name == name) {
			out = append(out, c)
		}
	}
	return out
}

// First liefert das erste Elementkind mit dem Namen oder nil.
func (n *Node) First(name string) *Node {
	for _, c := range n.Children {
		if c.Kind == Element && c.Name == name {
			return c
		}
	}
	return nil
}

// NearestID liefert die ID des Elements oder des nächsten Vorfahren mit ID.
func (n *Node) NearestID() string {
	for x := n; x != nil; x = x.Parent {
		if id := x.ID(); id != "" {
			return id
		}
	}
	return ""
}

// Template ist eine geladene Vorlage.
type Template struct {
	Root *Node
	ids  map[string]*Node
}

// ByID liefert das Element mit einer ID.
func (t *Template) ByID(id string) *Node { return t.ids[id] }

// Reindex baut den Index der IDs neu auf. Nötig nach Änderungen am
// Knotenbaum, bevor ByID oder Validate genutzt werden.
func (t *Template) Reindex() { t.index() }

// Error ist ein Fehler in einer Vorlage mit Vorlagenelement-ID, wenn
// zuordenbar, und Position.
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

// Errors sammelt mehrere Fehler.
type Errors []*Error

func (es Errors) Error() string {
	parts := make([]string, len(es))
	for i, e := range es {
		parts[i] = e.Error()
	}
	return strings.Join(parts, "\n")
}

// mixed sind Elemente mit gemischtem Inhalt, deren Text erhalten bleibt.
var mixed = map[string]bool{"text": true, "span": true, "string": true}

// Parse liest eine Vorlage und prüft sie gegen das XSD.
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

// ParseTree liest eine wohlgeformte Vorlage ohne Prüfung gegen das XSD, etwa
// einen Zwischenstand im Designer. Validate holt die Prüfung nach.
func ParseTree(data []byte) (*Template, error) { return parseTree(data) }

// parseTree liest den Knotenbaum ohne Schemaprüfung.
func parseTree(data []byte) (*Template, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = true
	var root *Node
	var stack []*Node
	count := 0
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
			if t.Name.Space != NS {
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

// nodeAt liefert das Element, das an einer Position im Quelltext beginnt oder
// sie zuletzt vorher begonnen hat.
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

// Validate prüft die Vorlage gegen das XSD. Die Vorlage wird dafür kanonisch
// geschrieben, Fehlerpositionen beziehen sich auf diese Fassung und werden
// dem Element mit ID zugeordnet.
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

// Write schreibt die Vorlage kanonisch.
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
