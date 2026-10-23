package compile

import (
	"regexp"
	"strings"

	"github.com/tim-riep/pdx/template"
)

// Kinds of tokens and of attributes that accept tokens.
const (
	kindColor  = "color"
	kindLength = "length"
	kindFont   = "font"
	kindBorder = "border" // border: lengths and colors mixed
	kindFamily = "family" // font family: family of a font token
)

// reTokenRef finds references like $primaer in an attribute value.
var reTokenRef = regexp.MustCompile(`\$[A-Za-z_][A-Za-z0-9_.\-]*`)

// tokenAttrs maps attribute names to the kind of tokens they accept.
// Colors and lengths come from the types Color, Length and Lengths of the
// template XSD, so new attributes are covered without maintenance.
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

// token is a resolved token.
type token struct {
	kind  string
	value string            // color or length
	font  map[string]string // style attributes of a font token
}

// fontAttrs maps attributes of a font token to style attributes.
var fontAttrs = [][2]string{{"family", "fontFamily"}, {"size", "fontSize"}, {"weight", "fontWeight"}, {"style", "fontStyle"}}

// resolveTokens returns the template with resolved references to tokens.
// Without tokens and references it is the same template, otherwise a copy.
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

// usesTokens reports whether an attribute that accepts tokens contains a
// reference.
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

// resolve resolves a token, including references to other tokens. at is the
// element at which an error is reported. An unknown token or a cycle
// yields nil.
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

// apply replaces the references in the attributes of an element. font
// becomes the style attributes of the font token, own attributes remain.
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
