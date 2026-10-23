package render

import (
	"fmt"
	"math"
	"sort"
	"strconv"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/font"
	"github.com/tim-riep/pdx/pdf"
)

// Interactive forms (ISO 32000-1 12.7). Every widget becomes a field,
// merged with its widget dictionary. Radio buttons of a group
// share a parent field. Every widget brings its own appearance, AP
// contains only N (ISO 19005-2 6.3.3), buttons as a dictionary of their
// states. pdx never sets NeedAppearances, pdx does not write actions on
// fields (ISO 19005-2 6.4.1).

// Field flags (ISO 32000-1 Tables 221, 226, 228, 230).
const (
	ffReadOnly      = 1 << 0
	ffRequired      = 1 << 1
	ffMultiline     = 1 << 12
	ffNoToggleToOff = 1 << 14
	ffRadio         = 1 << 15
	ffCombo         = 1 << 17
	ffEdit          = 1 << 18
)

// stateOn is the state of a checked check box.
const stateOn = pdf.Name("Yes")

// formState collects the fields of a document.
type formState struct {
	fields []pdf.Ref
	names  map[string]int
	radios map[string]*radioGroup
	order  []*radioGroup
	dr     map[areatree.FontID]*font.Simple
}

// radioGroup is the parent field of a radio button group.
type radioGroup struct {
	ref  pdf.Ref
	dict *pdf.Dict
	kids pdf.Array
}

func (r *renderer) forms() *formState {
	if r.form == nil {
		r.form = &formState{names: map[string]int{}, radios: map[string]*radioGroup{}, dr: map[areatree.FontID]*font.Simple{}}
	}
	return r.form
}

// fieldName returns a field name that is unique in the document. Repeated
// fields (for example in table rows) get a sequence number.
func (fs *formState) fieldName(name string) string {
	fs.names[name]++
	if n := fs.names[name]; n > 1 {
		return name + "_" + strconv.Itoa(n)
	}
	return name
}

// drFont embeds the default font of a field as a simple font with
// WinAnsiEncoding, once per font.
func (r *renderer) drFont(id areatree.FontID) (pdf.Name, error) {
	fs := r.forms()
	name := pdf.Name("F" + strconv.Itoa(int(id)))
	if _, ok := fs.dr[id]; ok {
		return name, nil
	}
	face, ok := r.doc.Fonts[id].Source.(*font.Face)
	if !ok {
		return "", fmt.Errorf("Schrift %d hat keine ladbare Schriftquelle", id)
	}
	s, err := font.EmbedWinAnsi(r.d, face)
	if err != nil {
		return "", err
	}
	fs.dr[id] = s
	return name, nil
}

// widget writes a form field with widget and appearance, when
// flattening only the appearance as page content.
func (pc *pageCtx) widget(w *areatree.Widget) error {
	if pc.r.opts.Flatten {
		return pc.flatWidget(w)
	}
	r := pc.r
	fs := r.forms()
	f := w.Field
	if w.W <= 0 || w.H <= 0 {
		return fmt.Errorf("Feld %s hat keine Fläche", f.Name)
	}
	annot := pdf.D(
		"Type", pdf.Name("Annot"),
		"Subtype", pdf.Name("Widget"),
		"Rect", pc.box(w.Rect),
		"F", pdf.Int(4),
		"P", r.pageRefs[r.pageIdx],
	)
	if mk := appearanceChars(f); mk.Len() > 0 {
		annot.Set("MK", mk)
	}
	if f.Border.Visible() {
		annot.Set("BS", pdf.D("W", pdf.Real(f.Border.Width.Pt()), "S", pdf.Name(borderStyle(f.Border.Style))))
	}
	flags := 0
	if f.ReadOnly {
		flags |= ffReadOnly
	}
	if f.Required {
		flags |= ffRequired
	}
	// Field attributes: for radio buttons on the parent field, otherwise on the widget.
	field := annot
	switch f.Type {
	case "radio":
		key := f.Name + "\x00" + f.Context
		g, ok := fs.radios[key]
		if !ok {
			g = &radioGroup{ref: r.d.Reserve(), dict: pdf.D("FT", pdf.Name("Btn"), "Ff", pdf.Int(flags|ffRadio|ffNoToggleToOff))}
			fs.radios[key] = g
			fs.order = append(fs.order, g)
			fs.fields = append(fs.fields, g.ref)
			field = g.dict
			g.dict.Set("V", pdf.Name("Off"))
		} else {
			field = nil
		}
		if f.Value == f.Export && f.Export != "" {
			g.dict.Set("V", pdf.Name(f.Export))
		}
		if field != nil {
			pc.fieldCommon(field, f)
		}
		annot.Set("Parent", g.ref)
		ap, err := pc.states(w, pdf.Name(f.Export))
		if err != nil {
			return err
		}
		annot.Set("AP", pdf.D("N", ap))
		state := pdf.Name("Off")
		if f.Value == f.Export {
			state = pdf.Name(f.Export)
		}
		annot.Set("AS", state)
		ref := r.d.Reserve()
		pc.r.tagWidget(w, annot, ref)
		r.d.Set(ref, annot)
		g.kids = append(g.kids, ref)
		pc.annots = append(pc.annots, ref)
		return nil
	case "checkbox":
		annot.Set("FT", pdf.Name("Btn"))
		state := pdf.Name("Off")
		if f.Value == f.OnValue {
			state = stateOn
		}
		annot.Set("V", state)
		annot.Set("AS", state)
		ap, err := pc.states(w, stateOn)
		if err != nil {
			return err
		}
		annot.Set("AP", pdf.D("N", ap))
	case "text", "combo", "list":
		ft := pdf.Name("Tx")
		if f.Type != "text" {
			ft = "Ch"
		}
		annot.Set("FT", ft)
		switch f.Type {
		case "text":
			if f.Multiline {
				flags |= ffMultiline
			}
			if f.MaxLength > 0 {
				annot.Set("MaxLen", pdf.Int(f.MaxLength))
			}
		case "combo":
			flags |= ffCombo
			if f.Editable {
				flags |= ffEdit
			}
		}
		if f.Type != "text" {
			var opt pdf.Array
			for _, o := range f.Options {
				opt = append(opt, pdf.Array{pdf.TextString(o.Value), pdf.TextString(o.Label)})
			}
			annot.Set("Opt", opt)
		}
		annot.Set("V", pdf.TextString(f.Value))
		dr, err := r.drFont(f.Font)
		if err != nil {
			return fmt.Errorf("Feld %s: %w", f.Name, err)
		}
		annot.Set("DA", pdf.String(fmt.Sprintf("/%s %s Tf %s", dr, pdf.FormatNumber(f.Size.Pt()), colorOp(f.Color))))
		ap, err := pc.appearance(w, "")
		if err != nil {
			return err
		}
		annot.Set("AP", pdf.D("N", ap))
	case "signature":
		annot.Set("FT", pdf.Name("Sig"))
		ap, err := pc.appearance(w, "")
		if err != nil {
			return err
		}
		annot.Set("AP", pdf.D("N", ap))
	default:
		return fmt.Errorf("Feld %s: Feldtyp %q ist unbekannt", f.Name, f.Type)
	}
	if flags != 0 {
		annot.Set("Ff", pdf.Int(flags))
	}
	pc.fieldCommon(annot, f)
	ref := r.d.Reserve()
	pc.r.tagWidget(w, annot, ref)
	r.d.Set(ref, annot)
	fs.fields = append(fs.fields, ref)
	pc.annots = append(pc.annots, ref)
	return nil
}

// fieldCommon sets the name, accessible name and export name of a field.
// TM carries the context path and template ID through which pdx extract-data
// writes the input back (ISO 32000-1 Table 220: name for export).
func (pc *pageCtx) fieldCommon(d *pdf.Dict, f areatree.Field) {
	d.Set("T", pdf.TextString(pc.r.forms().fieldName(f.Name)))
	if f.Label != "" {
		d.Set("TU", pdf.TextString(f.Label))
	}
	if f.Context != "" {
		d.Set("TM", pdf.TextString(f.Context+"|"+f.Name))
	}
}

// writeForm attaches AcroForm to the catalog.
func (r *renderer) writeForm() {
	fs := r.form
	if fs == nil || len(fs.fields) == 0 {
		return
	}
	for _, g := range fs.order {
		g.dict.Set("Kids", g.kids)
		r.d.Set(g.ref, g.dict)
	}
	acro := pdf.D("Fields", pdf.Array(refs(fs.fields)))
	if len(fs.dr) > 0 {
		ids := make([]int, 0, len(fs.dr))
		for id := range fs.dr {
			ids = append(ids, int(id))
		}
		sort.Ints(ids)
		fonts := pdf.NewDict()
		for _, id := range ids {
			fonts.Set(pdf.Name("F"+strconv.Itoa(id)), fs.dr[areatree.FontID(id)].Ref)
		}
		acro.Set("DR", pdf.D("Font", fonts))
		acro.Set("DA", pdf.String(fmt.Sprintf("/F%d 0 Tf 0 g", ids[0])))
	}
	r.catalog.Set("AcroForm", acro)
}

func refs(rs []pdf.Ref) []pdf.Object {
	out := make([]pdf.Object, len(rs))
	for i, r := range rs {
		out[i] = r
	}
	return out
}

// appearanceChars returns MK with border and background color so that a
// viewer builds the appearance the same way after an input.
func appearanceChars(f areatree.Field) *pdf.Dict {
	mk := pdf.NewDict()
	if f.Border.Visible() {
		mk.Set("BC", colorArray(f.Border.Color))
	}
	if f.Background != nil {
		mk.Set("BG", colorArray(*f.Background))
	}
	return mk
}

func colorArray(c areatree.Color) pdf.Array {
	switch c.Space {
	case areatree.Gray:
		return pdf.Array{pdf.Real(c.C[0])}
	default:
		return pdf.Array{pdf.Real(c.C[0]), pdf.Real(c.C[1]), pdf.Real(c.C[2])}
	}
}

func colorOp(c areatree.Color) string {
	if c.Space == areatree.Gray {
		return pdf.FormatNumber(c.C[0]) + " g"
	}
	return fmt.Sprintf("%s %s %s rg", pdf.FormatNumber(c.C[0]), pdf.FormatNumber(c.C[1]), pdf.FormatNumber(c.C[2]))
}

func borderStyle(s areatree.BorderStyle) string {
	switch s {
	case areatree.Dashed:
		return "D"
	}
	return "S"
}

// states returns the state dictionary of a button: on and Off.
func (pc *pageCtx) states(w *areatree.Widget, on pdf.Name) (*pdf.Dict, error) {
	if on == "" || on == "Off" {
		return nil, fmt.Errorf("Feld %s: Exportwert %q ist als Zustand nicht erlaubt", w.Field.Name, on)
	}
	onRef, err := pc.appearance(w, "on")
	if err != nil {
		return nil, err
	}
	offRef, err := pc.appearance(w, "off")
	if err != nil {
		return nil, err
	}
	return pdf.D(string(on), onRef, "Off", offRef), nil
}

// appearance draws the appearance of a widget as a form XObject in
// the coordinates of the widget. For buttons, state is on or off.
func (pc *pageCtx) appearance(w *areatree.Widget, state string) (pdf.Ref, error) {
	f := w.Field
	ap := &pageCtx{r: pc.r, res: &pageRes{fonts: pdf.NewDict(), xobjects: pdf.NewDict(), gstates: pdf.NewDict()},
		h: w.H.Pt(), ox: w.X, oy: w.Y, noTag: true}
	c := &ap.c
	if f.Background != nil {
		c.Save()
		if err := ap.fill(*f.Background); err != nil {
			return pdf.Ref{}, err
		}
		ap.rect(w.Rect)
		c.Fill()
		c.Restore()
	}
	// Buttons without a border style get a thin gray outline,
	// otherwise they would be invisible in the off state. Radio buttons are round.
	border := f.Border
	if !border.Visible() && (f.Type == "checkbox" || f.Type == "radio") {
		border = areatree.Border{Width: 750, Style: areatree.Solid, Color: areatree.GrayColor(0.4)}
	}
	if f.Type == "radio" {
		rad := (math.Min(w.W.Pt(), w.H.Pt()) - border.Width.Pt()) / 2
		c.Save()
		if err := ap.stroke(border.Color); err != nil {
			return pdf.Ref{}, err
		}
		c.LineWidth(border.Width.Pt())
		circle(c, w.W.Pt()/2, w.H.Pt()/2, rad)
		c.Stroke()
		c.Restore()
	} else if border.Visible() {
		bw := border.Width
		c.Save()
		if err := ap.stroke(border.Color); err != nil {
			return pdf.Ref{}, err
		}
		c.LineWidth(bw.Pt())
		ap.rect(areatree.Rect{X: w.X + bw/2, Y: w.Y + bw/2, W: w.W - bw, H: w.H - bw})
		c.Stroke()
		c.Restore()
	}
	wd, ht := w.W.Pt(), w.H.Pt()
	switch f.Type {
	case "checkbox":
		if state == "on" {
			// Check mark as a polyline.
			s := math.Min(wd, ht)
			c.Save()
			if err := ap.stroke(f.Color); err != nil {
				return pdf.Ref{}, err
			}
			c.LineWidth(s * 0.12)
			c.MoveTo(wd*0.2, ht*0.52)
			c.LineTo(wd*0.42, ht*0.28)
			c.LineTo(wd*0.8, ht*0.78)
			c.Stroke()
			c.Restore()
		}
	case "radio":
		if state == "on" {
			// Filled circle from four Bézier curves.
			rad := math.Min(wd, ht) * 0.25
			cx, cy := wd/2, ht/2
			c.Save()
			if err := ap.fill(f.Color); err != nil {
				return pdf.Ref{}, err
			}
			circle(c, cx, cy, rad)
			c.Fill()
			c.Restore()
		}
	case "text", "combo", "list":
		// Variable text is in /Tx BMC (ISO 32000-1 12.7.3.3).
		c.BeginTag("Tx")
		c.Save()
		ap.rect(areatree.Rect{X: w.X + 1000, Y: w.Y + 1000, W: w.W - 2000, H: w.H - 2000})
		c.Clip()
		if w.Selected != nil {
			c.Save()
			c.FillRGB(0.6, 0.75, 0.9)
			ap.rect(*w.Selected)
			c.Fill()
			c.Restore()
		}
		for _, run := range w.Runs {
			if err := ap.glyphs(run); err != nil {
				return pdf.Ref{}, err
			}
		}
		c.Restore()
		c.EndMarked()
	}
	resources := pdf.NewDict()
	if ap.res.fonts.Len() > 0 {
		resources.Set("Font", ap.res.fonts)
	}
	return pc.r.d.Add(pdf.NewFlateStream(pdf.D(
		"Type", pdf.Name("XObject"),
		"Subtype", pdf.Name("Form"),
		"BBox", pdf.Array{pdf.Int(0), pdf.Int(0), pdf.Real(wd), pdf.Real(ht)},
		"Resources", resources,
	), c.Bytes())), nil
}

// circle draws a circle from four Bézier curves.
func circle(c *pdf.Content, cx, cy, r float64) {
	k := r * 0.5523
	c.MoveTo(cx+r, cy)
	c.CurveTo(cx+r, cy+k, cx+k, cy+r, cx, cy+r)
	c.CurveTo(cx-k, cy+r, cx-r, cy+k, cx-r, cy)
	c.CurveTo(cx-r, cy-k, cx-k, cy-r, cx, cy-r)
	c.CurveTo(cx+k, cy-r, cx+r, cy-k, cx+r, cy)
	c.ClosePath()
}

// flatWidget draws the appearance of a field in its current state
// as a form XObject into the page content, marked as planned.
func (pc *pageCtx) flatWidget(w *areatree.Widget) error {
	f := w.Field
	state := ""
	switch f.Type {
	case "checkbox":
		state = "off"
		if f.Value == f.OnValue {
			state = "on"
		}
	case "radio":
		state = "off"
		if f.Value == f.Export {
			state = "on"
		}
	}
	ref, err := pc.appearance(w, state)
	if err != nil {
		return err
	}
	pc.r.flat++
	name := pdf.Name("Fm" + strconv.Itoa(pc.r.flat))
	pc.res.xobjects.Set(name, ref)
	if t := pc.r.tag; t != nil && !pc.noTag {
		m := t.marks[w]
		switch {
		case m.artifact:
			pc.c.BeginArtifact()
		case m.actual != "":
			pc.c.BeginMarkedText(m.tag, m.mcid, m.actual)
		default:
			pc.c.BeginMarked(m.tag, m.mcid)
		}
		defer pc.c.EndMarked()
	}
	pc.c.Save()
	pc.c.Transform(1, 0, 0, 1, pc.x(w.X), pc.y(w.Y+w.H))
	pc.c.XObject(name)
	pc.c.Restore()
	return nil
}
