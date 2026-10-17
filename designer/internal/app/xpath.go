package app

import (
	"errors"
	"strings"

	"github.com/tim-riep/pdx/designer/internal/model"
	"github.com/tim-riep/pdx/designer/internal/schema"
	"github.com/tim-riep/pdx/designer/internal/xpath"
)

// XPathQuery beschreibt einen Ausdruck an einem Vorlagenelement. Cursor ist
// die Stelle in Zeichen (UTF-16 wie im Browser) für die Vervollständigung.
type XPathQuery struct {
	Ref    string `json:"ref"`
	Expr   string `json:"expr"`
	Cursor int    `json:"cursor"`
}

// XPathInfo ist das Ergebnis der Prüfung eines Ausdrucks.
type XPathInfo struct {
	// Context ist der Datenkontext des Elements, leer ohne Datenwurzel.
	Context string `json:"context"`
	// Data nennt die Daten der Auswertung: den Pfad der Beispieldaten oder
	// leer für Platzhalter.
	Data     string          `json:"data"`
	Problems []xpath.Problem `json:"problems"`
	// Target ist der Datenknoten eines einfachen Pfads.
	Target *Field        `json:"target"`
	Result *xpath.Result `json:"result"`
}

// Field ist ein Datenknoten, relativ zum Kontext eines Elements.
type Field struct {
	Path     string `json:"path"` // relativer XPath
	Type     string `json:"type"`
	Repeated bool   `json:"repeated"`
	Optional bool   `json:"optional"`
}

// xpathInput sammelt unter der Sperre, was Prüfung und Vervollständigung
// brauchen.
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
	// Beispieldaten wie in der Vorschau, sonst Platzhalter.
	sample := s.c.r.sample
	if sample == "" {
		if l := d.Pkg.Samples(); len(l) > 0 {
			sample = l[0]
		}
	}
	if b, ok := d.Pkg.Files[sample]; ok {
		in.data, in.sample = b, sample
	} else if in.sc != nil {
		in.data = in.sc.Placeholder()
	}
	return in, nil
}

// CheckXPath prüft einen Ausdruck: Pfade gegen das Daten-XSD, Syntax beim
// Übersetzen und Wert mit Beispieldaten im ersten Vorkommen des Kontexts.
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

// CompleteXPath liefert Vorschläge für die Stelle Cursor.
func (s *TemplateService) CompleteXPath(q XPathQuery) (xpath.Completion, error) {
	in, err := s.xpathInput(q.Ref)
	if err != nil {
		return xpath.Completion{}, err
	}
	c := xpath.Complete(in.sc, in.ctx, q.Expr, byteIndex(q.Expr, q.Cursor))
	c.From, c.To = utf16Index(q.Expr, c.From), utf16Index(q.Expr, c.To)
	return c, nil
}

// XPathFields listet die Datenknoten unter dem Kontext eines Elements mit
// relativem Pfad, etwa für die Auswahl bei Präsenzbedingungen.
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
			// Nicht in wiederholte Knoten hinein: dort ist der Pfad mehrdeutig.
			if depth < 3 && !k.Repeated() {
				walk(k, depth+1)
			}
		}
	}
	walk(root, 0)
	return out, nil
}

// utf16Index rechnet eine Byte-Position in eine Position in UTF-16-Einheiten
// um, wie sie JavaScript zählt.
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

// byteIndex ist die Umkehrung von utf16Index.
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
