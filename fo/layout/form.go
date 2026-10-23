package layout

import (
	"strconv"
	"strings"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/fo/props"
	"github.com/tim-riep/pdx/fo/tree"
	"github.com/tim-riep/pdx/font"
)

// Form fields come as pdx:form-field in fo:instream-foreign-object.
// The layout determines size and position and places the glyphs of the
// appearance, the renderer writes field, widget and appearance
// stream from that.
//
//	<fo:instream-foreign-object content-width="90mm" content-height="7mm">
//	  <pdx:form-field type="text" name="name" label="Name" value="…" context="…"/>
//	</fo:instream-foreign-object>

// widgetArt is a form field as a piece of a line.
type widgetArt struct {
	field areatree.Field
	st    *style
}

// fieldPad is the inner spacing between border and text, mm a
// length unit for the natural sizes.
const (
	fieldPad = 2 * areatree.Pt
	mm       = areatree.Length(2835) // 72000/25.4 thousandths of a point
)

var fieldTypes = map[string]bool{"text": true, "checkbox": true, "radio": true, "list": true, "combo": true, "signature": true}

// formField takes a pdx:form-field.
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
	// Natural size per field type, content-width and content-height take precedence.
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

// fieldLength reads content-width or content-height as a length, if not given
// the natural size applies. Unlike images there is no scaling.
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

// build returns the widget in the rectangle x, y, w, h with the glyphs of its
// appearance.
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
	// centered is the top edge of a single, vertically centered line.
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

// optionLabel returns the text for the value, without matching option the value.
func optionLabel(opts []areatree.FieldOption, v string) string {
	for _, o := range opts {
		if o.Value == v {
			return o.Label
		}
	}
	return v
}

// wrap breaks text at spaces and line feeds to the width.
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
