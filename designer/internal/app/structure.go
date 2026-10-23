package app

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/tim-riep/pdx/designer/internal/model"
	"github.com/tim-riep/pdx/template"
)

// ChildInsert describes the insertion of any allowed child,
// for example from the context menu of the hierarchy.
type ChildInsert struct {
	Parent string `json:"parent"`
	// Key is the key of the element type in the catalog (for example formats/locale).
	Key string `json:"key"`
	// Index is the position among the children, -1 chooses the position by the
	// order of the XSD.
	Index int `json:"index"`
}

// notGeneric are elements that have their own ways: images need a
// file from the building block bar.
var notGeneric = map[string]bool{"image": true}

// Insertable returns the children that can still be inserted under an
// element now, in the order of the XSD.
func (s *TemplateService) Insertable(ref string) ([]model.Child, error) {
	cat, err := model.LoadCatalog()
	if err != nil {
		return nil, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	if s.c.doc == nil {
		return nil, errors.New("keine Vorlage geöffnet")
	}
	n, err := model.Resolve(s.c.doc.Pkg.Template, ref)
	if err != nil {
		return nil, err
	}
	out := []model.Child{}
	et := cat.TypeOf(n)
	if et == nil {
		return out, nil
	}
	for _, ch := range et.Children {
		if notGeneric[ch.Key] || cat[ch.Key] == nil {
			continue
		}
		if ch.Max >= 0 && len(n.Elements(ch.Name)) >= ch.Max {
			continue
		}
		out = append(out, ch)
	}
	return out, nil
}

// InsertChild inserts an element with valid required values.
func (s *TemplateService) InsertChild(ci ChildInsert) (Result, error) {
	cat, err := model.LoadCatalog()
	if err != nil {
		return Result{}, err
	}
	return s.change(func(d *model.Document) (string, error) {
		t := d.Pkg.Template
		parent, err := model.Resolve(t, ci.Parent)
		if err != nil {
			return "", err
		}
		pt := cat.TypeOf(parent)
		if pt == nil {
			return "", fmt.Errorf("%s nimmt keine Kinder auf", parent.Name)
		}
		pos := slices.IndexFunc(pt.Children, func(c model.Child) bool { return c.Key == ci.Key })
		if pos < 0 || notGeneric[ci.Key] {
			return "", fmt.Errorf("%s gehört nicht in %s", ci.Key, parent.Name)
		}
		b := &builder{t: t, used: map[string]bool{}}
		el := skeleton(cat, b, t, ci.Key, parent, 0)
		index := ci.Index
		if index < 0 {
			index = sequenceIndex(pt, parent, pos)
		}
		cmd := model.Command{Op: "insert", Parent: model.Ref(parent), Index: index, XML: fragment(el)}
		if err := d.Apply(fmt.Sprintf("%s einfügen", el.Name), cmd); err != nil {
			return "", err
		}
		// Elements without ID (such as formats) are selected via their path.
		return inserted(d, cmd), nil
	})
}

// sequenceIndex returns the position after the last child that comes before
// or at the same position as the new one in the XSD.
func sequenceIndex(pt *model.ElementType, parent *template.Node, pos int) int {
	index := 0
	for i, c := range parent.Children {
		if c.Kind != template.Element {
			continue
		}
		p := slices.IndexFunc(pt.Children, func(ch model.Child) bool { return ch.Name == c.Name })
		if p <= pos {
			index = i + 1
		}
	}
	return index
}

// requiredDefaults are defaults for required attributes that do not follow
// from the XSD, per element type and attribute.
var requiredDefaults = map[string]string{
	"attachments/attachment.name": "daten.xml", "attachments/attachment.source": "data",
	"attachments/attachment.mimeType": "application/xml",
	"locale/pattern.name":             "muster", "locale/pattern.picture": "#.##0,00",
	"lang/string.key": "text", "namespaces/ns.prefix": "ns", "namespaces/ns.uri": "https://example.org/ns",
	"tokens/color.value": "#336699", "tokens/length.value": "4mm",
	"pageMaster/header.height": "20mm", "pageMaster/footer.height": "15mm",
}

// skeleton builds an element with required attributes and required children.
func skeleton(cat model.Catalog, b *builder, t *template.Template, key string, parent *template.Node, depth int) *template.Node {
	et := cat[key]
	name := key[strings.LastIndex(key, "/")+1:]
	el := template.NewElement(name)
	if et == nil {
		return el
	}
	for _, p := range et.Properties {
		switch {
		case p.Name == "id":
			base := name
			if pid := parent.NearestID(); pid != "" && depth > 0 {
				base = pid + "-" + name
			}
			el.SetAttr("id", b.id(base))
		case !p.Required:
		case requiredDefaults[key+"."+p.Name] != "":
			el.SetAttr(p.Name, requiredDefaults[key+"."+p.Name])
		case p.Name == "lang":
			lang := t.Root.Get("lang", "de")
			if name == "lang" {
				lang = strings.SplitN(lang, "-", 2)[0]
			}
			el.SetAttr("lang", lang)
		case p.Name == "layout":
			el.SetAttr("layout", "flowed")
		case p.Default != "":
			el.SetAttr(p.Name, p.Default)
		case len(p.Enum) > 0:
			el.SetAttr(p.Name, p.Enum[0])
		case p.Kind == "xpath":
			el.SetAttr(p.Name, ".")
		case p.Kind == "length":
			el.SetAttr(p.Name, "10mm")
		default:
			el.SetAttr(p.Name, name)
		}
	}
	if name == "string" {
		el.InsertChild(-1, template.NewText("Text"))
	}
	if depth < 4 {
		for _, ch := range et.Children {
			for i := 0; i < ch.Min; i++ {
				el.InsertChild(-1, skeleton(cat, b, t, ch.Key, el, depth+1))
			}
		}
	}
	return el
}

// Duplicate inserts a copy of the element with new IDs directly after it.
func (s *TemplateService) Duplicate(ref string) (Result, error) {
	return s.change(func(d *model.Document) (string, error) {
		t := d.Pkg.Template
		n, err := model.Resolve(t, ref)
		if err != nil {
			return "", err
		}
		if n.Parent == nil {
			return "", errors.New("die Vorlage selbst lässt sich nicht duplizieren")
		}
		b := &builder{t: t, used: map[string]bool{}}
		c := n.Clone()
		var renew func(x *template.Node)
		renew = func(x *template.Node) {
			if id := x.ID(); id != "" {
				x.SetAttr("id", b.id(strings.TrimRight(id, "-0123456789")+"-kopie"))
			}
			for _, ch := range x.Elements("") {
				renew(ch)
			}
		}
		renew(c)
		cmd := model.Command{Op: "insert", Parent: model.Ref(n.Parent), Index: n.Index() + 1, XML: fragment(c)}
		return c.ID(), d.Apply(fmt.Sprintf("%s duplizieren", n.Name), cmd)
	})
}

// Move moves an element among its siblings by delta elements
// forward (negative) or backward. The XSD decides whether the new
// order is valid.
func (s *TemplateService) Move(ref string, delta int) (Result, error) {
	return s.change(func(d *model.Document) (string, error) {
		t := d.Pkg.Template
		n, err := model.Resolve(t, ref)
		if err != nil {
			return "", err
		}
		if n.Parent == nil || delta == 0 {
			return model.Ref(n), nil
		}
		sib := n.Parent.Elements("")
		i := slices.Index(sib, n)
		j := i + delta
		if j < 0 || j >= len(sib) {
			return "", errors.New("weiter geht es nicht")
		}
		// Position among all children (including text) of the target sibling.
		target := sib[j].Index()
		if delta > 0 {
			target++
		}
		if target > n.Index() {
			target-- // the element itself drops out first
		}
		cmd := model.Command{Op: "move", Ref: model.Ref(n), Parent: model.Ref(n.Parent), Index: target}
		if err := d.Apply(map[bool]string{true: "Nach oben", false: "Nach unten"}[delta < 0], cmd); err != nil {
			return "", err
		}
		return model.Ref(n), nil // only after moving, paths without ID change
	})
}
