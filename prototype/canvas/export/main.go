// Command export writes the area tree of the sample invoice as compact JSON
// for the canvas prototype, plus the fonts used. Only for the
// phase 2 prototype, not part of the engine.
//
// Call from the root directory: go run ./prototype/canvas/export
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/tim-riep/pdx"
	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/compile"
	"github.com/tim-riep/pdx/dataschema"
	"github.com/tim-riep/pdx/fo/tree"
	"github.com/tim-riep/pdx/font"
	"github.com/tim-riep/pdx/font/fonts"
	"github.com/tim-riep/pdx/template/pkg"
)

type doc struct {
	Fonts []fontRef `json:"fonts"`
	Pages []page    `json:"pages"`
}

type fontRef struct {
	File   string `json:"file"`
	Family string `json:"family"`
}

type page struct {
	N    int     `json:"n"`
	W    float64 `json:"w"`
	H    float64 `json:"h"`
	Draw []any   `json:"draw"`
	// Els are the areas of the page with the ID of their template element, in
	// drawing order. Repeated elements occur several times.
	Els []el `json:"els"`
}

type el struct {
	ID   string     `json:"id"`
	Rect [4]float64 `json:"r"`
}

type rectItem struct {
	K  string     `json:"k"`
	ID string     `json:"id,omitempty"`
	R  [4]float64 `json:"r"`
	Bg string     `json:"bg,omitempty"`
	// B are the borders top, right, bottom, left as [width, color].
	B []any `json:"b,omitempty"`
}

type textItem struct {
	K  string    `json:"k"`
	ID string    `json:"id,omitempty"`
	X  float64   `json:"x"`
	Y  float64   `json:"y"`
	F  int       `json:"f"`
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
}

type pathItem struct {
	K  string       `json:"k"`
	ID string       `json:"id,omitempty"`
	C  string       `json:"c"`
	R  [][4]float64 `json:"r"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "export:", err)
		os.Exit(1)
	}
}

func run() error {
	p, err := pkg.ReadDir("testdata/templates/musterrechnung")
	if err != nil {
		return err
	}
	res, err := compile.Compile(p.Template, compile.Options{Enums: dataschema.PackageEnums(p)})
	if err != nil {
		return err
	}
	data, _ := p.File("samples/rechnung.xml")
	fo, err := res.Transform(data)
	if err != nil {
		return err
	}
	root, err := tree.ParseBytes(fo)
	if err != nil {
		return err
	}
	ids := map[string]string{}
	var walk func(n *tree.Node, inherited string)
	walk = func(n *tree.Node, inherited string) {
		if n.Kind != tree.Element {
			return
		}
		id := inherited
		if v, ok := n.Attrs["{"+tree.NSPDX+"}id"]; ok && v != "" {
			id = v
		}
		ids[n.Label()] = id
		for _, c := range n.Children {
			walk(c, id)
		}
	}
	walk(root, "")

	reg := font.NewRegistry()
	out, err := pdx.Render(bytes.NewReader(fo), pdx.Options{Profile: "pdf-1.7", CreationDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Fonts: reg})
	if err != nil {
		return err
	}
	dir := filepath.Join("prototype", "canvas", "public")
	if err := os.MkdirAll(filepath.Join(dir, "fonts"), 0o755); err != nil {
		return err
	}
	d := doc{}
	files, err := fontFiles()
	if err != nil {
		return err
	}
	for i, f := range out.Tree.Fonts {
		face, ok := f.Source.(*font.Face)
		if !ok {
			return fmt.Errorf("Schrift %d ohne Face", i)
		}
		file, ok := files[face.PostScript]
		if !ok {
			return fmt.Errorf("Schriftdatei für %s fehlt", face.PostScript)
		}
		b, _ := fonts.FS.ReadFile("liberation/" + file)
		if err := os.WriteFile(filepath.Join(dir, "fonts", file), b, 0o644); err != nil {
			return err
		}
		d.Fonts = append(d.Fonts, fontRef{File: file, Family: fmt.Sprintf("pdxf%d", i)})
	}
	for _, pg := range out.Tree.Pages {
		d.Pages = append(d.Pages, convert(pg, ids))
	}
	b, err := json.Marshal(d)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0o755); err != nil {
		return err
	}
	file := filepath.Join(dir, "data", "musterrechnung.json")
	if err := os.WriteFile(file, b, 0o644); err != nil {
		return err
	}
	n := 0
	for _, pg := range d.Pages {
		n += len(pg.Draw)
	}
	fmt.Printf("%s: %d Seiten, %d Zeichenbefehle, %d Bytes\n", file, len(d.Pages), n, len(b))
	return nil
}

// fontFiles maps the embedded Liberation files to their PostScript name.
func fontFiles() (map[string]string, error) {
	entries, err := fonts.FS.ReadDir("liberation")
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".ttf" {
			continue
		}
		b, _ := fonts.FS.ReadFile("liberation/" + e.Name())
		f, err := font.Load(b, 0)
		if err != nil {
			return nil, err
		}
		out[f.PostScript] = e.Name()
	}
	return out, nil
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

func convert(pg *areatree.Page, ids map[string]string) page {
	out := page{N: pg.Number, W: pt(pg.Width), H: pt(pg.Height)}
	seen := map[el]bool{}
	grow := func(id string, r [4]float64) {
		e := el{ID: id, Rect: r}
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
				if v, ok := ids[a.Source]; ok && v != "" {
					cid = v
				}
				r := rect(a.Rect)
				if a.Kind != areatree.KindLine && a.Kind != areatree.KindRegion {
					grow(cid, r)
				}
				it := rectItem{K: "r", ID: cid, R: r}
				if a.Background != nil {
					it.Bg = color(*a.Background)
				}
				visible := it.Bg != ""
				for _, b := range a.Border {
					if b.Visible() {
						it.B = append(it.B, []any{pt(b.Width), color(b.Color)})
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
				t := textItem{K: "t", ID: id, X: pt(a.Origin.X), Y: pt(a.Origin.Y), F: int(a.Font), S: pt(a.Size), C: color(a.Color)}
				for _, g := range a.Glyphs {
					t.T = append(t.T, g.Text)
					t.A = append(t.A, pt(g.Advance))
				}
				out.Draw = append(out.Draw, t)
			case *areatree.Line:
				out.Draw = append(out.Draw, lineItem{K: "l", ID: id, P: [4]float64{pt(a.From.X), pt(a.From.Y), pt(a.To.X), pt(a.To.Y)}, W: pt(a.Width), C: color(a.Color)})
			case *areatree.Box:
				it := rectItem{K: "r", ID: id, R: rect(a.Rect)}
				if a.Fill != nil {
					it.Bg = color(*a.Fill)
				}
				if a.Stroke != nil && a.Stroke.Visible() {
					s := []any{pt(a.Stroke.Width), color(a.Stroke.Color)}
					it.B = []any{s, s, s, s}
				}
				out.Draw = append(out.Draw, it)
				grow(id, it.R)
			case *areatree.Path:
				it := pathItem{K: "p", ID: id, C: color(a.Fill)}
				for _, r := range a.Rects {
					it.R = append(it.R, rect(r))
				}
				out.Draw = append(out.Draw, it)
				grow(id, rect(a.Rect))
			case *areatree.Image:
				out.Draw = append(out.Draw, rectItem{K: "r", ID: id, R: rect(a.Rect), Bg: "#dddddd"})
				grow(id, rect(a.Rect))
			}
		}
	}
	visit(pg.Areas, "")
	return out
}
