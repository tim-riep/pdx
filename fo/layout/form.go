package layout

import (
	"strconv"
	"strings"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/fo/props"
	"github.com/tim-riep/pdx/fo/tree"
	"github.com/tim-riep/pdx/font"
)

// Formularfelder kommen als pdx:form-field in fo:instream-foreign-object.
// Das Layout bestimmt Größe und Lage und setzt die Glyphen des
// Erscheinungsbilds, der Renderer schreibt daraus Feld, Widget und Appearance
// Stream.
//
//	<fo:instream-foreign-object content-width="90mm" content-height="7mm">
//	  <pdx:form-field type="text" name="name" label="Name" value="…" context="…"/>
//	</fo:instream-foreign-object>

// widgetArt ist ein Formularfeld als Stück einer Zeile.
type widgetArt struct {
	field areatree.Field
	st    *style
}

// fieldPad ist der Innenabstand zwischen Rahmen und Text, mm eine
// Längeneinheit für die natürlichen Größen.
const (
	fieldPad = 2 * areatree.Pt
	mm       = areatree.Length(2835) // 72000/25.4 Tausendstel Punkt
)

var fieldTypes = map[string]bool{"text": true, "checkbox": true, "radio": true, "list": true, "combo": true, "signature": true}

// formField nimmt ein pdx:form-field auf.
func (pa *paragraph) formField(n, fn *tree.Node, p *props.Computed) error {
	typ := attrOr(fn, "type", "")
	if !fieldTypes[typ] {
		return tree.Errorf(fn, "pdx:form-field: Feldtyp %q ist unbekannt", typ)
	}
	name := attrOr(fn, "name", "")
	if name == "" || strings.ContainsAny(name, ".") {
		return tree.Errorf(fn, "pdx:form-field: name %q fehlt oder enthält einen Punkt", name)
	}
	st, err := pa.style(n, p, nil)
	if err != nil {
		return err
	}
	f := areatree.Field{
		Type: typ, Name: name, Label: attrOr(fn, "label", ""),
		Value: attrOr(fn, "value", ""), Export: attrOr(fn, "export", ""),
		OnValue: attrOr(fn, "on-value", "true"), OffValue: attrOr(fn, "off-value", "false"),
		Multiline: attrOr(fn, "multiline", "") == "true", ReadOnly: attrOr(fn, "read-only", "") == "true",
		Required: attrOr(fn, "required", "") == "true", Editable: attrOr(fn, "editable", "") == "true",
		Context: attrOr(fn, "context", ""),
		Font:    st.fontID, Size: p.FontSize, Color: p.Color,
		Border: p.Border[areatree.Top], Background: p.Background,
	}
	if v := attrOr(fn, "max-length", ""); v != "" {
		if f.MaxLength, err = strconv.Atoi(v); err != nil || f.MaxLength <= 0 {
			return tree.Errorf(fn, "pdx:form-field: max-length %q ist keine positive Zahl", v)
		}
	}
	for _, c := range fn.Children {
		if c.Kind == tree.Element && c.Is(tree.NSPDX, "option") {
			v := attrOr(c, "value", "")
			f.Options = append(f.Options, areatree.FieldOption{Value: v, Label: attrOr(c, "label", v)})
		}
	}
	if typ == "radio" && f.Export == "" {
		return tree.Errorf(fn, "pdx:form-field: Optionsfeld ohne export")
	}
	// Natürliche Größe je Feldtyp, content-width und content-height gehen vor.
	line := p.FontSize * 12 / 10
	iw, ih := 50*mm, line+2*fieldPad
	switch typ {
	case "checkbox", "radio":
		iw, ih = p.FontSize, p.FontSize
	case "combo":
		iw = 40 * mm
	case "list":
		iw, ih = 40*mm, areatree.Length(max(len(f.Options), 1))*line+2*fieldPad
	case "signature":
		iw, ih = 60*mm, 15*mm
	case "text":
		if f.Multiline {
			ih = 4*line + 2*fieldPad
		}
	}
	w, err := fieldLength(n, "content-width", iw, p.FontSize)
	if err != nil {
		return err
	}
	h, err := fieldLength(n, "content-height", ih, p.FontSize)
	if err != nil {
		return err
	}
	pa.pieces = append(pa.pieces, piece{kind: pGraphic, width: w, height: h, st: st,
		widget: &widgetArt{field: f, st: st}})
	return nil
}

// fieldLength liest content-width oder content-height als Länge, ohne Angabe
// gilt die natürliche Größe. Anders als bei Bildern wird nicht skaliert.
func fieldLength(n *tree.Node, name string, def, fontSize areatree.Length) (areatree.Length, error) {
	v, ok := n.Attr(name)
	if !ok || v == "" || v == "auto" {
		return def, nil
	}
	l, err := props.ParseLength(v, fontSize, def)
	if err != nil || l <= 0 {
		return 0, tree.Errorf(n, "%s %q ist für ein Formularfeld ungültig", name, v)
	}
	return l, nil
}

// build liefert das Widget im Rechteck x, y, w, h mit den Glyphen seines
// Erscheinungsbilds.
func (wa *widgetArt) build(x, y, w, h areatree.Length) *areatree.Widget {
	wd := &areatree.Widget{Rect: areatree.Rect{X: x, Y: y, W: w, H: h}, Field: wa.field}
	f := wa.field
	inset := fieldPad
	if f.Border.Visible() {
		inset += f.Border.Width
	}
	line := f.Size * 12 / 10
	st := wa.st
	run := func(text string, top areatree.Length) *areatree.GlyphRun {
		gs, err := st.face.Shape(text, f.Size, font.ShapeOptions{})
		if err != nil || len(gs) == 0 {
			return nil
		}
		return &areatree.GlyphRun{Origin: areatree.Point{X: x + inset, Y: top + st.asc}, Font: st.fontID,
			Size: f.Size, Color: f.Color, Ascent: st.asc, Descent: st.desc, Glyphs: gs}
	}
	add := func(r *areatree.GlyphRun) {
		if r != nil {
			wd.Runs = append(wd.Runs, r)
		}
	}
	// centered ist die Oberkante einer einzelnen, senkrecht zentrierten Zeile.
	centered := y + (h-st.asc-st.desc)/2
	switch f.Type {
	case "text":
		if !f.Multiline {
			add(run(f.Value, centered))
			break
		}
		top := y + inset
		for _, l := range wa.wrap(f.Value, w-2*inset) {
			add(run(l, top))
			top += line
		}
	case "combo":
		add(run(optionLabel(f.Options, f.Value), centered))
	case "list":
		top := y + inset
		for _, o := range f.Options {
			if o.Value == f.Value {
				wd.Selected = &areatree.Rect{X: x + inset/2, Y: top, W: w - inset, H: line}
			}
			add(run(o.Label, top+(line-st.asc-st.desc)/2))
			top += line
		}
	}
	return wd
}

// optionLabel liefert den Text zum Wert, ohne passende Option den Wert.
func optionLabel(opts []areatree.FieldOption, v string) string {
	for _, o := range opts {
		if o.Value == v {
			return o.Label
		}
	}
	return v
}

// wrap bricht Text an Leerzeichen und Zeilenwechseln auf die Breite um.
func (wa *widgetArt) wrap(text string, width areatree.Length) []string {
	var out []string
	adv := func(s string) areatree.Length {
		gs, _ := wa.st.face.Shape(s, wa.field.Size, font.ShapeOptions{})
		var w areatree.Length
		for _, g := range gs {
			w += g.Advance
		}
		return w
	}
	for _, para := range strings.Split(text, "\n") {
		cur := ""
		for _, word := range strings.Fields(para) {
			next := word
			if cur != "" {
				next = cur + " " + word
			}
			if cur != "" && adv(next) > width {
				out = append(out, cur)
				cur = word
				continue
			}
			cur = next
		}
		out = append(out, cur)
	}
	return out
}
