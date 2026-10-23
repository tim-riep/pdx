package pipeline

import (
	"fmt"
	"math"

	"github.com/tim-riep/pdx/areatree"
)

// Page is a page in the compact format for the canvas. All sizes in points,
// origin top left. Every entry carries the template ID, so that click and
// hover lead back to the template element.
type Page struct {
	N    int     `json:"n"`
	W    float64 `json:"w"`
	H    float64 `json:"h"`
	Draw []any   `json:"draw"`
	// Els are the areas with template ID in drawing order, for
	// hit testing, selection and guides. Repeated elements occur
	// several times.
	Els []El `json:"els"`
	// Fields are the form fields of the page for the fillable preview.
	Fields []FieldEl `json:"fields"`
}

// FieldEl is a form field for the preview.
type FieldEl struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Rect      [4]float64 `json:"r"`
	Type      string     `json:"type"`
	Value     string     `json:"value"`
	Export    string     `json:"export,omitempty"`
	On        string     `json:"on,omitempty"`
	Label     string     `json:"label,omitempty"`
	Multiline bool       `json:"multiline,omitempty"`
	MaxLength int        `json:"maxLength,omitempty"`
	ReadOnly  bool       `json:"readOnly,omitempty"`
	Options   []Option   `json:"options,omitempty"`
}

// Option is a choice value of a field.
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// widgetItem draws checkboxes and radio buttons: T is checkbox
// or radio, On the state.
type widgetItem struct {
	K  string     `json:"k"`
	ID string     `json:"id,omitempty"`
	R  [4]float64 `json:"r"`
	T  string     `json:"t"`
	On bool       `json:"on"`
	C  string     `json:"c"`
}

// El is an area of a template element.
type El struct {
	ID   string     `json:"id"`
	Rect [4]float64 `json:"r"`
	// Kind is block, inline, text, image or code.
	Kind string `json:"k"`
}

type rectItem struct {
	K  string     `json:"k"`
	ID string     `json:"id,omitempty"`
	R  [4]float64 `json:"r"`
	Bg string     `json:"bg,omitempty"`
	// B are the borders top, right, bottom, left as [width, color, style]
	// or 0. They lie outside the rectangle.
	B []any `json:"b,omitempty"`
}

type textItem struct {
	K  string    `json:"k"`
	ID string    `json:"id,omitempty"`
	X  float64   `json:"x"`
	Y  float64   `json:"y"`
	F  string    `json:"f"` // key of the font
	S  float64   `json:"s"`
	C  string    `json:"c"`
	T  []string  `json:"t"`
	A  []float64 `json:"a"`
}

type lineItem struct {
	K  string     `json:"k"`
	ID string     `json:"id,omitempty"`
	P  [4]float64 `json:"p"`
	W  float64    `json:"w"`
	C  string     `json:"c"`
	St string     `json:"st,omitempty"`
}

type pathItem struct {
	K  string       `json:"k"`
	ID string       `json:"id,omitempty"`
	C  string       `json:"c"`
	R  [][4]float64 `json:"r"`
}

type imageItem struct {
	K  string     `json:"k"`
	ID string     `json:"id,omitempty"`
	R  [4]float64 `json:"r"`
	I  string     `json:"i"` // key of the image
}

func pt(l areatree.Length) float64 { return math.Round(float64(l)/10) / 100 }

func rect(r areatree.Rect) [4]float64 { return [4]float64{pt(r.X), pt(r.Y), pt(r.W), pt(r.H)} }

func color(c areatree.Color) string {
	var r, g, b float64
	switch c.Space {
	case areatree.Gray:
		r, g, b = c.C[0], c.C[0], c.C[0]
	case areatree.RGB:
		r, g, b = c.C[0], c.C[1], c.C[2]
	case areatree.CMYK:
		k := c.C[3]
		r, g, b = (1-c.C[0])*(1-k), (1-c.C[1])*(1-k), (1-c.C[2])*(1-k)
	}
	return fmt.Sprintf("#%02x%02x%02x", int(math.Round(r*255)), int(math.Round(g*255)), int(math.Round(b*255)))
}

// styleName returns the style of a line, empty for solid.
func styleName(s areatree.BorderStyle) string {
	if s == areatree.Solid {
		return ""
	}
	return s.String()
}

// convert translates a page. fontKey and imageKey return the keys
// of the fonts and images of the document.
func convert(pg *areatree.Page, fontKey func(areatree.FontID) string, imageKey func(areatree.ImageID) string) Page {
	out := Page{N: pg.Number, W: pt(pg.Width), H: pt(pg.Height), Draw: []any{}, Els: []El{}, Fields: []FieldEl{}}
	seen := map[El]bool{}
	el := func(id, kind string, r [4]float64) {
		e := El{ID: id, Rect: r, Kind: kind}
		if id == "" || r[2] <= 0 && r[3] <= 0 || seen[e] {
			return
		}
		seen[e] = true
		out.Els = append(out.Els, e)
	}
	var visit func(as []areatree.Area, id string)
	visit = func(as []areatree.Area, id string) {
		for _, a := range as {
			switch a := a.(type) {
			case *areatree.Container:
				cid := id
				if a.ID != "" {
					cid = a.ID
				}
				r := rect(a.Rect)
				switch a.Kind {
				case areatree.KindBlock, areatree.KindViewport:
					el(cid, "block", r)
				case areatree.KindInline:
					el(cid, "inline", r)
				}
				it := rectItem{K: "r", ID: cid, R: r}
				visible := false
				if a.Background != nil {
					it.Bg = color(*a.Background)
					visible = true
				}
				for _, b := range a.Border {
					if b.Visible() {
						it.B = append(it.B, []any{pt(b.Width), color(b.Color), styleName(b.Style)})
						visible = true
					} else {
						it.B = append(it.B, 0)
					}
				}
				if !visible {
					it.B = nil
				}
				if visible {
					out.Draw = append(out.Draw, it)
				}
				visit(a.Children, cid)
			case *areatree.GlyphRun:
				rid := id
				if a.ID != "" {
					rid = a.ID
					el(rid, "text", rect(a.Bounds()))
				}
				t := textItem{K: "t", ID: rid, X: pt(a.Origin.X), Y: pt(a.Origin.Y), F: fontKey(a.Font), S: pt(a.Size), C: color(a.Color)}
				for _, g := range a.Glyphs {
					t.T = append(t.T, g.Text)
					t.A = append(t.A, pt(g.Advance))
				}
				out.Draw = append(out.Draw, t)
			case *areatree.Line:
				out.Draw = append(out.Draw, lineItem{K: "l", ID: id, P: [4]float64{pt(a.From.X), pt(a.From.Y), pt(a.To.X), pt(a.To.Y)},
					W: pt(a.Width), C: color(a.Color), St: styleName(a.Style)})
			case *areatree.Box:
				it := rectItem{K: "r", ID: id, R: rect(a.Rect)}
				if a.Fill != nil {
					it.Bg = color(*a.Fill)
				}
				if a.Stroke != nil && a.Stroke.Visible() {
					s := []any{pt(a.Stroke.Width), color(a.Stroke.Color), styleName(a.Stroke.Style)}
					it.B = []any{s, s, s, s}
				}
				out.Draw = append(out.Draw, it)
			case *areatree.Path:
				pid := id
				if a.ID != "" {
					pid = a.ID
				}
				it := pathItem{K: "p", ID: pid, C: color(a.Fill)}
				for _, r := range a.Rects {
					it.R = append(it.R, rect(r))
				}
				out.Draw = append(out.Draw, it)
				el(pid, "code", rect(a.Rect))
			case *areatree.Widget:
				f := a.Field
				r := rect(a.Rect)
				it := rectItem{K: "r", ID: a.ID, R: r}
				if f.Background != nil {
					it.Bg = color(*f.Background)
				}
				b := f.Border
				if !b.Visible() && (f.Type == "checkbox" || f.Type == "radio") {
					b = areatree.Border{Width: 750, Style: areatree.Solid, Color: areatree.GrayColor(0.4)}
				}
				if b.Visible() && f.Type != "radio" {
					// The canvas draws borders outside the rectangle, the
					// appearance in the PDF inside.
					bw := pt(b.Width)
					it.R = [4]float64{r[0] + bw, r[1] + bw, r[2] - 2*bw, r[3] - 2*bw}
					s := []any{bw, color(b.Color), styleName(b.Style)}
					it.B = []any{s, s, s, s}
				}
				if it.Bg != "" || it.B != nil {
					out.Draw = append(out.Draw, it)
				}
				switch f.Type {
				case "checkbox":
					out.Draw = append(out.Draw, widgetItem{K: "w", ID: a.ID, R: r, T: "checkbox", On: f.Value == f.OnValue, C: color(f.Color)})
				case "radio":
					out.Draw = append(out.Draw, widgetItem{K: "w", ID: a.ID, R: r, T: "radio", On: f.Value == f.Export, C: color(f.Color)})
				}
				if a.Selected != nil {
					out.Draw = append(out.Draw, rectItem{K: "r", ID: a.ID, R: rect(*a.Selected), Bg: "#99bfe6"})
				}
				visit(runsOf(a), a.ID)
				el(a.ID, "field", r)
				fe := FieldEl{ID: a.ID, Name: f.Name, Rect: r, Type: f.Type, Value: f.Value, Label: f.Label,
					Multiline: f.Multiline, MaxLength: f.MaxLength, ReadOnly: f.ReadOnly}
				switch f.Type {
				case "radio":
					fe.Export = f.Export
				case "checkbox":
					fe.On = f.OnValue
				}
				for _, o := range f.Options {
					fe.Options = append(fe.Options, Option{Value: o.Value, Label: o.Label})
				}
				out.Fields = append(out.Fields, fe)
			case *areatree.Image:
				iid := id
				if a.ID != "" {
					iid = a.ID
				}
				out.Draw = append(out.Draw, imageItem{K: "i", ID: iid, R: rect(a.Rect), I: imageKey(a.Image)})
				el(iid, "image", rect(a.Rect))
			}
		}
	}
	visit(pg.Areas, "")
	return out
}

// runsOf returns the glyph runs of a widget as areas.
func runsOf(w *areatree.Widget) []areatree.Area {
	out := make([]areatree.Area, len(w.Runs))
	for i, r := range w.Runs {
		out[i] = r
	}
	return out
}
