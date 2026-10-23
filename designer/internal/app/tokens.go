package app

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/tim-riep/pdx/designer/internal/model"
	"github.com/tim-riep/pdx/template"
)

// TokenOp is a change to the design tokens.
//
//	add     Kind color, length or font, ID as suggestion (empty: by kind)
//	rename  ID, NewID  references in the template follow along
//	remove  ID         only without references
type TokenOp struct {
	Op    string `json:"op"`
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	NewID string `json:"newId"`
}

// Defaults of new tokens per kind.
var tokenDefaults = map[string][]template.Attr{
	"color":  {{Name: "value", Value: "#336699"}},
	"length": {{Name: "value", Value: "4mm"}},
	"font":   {{Name: "family", Value: "Liberation Sans"}, {Name: "size", Value: "10pt"}},
}

var tokenBase = map[string]string{"color": "farbe", "length": "abstand", "font": "schrift"}

// Tokens changes the design tokens as one step.
func (s *TemplateService) Tokens(op TokenOp) (Result, error) {
	cat, err := model.LoadCatalog()
	if err != nil {
		return Result{}, err
	}
	return s.change(func(d *model.Document) (string, error) {
		t := d.Pkg.Template
		b := &builder{t: t, used: map[string]bool{}}
		tokens := t.Root.First("tokens")
		switch op.Op {
		case "add":
			defs, ok := tokenDefaults[op.Kind]
			if !ok {
				return "", fmt.Errorf("Token-Art %q unbekannt", op.Kind)
			}
			base := op.ID
			if base == "" {
				base = tokenBase[op.Kind]
			}
			id := b.id(base)
			el := template.NewElement(op.Kind, append([]template.Attr{{Name: "id", Value: id}}, defs...)...)
			var cmd model.Command
			if tokens == nil {
				// tokens comes after namespaces and styles.
				wrap := template.NewElement("tokens")
				wrap.InsertChild(-1, el)
				cmd = model.Command{Op: "insert", Parent: model.Ref(t.Root), Index: tokensIndex(t.Root), XML: fragment(wrap)}
			} else {
				cmd = model.Command{Op: "insert", Parent: model.Ref(tokens), Index: -1, XML: fragment(el)}
			}
			return id, d.Apply(map[string]string{"color": "Farb-Token anlegen", "length": "Längen-Token anlegen", "font": "Schrift-Token anlegen"}[op.Kind], cmd)
		case "rename":
			n := tokenNode(t, op.ID)
			if n == nil {
				return "", fmt.Errorf("Token $%s gibt es nicht", op.ID)
			}
			if op.NewID == op.ID {
				return op.ID, nil
			}
			if !idPattern.MatchString(op.NewID) {
				return "", fmt.Errorf("„%s“ taugt nicht als Name eines Tokens", op.NewID)
			}
			if t.ByID(op.NewID) != nil {
				return "", fmt.Errorf("Der Name %s ist schon vergeben", op.NewID)
			}
			cmds := []model.Command{{Op: "setAttr", Ref: model.Ref(n), Name: "id", Value: op.NewID}}
			re := refPattern(op.ID)
			eachTokenAttr(t, cat, func(x *template.Node, a template.Attr) {
				if v := replaceRef(re, a.Value, op.NewID); v != a.Value {
					cmds = append(cmds, model.Command{Op: "setAttr", Ref: model.Ref(x), Name: a.Name, Value: v})
				}
			})
			return op.NewID, d.Apply("Token umbenennen", model.Command{Op: "batch", Cmds: cmds})
		case "remove":
			n := tokenNode(t, op.ID)
			if n == nil {
				return "", fmt.Errorf("Token $%s gibt es nicht", op.ID)
			}
			if used := tokenUsage(t, cat)[op.ID]; used > 0 {
				return "", fmt.Errorf("Token $%s wird noch an %d Stellen verwendet", op.ID, used)
			}
			target := n
			if len(tokens.Elements("")) == 1 {
				target = tokens // an empty tokens is invalid
			}
			return "", d.Apply("Token entfernen", model.Command{Op: "remove", Ref: model.Ref(target)})
		}
		return "", fmt.Errorf("Token-Befehl %q unbekannt", op.Op)
	})
}

// TokenUsage counts the references in the template per token.
func (s *TemplateService) TokenUsage() (map[string]int, error) {
	cat, err := model.LoadCatalog()
	if err != nil {
		return nil, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	if s.c.doc == nil {
		return nil, errors.New("keine Vorlage geöffnet")
	}
	return tokenUsage(s.c.doc.Pkg.Template, cat), nil
}

var (
	idPattern  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.\-]*$`)
	anyRef     = regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_.\-]*)`)
	tokenKinds = map[string]bool{"color": true, "length": true, "font": true}
)

// refPattern finds references to exactly one token, not to longer names
// with the same beginning.
func refPattern(id string) *regexp.Regexp {
	return regexp.MustCompile(`(^|[^A-Za-z0-9_.\-])\$` + regexp.QuoteMeta(id) + `($|[^A-Za-z0-9_.\-])`)
}

// replaceRef replaces all references. Adjacent references such as "$m $m"
// overlap in the pattern, so it repeats until nothing matches.
func replaceRef(re *regexp.Regexp, v, id string) string {
	for {
		next := re.ReplaceAllString(v, "${1}$$"+id+"${2}")
		if next == v {
			return v
		}
		v = next
	}
}

func tokenNode(t *template.Template, id string) *template.Node {
	n := t.ByID(id)
	if n == nil || n.Parent == nil || n.Parent.Name != "tokens" || !tokenKinds[n.Name] {
		return nil
	}
	return n
}

// tokensIndex returns the position for a new tokens: after namespaces and
// styles.
func tokensIndex(root *template.Node) int {
	idx := 0
	for i, c := range root.Children {
		if c.Kind == template.Element && (c.Name == "namespaces" || c.Name == "styles") {
			idx = i + 1
		}
	}
	return idx
}

// eachTokenAttr calls f for every attribute that can contain references to
// tokens: all except XPath, IDs and texts such as patterns. This way
// variables in XPath expressions do not interfere.
func eachTokenAttr(t *template.Template, cat model.Catalog, f func(*template.Node, template.Attr)) {
	var walk func(n *template.Node)
	walk = func(n *template.Node) {
		et := cat.TypeOf(n)
		for _, a := range n.Attrs {
			if !strings.Contains(a.Value, "$") {
				continue
			}
			kind := ""
			if et != nil {
				for _, p := range et.Properties {
					if p.Name == a.Name {
						kind = p.Kind
					}
				}
			}
			switch kind {
			case "color", "length", "lengths", "font":
				f(n, a)
			case "text":
				// Border, font family and line height are free texts.
				if strings.HasPrefix(a.Name, "border") || a.Name == "fontFamily" || a.Name == "lineHeight" {
					f(n, a)
				}
			}
		}
		for _, c := range n.Elements("") {
			walk(c)
		}
	}
	walk(t.Root)
}

func tokenUsage(t *template.Template, cat model.Catalog) map[string]int {
	out := map[string]int{}
	eachTokenAttr(t, cat, func(_ *template.Node, a template.Attr) {
		for _, m := range anyRef.FindAllStringSubmatch(a.Value, -1) {
			out[m[1]]++
		}
	})
	return out
}

// AddStyle creates a style, optionally with a parent style, and creates
// styles if it is missing.
func (s *TemplateService) AddStyle(parent string) (Result, error) {
	return s.change(func(d *model.Document) (string, error) {
		t := d.Pkg.Template
		if parent != "" {
			if p := t.ByID(parent); p == nil || p.Name != "style" {
				return "", fmt.Errorf("%s ist kein Stil", parent)
			}
		}
		b := &builder{t: t, used: map[string]bool{}}
		attrs := []template.Attr{{Name: "id", Value: b.id("stil")}}
		if parent != "" {
			attrs = append(attrs, template.Attr{Name: "parent", Value: parent})
		}
		st := template.NewElement("style", attrs...)
		id := st.ID()
		if styles := t.Root.First("styles"); styles != nil {
			return id, d.Apply("Stil anlegen", model.Command{Op: "insert", Parent: model.Ref(styles), Index: -1, XML: fragment(st)})
		}
		wrap := template.NewElement("styles")
		wrap.InsertChild(-1, st)
		idx := 0
		for i, c := range t.Root.Children {
			if c.Kind == template.Element && c.Name == "namespaces" {
				idx = i + 1
			}
		}
		return id, d.Apply("Stil anlegen", model.Command{Op: "insert", Parent: model.Ref(t.Root), Index: idx, XML: fragment(wrap)})
	})
}
