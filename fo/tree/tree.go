// Package tree liest XSL-FO in einen Knotenbaum mit Zeilen- und
// Spaltenangaben. Er prüft, dass nur Elemente des unterstützten Teilumfangs
// vorkommen, und meldet alles andere mit Position statt es still zu
// übergehen.
//
// Elemente fremder Namespaces in fo:declarations werden übernommen, damit
// das Layout die Erweiterungen von pdx (Namespace https://pdx.example/ns/pdx/1.0) lesen
// kann. Andere fremde Elemente werden ignoriert, wie es XSL 1.1 erlaubt.
package tree

import (
	"golang.org/x/text/encoding/charmap"

	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	// NSFO ist der XSL-FO-Namespace.
	NSFO = "http://www.w3.org/1999/XSL/Format"
	// NSPDX ist der Erweiterungs-Namespace von pdx. Die Domain pdx.example
	// ist ein Platzhalter (RFC 2606) und wird durch die endgültige Domain
	// ersetzt. Der Namespace steht nur hier, in den Testdaten und im README.
	NSPDX = "https://pdx.example/ns/pdx/1.0"
	// NSXML ist der XML-Namespace für xml:lang.
	NSXML = "http://www.w3.org/XML/1998/namespace"
)

// Kind unterscheidet Elemente und Text.
type Kind uint8

const (
	// Element ist ein Elementknoten.
	Element Kind = iota
	// Text ist ein Textknoten.
	Text
)

// Node ist ein Knoten des FO-Baums.
type Node struct {
	Kind Kind
	// Space ist der Namespace, Name der lokale Name, etwa NSFO und "block".
	Space, Name string
	// Attrs enthält Attribute ohne Namespace unter ihrem Namen, xml:lang als
	// "xml:lang" und Attribute fremder Namespaces als "{ns}name".
	Attrs    map[string]string
	Children []*Node
	Parent   *Node
	// Data ist der Inhalt eines Textknotens.
	Data string
	// Line und Col geben die Position im Quelltext an (ab 1).
	Line, Col int
}

// Pos liefert die Position als "Zeile:Spalte".
func (n *Node) Pos() string { return fmt.Sprintf("%d:%d", n.Line, n.Col) }

// Is prüft Namespace und lokalen Namen.
func (n *Node) Is(space, name string) bool {
	return n.Kind == Element && n.Space == space && n.Name == name
}

// IsFO prüft auf ein FO-Element mit diesem Namen.
func (n *Node) IsFO(name string) bool { return n.Is(NSFO, name) }

// Attr liefert ein Attribut und ob es gesetzt ist.
func (n *Node) Attr(name string) (string, bool) {
	v, ok := n.Attrs[name]
	return v, ok
}

// Label beschreibt den Knoten für Fehlermeldungen, etwa "fo:block@12:5".
func (n *Node) Label() string {
	prefix := "{" + n.Space + "}"
	switch n.Space {
	case NSFO:
		prefix = "fo:"
	case NSPDX:
		prefix = "pdx:"
	}
	return prefix + n.Name + "@" + n.Pos()
}

// Error ist ein Fehler mit Position im Quelltext.
type Error struct {
	Line, Col int
	Msg       string
}

func (e *Error) Error() string { return fmt.Sprintf("%d:%d: %s", e.Line, e.Col, e.Msg) }

// Errorf baut einen Fehler an der Position eines Knotens.
func Errorf(n *Node, format string, args ...any) error {
	return &Error{Line: n.Line, Col: n.Col, Msg: fmt.Sprintf(format, args...)}
}

// Elemente des unterstützten Teilumfangs und ihre erlaubten FO-Kinder. "#text"
// erlaubt Text, "#block" alle Blockelemente, "#inline" alle Inlineelemente.
var allowed = map[string][]string{
	"root":                                {"layout-master-set", "declarations", "bookmark-tree", "page-sequence"},
	"bookmark-tree":                       {"bookmark"},
	"bookmark":                            {"bookmark-title", "bookmark"},
	"bookmark-title":                      {"#text"},
	"leader":                              {"#inline", "#text"},
	"basic-link":                          {"#inline", "#text"},
	"instream-foreign-object":             {},
	"layout-master-set":                   {"simple-page-master", "page-sequence-master"},
	"page-sequence-master":                {"single-page-master-reference", "repeatable-page-master-reference", "repeatable-page-master-alternatives"},
	"single-page-master-reference":        {},
	"repeatable-page-master-reference":    {},
	"repeatable-page-master-alternatives": {"conditional-page-master-reference"},
	"conditional-page-master-reference":   {},
	"declarations":                        {},
	"simple-page-master":                  {"region-body", "region-before", "region-after"},
	"region-body":                         {},
	"region-before":                       {},
	"region-after":                        {},
	"page-sequence":                       {"flow", "static-content"},
	"flow":                                {"#block"},
	"static-content":                      {"#block"},
	"block":                               {"#block", "#inline", "#text"},
	"block-container":                     {"#block"},
	"inline":                              {"#inline", "#text"},
	"external-graphic":                    {},
	"table":                               {"table-column", "table-header", "table-footer", "table-body"},
	"table-column":                        {},
	"table-header":                        {"table-row", "table-cell"},
	"table-footer":                        {"table-row", "table-cell"},
	"table-body":                          {"table-row", "table-cell"},
	"table-row":                           {"table-cell"},
	"table-cell":                          {"#block"},
	"list-block":                          {"list-item"},
	"list-item":                           {"list-item-label", "list-item-body"},
	"list-item-label":                     {"#block"},
	"list-item-body":                      {"#block"},
	"marker":                              {"#block", "#inline", "#text"},
	"retrieve-marker":                     {},
	"retrieve-table-marker":               {},
	"page-number":                         {},
	"page-number-citation":                {},
	"page-number-citation-last":           {},
}

var blockLevel = map[string]bool{"block": true, "block-container": true, "table": true, "list-block": true,
	"retrieve-marker": true, "retrieve-table-marker": true}
var inlineLevel = map[string]bool{"inline": true, "external-graphic": true, "page-number": true,
	"page-number-citation": true, "page-number-citation-last": true,
	"retrieve-marker": true, "retrieve-table-marker": true, "leader": true, "basic-link": true, "instream-foreign-object": true}

// markerParents dürfen fo:marker als Kinder haben.
var markerParents = map[string]bool{"block": true, "block-container": true, "inline": true, "table-row": true,
	"table-cell": true, "list-item": true, "list-item-label": true, "list-item-body": true}

// Limits schützen vor entarteten Eingaben.
const (
	maxDepth = 256
	maxNodes = 1_000_000
)

// Parse liest ein FO-Dokument.
func Parse(r io.Reader) (*Node, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return ParseBytes(data)
}

// ParseBytes liest ein FO-Dokument aus Bytes.
func ParseBytes(data []byte) (*Node, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = true
	dec.CharsetReader = charsetReader
	var root *Node
	var stack []*Node
	nodes := 0
	// ignoreDepth > 0 bedeutet: innerhalb eines ignorierten fremden Elements.
	ignoreDepth := 0
	for {
		line, col := dec.InputPos()
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, &Error{Line: line, Col: col, Msg: "XML nicht wohlgeformt: " + xmlMsg(err)}
		}
		nodes++
		if nodes > maxNodes {
			return nil, &Error{Line: line, Col: col, Msg: "Dokument hat zu viele Knoten"}
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if ignoreDepth > 0 {
				ignoreDepth++
				continue
			}
			n := &Node{Kind: Element, Space: t.Name.Space, Name: t.Name.Local, Attrs: map[string]string{}, Line: line, Col: col}
			for _, a := range t.Attr {
				switch {
				case a.Name.Space == "xmlns" || (a.Name.Space == "" && a.Name.Local == "xmlns"):
				case a.Name.Space == "":
					n.Attrs[a.Name.Local] = a.Value
				case a.Name.Space == NSXML || a.Name.Space == "xml":
					n.Attrs["xml:lang"] = a.Value
				default:
					n.Attrs["{"+a.Name.Space+"}"+a.Name.Local] = a.Value
				}
			}
			if len(stack) == 0 {
				if root != nil {
					return nil, Errorf(n, "mehr als ein Wurzelelement")
				}
				if !n.IsFO("root") {
					return nil, Errorf(n, "Wurzelelement muss fo:root sein, gefunden %s", t.Name.Local)
				}
				root = n
				stack = append(stack, n)
				continue
			}
			parent := stack[len(stack)-1]
			if len(stack) >= maxDepth {
				return nil, Errorf(n, "Verschachtelung tiefer als %d Ebenen", maxDepth)
			}
			if n.Space != NSFO {
				// Fremde Elemente: in fo:declarations übernehmen, sonst ignorieren.
				if parent.IsFO("declarations") || parent.IsFO("instream-foreign-object") || (parent.Space != NSFO && parent.Space != "") {
					n.Parent = parent
					parent.Children = append(parent.Children, n)
					stack = append(stack, n)
				} else {
					ignoreDepth = 1
				}
				continue
			}
			if parent.Space != NSFO {
				return nil, Errorf(n, "fo:%s darf nicht in einem fremden Element stehen", n.Name)
			}
			if err := checkChild(parent, n); err != nil {
				return nil, err
			}
			n.Parent = parent
			parent.Children = append(parent.Children, n)
			stack = append(stack, n)
		case xml.EndElement:
			if ignoreDepth > 0 {
				ignoreDepth--
				continue
			}
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if ignoreDepth > 0 || len(stack) == 0 {
				continue
			}
			parent := stack[len(stack)-1]
			s := string(t)
			if parent.Space != NSFO {
				// Text in Erweiterungselementen wird übernommen.
				parent.Children = append(parent.Children, &Node{Kind: Text, Data: s, Parent: parent, Line: line, Col: col})
				continue
			}
			if !acceptsText(parent.Name) {
				if strings.TrimSpace(s) != "" {
					return nil, &Error{Line: line, Col: col, Msg: fmt.Sprintf("fo:%s darf keinen Text enthalten", parent.Name)}
				}
				continue
			}
			if k := len(parent.Children); k > 0 && parent.Children[k-1].Kind == Text {
				parent.Children[k-1].Data += s
				continue
			}
			parent.Children = append(parent.Children, &Node{Kind: Text, Data: s, Parent: parent, Line: line, Col: col})
		}
	}
	if root == nil {
		return nil, &Error{Line: 1, Col: 1, Msg: "Dokument ist leer"}
	}
	resolveInherit(root)
	return root, nil
}

// charsetReader liest Dokumente in ISO-8859-1 und windows-1252. UTF-8 und
// UTF-16 erkennt encoding/xml selbst, weitere Kodierungen verlangt XML nicht.
func charsetReader(label string, in io.Reader) (io.Reader, error) {
	switch strings.ToLower(label) {
	case "iso-8859-1", "latin1", "iso_8859-1", "l1":
		return charmap.ISO8859_1.NewDecoder().Reader(in), nil
	case "windows-1252", "cp1252":
		return charmap.Windows1252.NewDecoder().Reader(in), nil
	case "us-ascii", "ascii":
		return in, nil
	}
	return nil, fmt.Errorf("Zeichenkodierung %q wird nicht unterstützt, erlaubt sind UTF-8, UTF-16, ISO-8859-1 und windows-1252", label)
}

// resolveInherit ersetzt den Wert inherit, der nach XSL 1.1 5.1.4 bei jeder
// Eigenschaft erlaubt ist, durch den Wert desselben Attributs am nächsten
// Vorfahren. Hat kein Vorfahr das Attribut, entfällt es und der Anfangswert
// gilt.
func resolveInherit(n *Node) {
	for name, v := range n.Attrs {
		if strings.TrimSpace(v) != "inherit" {
			continue
		}
		delete(n.Attrs, name)
		for p := n.Parent; p != nil; p = p.Parent {
			if pv, ok := p.Attrs[name]; ok {
				n.Attrs[name] = pv
				break
			}
			// Zusammengesetzte Eigenschaften wie space-after können am
			// Vorfahren nur als Komponenten stehen (space-after.optimum).
			found := false
			for pn, pv := range p.Attrs {
				if strings.HasPrefix(pn, name+".") {
					if _, own := n.Attrs[pn]; !own {
						n.Attrs[pn] = pv
					}
					found = true
				}
			}
			if found {
				break
			}
		}
	}
	for _, c := range n.Children {
		if c.Kind == Element {
			resolveInherit(c)
		}
	}
}

func acceptsText(name string) bool {
	for _, c := range allowed[name] {
		if c == "#text" {
			return true
		}
	}
	return false
}

func checkChild(parent, n *Node) error {
	if n.Name == "marker" {
		if markerParents[parent.Name] {
			return nil
		}
		return Errorf(n, "fo:marker ist in fo:%s nicht erlaubt", parent.Name)
	}
	if _, known := allowed[n.Name]; !known {
		return Errorf(n, "fo:%s wird noch nicht unterstützt", n.Name)
	}
	for _, c := range allowed[parent.Name] {
		switch {
		case c == n.Name:
			return nil
		case c == "#block" && blockLevel[n.Name]:
			return nil
		case c == "#inline" && inlineLevel[n.Name]:
			return nil
		}
	}
	return Errorf(n, "fo:%s ist in fo:%s nicht erlaubt", n.Name, parent.Name)
}

func xmlMsg(err error) string {
	var se *xml.SyntaxError
	if errors.As(err, &se) {
		return se.Msg
	}
	return err.Error()
}
