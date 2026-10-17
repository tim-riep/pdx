package dataschema

import (
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/tim-riep/pdx/template"
)

// Level ist die Schwere eines Befunds.
type Level string

const (
	// Error macht die Vorlage unbrauchbar für das Datenschema.
	Error Level = "Fehler"
	// Warning ist ein Hinweis, die Vorlage bleibt nutzbar.
	Warning Level = "Hinweis"
)

// Finding ist ein Befund der Bindungsprüfung.
type Finding struct {
	Level     Level
	ID        string
	Line, Col int
	Msg       string
}

func (f Finding) String() string {
	return (&template.Error{ID: f.ID, Line: f.Line, Col: f.Col, Msg: string(f.Level) + ": " + f.Msg}).Error()
}

// HasErrors meldet, ob unter den Befunden Fehler sind.
func HasErrors(fs []Finding) bool {
	for _, f := range fs {
		if f.Level == Error {
			return true
		}
	}
	return false
}

// numeric sind die eingebauten Typen, die format-number verarbeitet.
var numeric = map[string]bool{
	"decimal": true, "integer": true, "nonPositiveInteger": true, "negativeInteger": true,
	"long": true, "int": true, "short": true, "byte": true, "nonNegativeInteger": true,
	"unsignedLong": true, "unsignedInt": true, "unsignedShort": true, "unsignedByte": true,
	"positiveInteger": true, "float": true, "double": true,
}

// ctx ist der Kontext einer Bindung: der Pfad der Elemente von der Wurzel
// bis zum Kontextknoten. Ein leerer Pfad steht für den Dokumentknoten.
type ctx []*Decl

type checker struct {
	m       Model
	t       *template.Template
	ns      map[string]string
	formats map[string]string // Mustername zu Art
	out     []Finding
	kids    map[*Decl][]*Decl
	open    map[*Decl]bool
}

// Check prüft alle Bindungen der Vorlage gegen das Modell.
func Check(t *template.Template, m Model) []Finding {
	c := &checker{m: m, t: t, ns: map[string]string{}, formats: map[string]string{},
		kids: map[*Decl][]*Decl{}, open: map[*Decl]bool{}}
	root := t.Root
	if nss := root.First("namespaces"); nss != nil {
		for _, n := range nss.Elements("ns") {
			c.ns[n.Get("prefix", "")] = n.Get("uri", "")
		}
	}
	if f := root.First("formats"); f != nil {
		for _, l := range f.Elements("locale") {
			for _, p := range l.Elements("pattern") {
				c.formats[p.Get("name", "")] = p.Get("kind", "")
			}
		}
	}
	start, ok := c.dataRoot()
	if !ok {
		return c.out
	}
	for _, name := range []string{"pageSet", "body"} {
		if n := root.First(name); n != nil {
			c.walk(n, start)
		}
	}
	return c.out
}

func (c *checker) report(l Level, n *template.Node, format string, args ...any) {
	c.out = append(c.out, Finding{Level: l, ID: n.NearestID(), Line: n.Line, Col: n.Col, Msg: fmt.Sprintf(format, args...)})
}

func (c *checker) dataRoot() (ctx, bool) {
	root := c.t.Root
	roots := c.m.Roots()
	dr, ok := root.Attr("dataRoot")
	if !ok {
		if len(roots) == 1 {
			return ctx{roots[0]}, true
		}
		c.report(Error, root, "dataRoot fehlt und das Datenschema hat %d globale Elemente", len(roots))
		return nil, false
	}
	name, ok := c.qname(dr, false)
	if !ok {
		c.report(Error, root, "Präfix in dataRoot %q ist nicht in namespaces deklariert", dr)
		return nil, false
	}
	for _, r := range roots {
		if r.Name == name {
			return ctx{r}, true
		}
	}
	c.report(Error, root, "dataRoot %s ist kein globales Element des Datenschemas", dr)
	return nil, false
}

// qname löst einen Namen mit Präfix auf. Ohne Präfix gilt in XPath kein
// Namespace.
func (c *checker) qname(s string, attr bool) (xml.Name, bool) {
	if i := strings.IndexByte(s, ':'); i >= 0 {
		uri, ok := c.ns[s[:i]]
		return xml.Name{Space: uri, Local: s[i+1:]}, ok
	}
	return xml.Name{Local: s}, true
}

func (c *checker) children(d *Decl) ([]*Decl, bool) {
	if k, ok := c.kids[d]; ok {
		return k, c.open[d]
	}
	k, open := c.m.Children(d)
	c.kids[d], c.open[d] = k, open
	return k, open
}

// walk prüft die Bindungen unterhalb eines Vorlagenelements im Kontext cur.
func (c *checker) walk(n *template.Node, cur ctx) {
	for _, ch := range n.Elements("") {
		c.elem(ch, cur)
	}
}

// elem prüft ein Vorlagenelement und seine Kinder im Kontext cur.
func (c *checker) elem(n *template.Node, cur ctx) {
	switch n.Name {
	case "subform":
		c.walk(n, c.subform(n, cur))
	case "field":
		c.value(n, "bind", cur, n.Get("format", ""))
	case "barcode":
		if _, ok := n.Attr("bind"); ok {
			c.value(n, "bind", cur, "")
		}
		for _, d := range n.Elements("data") {
			if _, ok := d.Attr("bind"); ok {
				c.value(d, "bind", cur, "")
			}
		}
	case "table":
		c.table(n, cur)
	default:
		c.walk(n, cur)
	}
}

func (c *checker) subform(n *template.Node, cur ctx) ctx {
	b, ok := n.Attr("bind")
	if !ok {
		return cur
	}
	r, ok := c.resolve(n, b, cur)
	if !ok {
		return nil
	}
	if r.max != 1 && n.Get("repeat", "false") != "true" {
		c.report(Error, n, "%s kann mehrfach vorkommen, das Teilformular wiederholt sich aber nicht (repeat=\"true\" fehlt)", b)
	}
	return r.path
}

func (c *checker) table(n *template.Node, cur ctx) {
	b := n.Get("bind", "")
	r, ok := c.resolve(n, b, cur)
	var item ctx
	if ok {
		item = r.path
		if r.max == 1 {
			c.report(Warning, n, "%s kommt höchstens einmal vor, die Tabelle hat dann nur eine Zeile", b)
		}
	}
	for _, sec := range n.Elements("") {
		switch sec.Name {
		case "header", "footer":
			c.section(sec, cur, item)
		case "rows":
			c.section(sec, item, item)
		case "group":
			if item != nil {
				c.value(sec, "by", item, "")
			}
			for _, s := range sec.Elements("") {
				c.section(s, item, item)
			}
		}
	}
}

// section prüft Zeilen einer Tabelle. Summen beziehen sich immer auf die
// Datenknoten der Tabelle (item).
func (c *checker) section(s *template.Node, cur, item ctx) {
	var walk func(n *template.Node)
	walk = func(n *template.Node) {
		for _, ch := range n.Elements("") {
			switch ch.Name {
			case "carryOver", "subtotal", "total":
				if item != nil {
					c.sum(ch, item)
				}
			case "row", "cell":
				walk(ch)
			default:
				c.elem(ch, cur)
			}
		}
	}
	walk(s)
}

func (c *checker) sum(n *template.Node, item ctx) {
	of := n.Get("of", "")
	r, ok := c.resolve(n, of, item)
	if !ok {
		return
	}
	if r.max != 1 {
		c.report(Error, n, "%s kann je Datenknoten mehrfach vorkommen, eine Summe braucht einen Wert", of)
	}
	if r.decl.Type != "" && !numeric[r.decl.Type] {
		c.report(Error, n, "%s hat den Typ xs:%s, summiert werden nur Zahlen", of, r.decl.Type)
	} else if !r.decl.Simple {
		c.report(Error, n, "%s hat keinen Textinhalt, summiert werden nur Zahlen", of)
	}
}

// value prüft eine Bindung, die einen einzelnen Wert liefern muss.
func (c *checker) value(n *template.Node, attr string, cur ctx, format string) {
	expr := n.Get(attr, "")
	r, ok := c.resolve(n, expr, cur)
	if !ok {
		return
	}
	if r.max != 1 {
		c.report(Error, n, "%s kann mehrfach vorkommen, wiederholt werden nur Tabellen und Teilformulare mit repeat=\"true\"", expr)
	}
	if !r.decl.Simple {
		c.report(Error, n, "%s hat keinen Textinhalt, sondern nur Unterelemente", expr)
		return
	}
	if format == "" {
		return
	}
	kind, ok := c.formats[format]
	if !ok || r.decl.Type == "" {
		return
	}
	switch kind {
	case "number":
		if !numeric[r.decl.Type] {
			c.report(Error, n, "Zahlenformat %s an %s vom Typ xs:%s", format, expr, r.decl.Type)
		}
	case "date":
		if r.decl.Type != "date" {
			c.report(Error, n, "Datumsformat %s an %s vom Typ xs:%s, erwartet ist xs:date", format, expr, r.decl.Type)
		}
	case "dateTime":
		if r.decl.Type != "dateTime" {
			c.report(Error, n, "Format %s an %s vom Typ xs:%s, erwartet ist xs:dateTime", format, expr, r.decl.Type)
		}
	}
}

// result ist das Ergebnis eines aufgelösten Pfades.
type result struct {
	path     ctx
	decl     *Decl
	min, max int
}

// resolve wertet einen Pfad statisch aus. Ausdrücke, die kein einfacher Pfad
// sind, werden nicht geprüft (ok ist dann false ohne Befund).
func (c *checker) resolve(n *template.Node, expr string, cur ctx) (result, bool) {
	if cur == nil {
		return result{}, false
	}
	steps, abs, ok := splitPath(strings.TrimSpace(expr))
	if !ok {
		return result{}, false
	}
	path := append(ctx(nil), cur...)
	if abs {
		path = ctx{}
	}
	lo, hi := 1, 1
	var last *Decl
	if len(path) > 0 {
		last = path[len(path)-1]
	}
	for _, st := range steps {
		switch st.kind {
		case stepSelf:
		case stepParent:
			if len(path) == 0 {
				c.report(Error, n, "%s führt über die Wurzel der Daten hinaus", expr)
				return result{}, false
			}
			path = path[:len(path)-1]
			if len(path) == 0 {
				last = nil
			} else {
				last = path[len(path)-1]
			}
		default:
			name, ok := c.qname(st.name, st.kind == stepAttr)
			if !ok {
				c.report(Error, n, "Präfix in %s ist nicht in namespaces deklariert", st.name)
				return result{}, false
			}
			var next *Decl
			if len(path) == 0 {
				// Schritt vom Dokumentknoten: nur das Wurzelelement.
				for _, r := range c.m.Roots() {
					if r.Name == name && !st.attr() {
						next = r
					}
				}
			} else {
				kids, open := c.children(last)
				for _, k := range kids {
					if k.Name == name && k.Attr == st.attr() {
						next = k
						break
					}
				}
				if next == nil && open {
					return result{}, false
				}
			}
			if next == nil {
				where := "der Wurzel"
				if last != nil {
					where = display(last.Name, c.ns)
				}
				what := "Element"
				if st.attr() {
					what = "Attribut"
				}
				c.report(Error, n, "%s: %s %s gibt es unter %s nicht", expr, what, st.name, where)
				return result{}, false
			}
			lo, hi = lo*next.Min, mulMax(hi, next.Max)
			path = append(path, next)
			last = next
		}
		switch st.pred {
		case predIndex:
			if hi != 0 {
				hi = 1
			}
			lo = 0
		case predFilter:
			lo = 0
		}
	}
	if last == nil {
		return result{}, false
	}
	return result{path: path, decl: last, min: lo, max: hi}, true
}

// display schreibt einen Namen mit dem Präfix der Vorlage, bei mehreren
// Präfixen für denselben Namespace mit dem alphabetisch ersten.
func display(n xml.Name, ns map[string]string) string {
	best := ""
	for p, uri := range ns {
		if uri == n.Space && n.Space != "" && (best == "" || p < best) {
			best = p
		}
	}
	if best == "" {
		return n.Local
	}
	return best + ":" + n.Local
}
