package compile

import (
	"regexp"
	"strings"

	"github.com/tim-riep/pdx/template"
)

// Arten von Tokens und von Attributen, die Tokens aufnehmen.
const (
	kindColor  = "color"
	kindLength = "length"
	kindFont   = "font"
	kindBorder = "border" // Rahmen: Längen und Farben gemischt
	kindFamily = "family" // Schriftfamilie: Familie eines Schrift-Tokens
)

// reTokenRef findet Verweise wie $primaer in einem Attributwert.
var reTokenRef = regexp.MustCompile(`\$[A-Za-z_][A-Za-z0-9_.\-]*`)

// tokenAttrs ordnet Attributnamen die Art der Tokens zu, die sie aufnehmen.
// Farben und Längen kommen aus den Typen Color, Length und Lengths des
// Vorlagen-XSD, so bleiben neue Attribute ohne Pflege dabei.
var tokenAttrs = func() map[string]string {
	m := map[string]string{
		"font": kindFont, "fontFamily": kindFamily, "lineHeight": kindLength,
		"border": kindBorder, "borderTop": kindBorder, "borderBottom": kindBorder,
		"borderLeft": kindBorder, "borderRight": kindBorder,
	}
	re := regexp.MustCompile(`<xs:attribute name="([A-Za-z]+)" type="(Color|Length|Lengths)"`)
	for _, g := range re.FindAllStringSubmatch(template.Schema, -1) {
		if g[2] == "Color" {
			m[g[1]] = kindColor
		} else {
			m[g[1]] = kindLength
		}
	}
	return m
}()

// token ist ein aufgelöstes Token.
type token struct {
	kind  string
	value string            // Farbe oder Länge
	font  map[string]string // Stilattribute eines Schrift-Tokens
}

// fontAttrs ordnet Attribute eines Schrift-Tokens Stilattributen zu.
var fontAttrs = [][2]string{{"family", "fontFamily"}, {"size", "fontSize"}, {"weight", "fontWeight"}, {"style", "fontStyle"}}

// resolveTokens liefert die Vorlage mit aufgelösten Verweisen auf Tokens.
// Ohne Tokens und Verweise ist es dieselbe Vorlage, sonst eine Kopie.
func resolveTokens(t *template.Template) (*template.Template, template.Errors) {
	if t.Root.First("tokens") == nil && !usesTokens(t.Root) {
		return t, nil
	}
	root := t.Root.Clone()
	out := &template.Template{Root: root}
	out.Reindex()
	r := &tokenResolver{decl: map[string]*template.Node{}, done: map[string]*token{}, busy: map[string]bool{}}
	if tk := root.First("tokens"); tk != nil {
		for _, n := range tk.Elements("") {
			r.decl[n.ID()] = n
		}
		for _, n := range tk.Elements("") {
			r.resolve(n.ID(), n)
		}
	}
	var walk func(n *template.Node)
	walk = func(n *template.Node) {
		if n.Name == "tokens" && n.Parent == root {
			return
		}
		r.apply(n)
		for _, c := range n.Elements("") {
			walk(c)
		}
	}
	walk(root)
	if len(r.errs) > 0 {
		return nil, r.errs
	}
	return out, nil
}

// usesTokens meldet, ob ein Attribut, das Tokens aufnimmt, einen Verweis
// enthält.
func usesTokens(n *template.Node) bool {
	for _, a := range n.Attrs {
		if _, ok := tokenAttrs[a.Name]; ok && strings.Contains(a.Value, "$") {
			return true
		}
	}
	for _, c := range n.Elements("") {
		if usesTokens(c) {
			return true
		}
	}
	return false
}

type tokenResolver struct {
	decl map[string]*template.Node
	done map[string]*token
	busy map[string]bool
	errs template.Errors
}

func (r *tokenResolver) errorf(n *template.Node, msg string) {
	r.errs = append(r.errs, &template.Error{ID: n.NearestID(), Line: n.Line, Col: n.Col, Msg: msg})
}

// resolve löst ein Token auf, auch Verweise auf andere Tokens. at ist das
// Element, an dem ein Fehler gemeldet wird. Ein unbekanntes Token oder ein
// Kreis ergibt nil.
func (r *tokenResolver) resolve(id string, at *template.Node) *token {
	if t, ok := r.done[id]; ok {
		return t
	}
	n, ok := r.decl[id]
	if !ok {
		r.errorf(at, "Token $"+id+" gibt es nicht")
		return nil
	}
	if r.busy[id] {
		r.errorf(n, "Token $"+id+" verweist im Kreis auf sich selbst")
		r.done[id] = nil
		return nil
	}
	r.busy[id] = true
	defer delete(r.busy, id)
	t := &token{kind: n.Name}
	switch n.Name {
	case kindColor, kindLength:
		v := n.Get("value", "")
		if strings.HasPrefix(v, "$") {
			ref := r.resolve(v[1:], n)
			if ref == nil {
				r.done[id] = nil
				return nil
			}
			if ref.kind != n.Name {
				r.errorf(n, "Token $"+id+" verweist auf $"+v[1:]+", das ist "+kindName(ref.kind))
				r.done[id] = nil
				return nil
			}
			v = ref.value
		}
		t.value = v
	case kindFont:
		t.font = map[string]string{}
		for _, fa := range fontAttrs {
			if v, ok := n.Attr(fa[0]); ok {
				if fa[0] == "size" && strings.HasPrefix(v, "$") {
					ref := r.resolve(v[1:], n)
					if ref == nil || ref.kind != kindLength {
						if ref != nil {
							r.errorf(n, "Schriftgröße $"+v[1:]+" ist kein Längen-Token")
						}
						continue
					}
					v = ref.value
				}
				t.font[fa[1]] = v
			}
		}
	}
	r.done[id] = t
	return t
}

// apply ersetzt die Verweise in den Attributen eines Elements. font wird zu
// den Stilattributen des Schrift-Tokens, eigene Attribute bleiben.
func (r *tokenResolver) apply(n *template.Node) {
	var font *token
	for i := range n.Attrs {
		a := &n.Attrs[i]
		kind, ok := tokenAttrs[a.Name]
		if !ok || !strings.Contains(a.Value, "$") {
			continue
		}
		if kind == kindFont {
			t := r.resolve(strings.TrimPrefix(a.Value, "$"), n)
			if t != nil && t.kind != kindFont {
				r.errorf(n, "font verweist auf $"+strings.TrimPrefix(a.Value, "$")+", das ist kein Schrift-Token")
				continue
			}
			font = t
			continue
		}
		a.Value = reTokenRef.ReplaceAllStringFunc(a.Value, func(ref string) string {
			t := r.resolve(ref[1:], n)
			if t == nil {
				return ref
			}
			switch {
			case kind == kindFamily && t.kind == kindFont:
				if f, ok := t.font["fontFamily"]; ok {
					return f
				}
				r.errorf(n, "Schrift-Token "+ref+" nennt keine Familie")
			case kind == kindBorder && (t.kind == kindColor || t.kind == kindLength):
				return t.value
			case kind == t.kind:
				return t.value
			default:
				r.errorf(n, a.Name+" erwartet "+kindName(kind)+", "+ref+" ist "+kindName(t.kind))
			}
			return ref
		})
	}
	n.RemoveAttr("font")
	if font != nil {
		for _, fa := range fontAttrs {
			if v, ok := font.font[fa[1]]; ok {
				if _, own := n.Attr(fa[1]); !own {
					n.SetAttr(fa[1], v)
				}
			}
		}
	}
}

func kindName(kind string) string {
	switch kind {
	case kindColor:
		return "ein Farb-Token"
	case kindLength:
		return "ein Längen-Token"
	case kindFont:
		return "ein Schrift-Token"
	case kindFamily:
		return "eine Schriftfamilie"
	}
	return "ein passendes Token"
}
