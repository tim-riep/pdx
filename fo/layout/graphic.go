package layout

import (
	"strings"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/fo/props"
	"github.com/tim-riep/pdx/fo/tree"
)

// srcURI entfernt url(...) und Anführungszeichen aus einem src-Attribut.
func srcURI(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "url(") && strings.HasSuffix(s, ")") {
		s = strings.TrimSpace(s[4 : len(s)-1])
	}
	return strings.Trim(s, `"'`)
}

// graphic nimmt ein fo:external-graphic als Inline-Box auf. Die Grundlinie
// liegt an der Unterkante des Bildes.
func (pa *paragraph) graphic(n *tree.Node, parent *props.Computed) error {
	avail := pa.g.contentW(pa.p)
	p, err := props.Compute(parent, n, avail)
	if err != nil {
		return err
	}
	src, ok := n.Attr("src")
	if !ok || srcURI(src) == "" {
		return tree.Errorf(n, "src fehlt")
	}
	id, im, err := pa.g.e.imageID(n, srcURI(src))
	if err != nil {
		return err
	}
	iw, ih := im.IntrinsicSize()
	w, h, err := graphicSize(n, p, iw, ih, avail)
	if err != nil {
		return err
	}
	// Rahmen und Padding gehören zum Inline-Bereich des Bildes (XSL 1.1
	// 4.2.2). Die Unterkante der Rahmenbox liegt auf der Grundlinie.
	pc := piece{kind: pGraphic, img: id, width: w, height: h}
	if hasDeco(p) {
		pc.box = &graphicBox{p: p, n: n, start: edgeWidth(p, areatree.Left), before: edgeWidth(p, areatree.Top)}
		pc.width += edgeWidth(p, areatree.Left) + edgeWidth(p, areatree.Right)
		pc.height += edgeWidth(p, areatree.Top) + edgeWidth(p, areatree.Bottom)
	}
	pa.pieces = append(pa.pieces, pc)
	return nil
}

// graphicBox beschreibt Rahmen, Padding und Hintergrund eines Bildes.
type graphicBox struct {
	p             *props.Computed
	n             *tree.Node
	start, before areatree.Length
}

// graphicSize bestimmt die Größe nach content-width, content-height und
// scaling (XSL 1.1 7.15). Prozentangaben beziehen sich auf die natürliche
// Größe, scale-to-fit auf width und height des Viewports.
func graphicSize(n *tree.Node, p *props.Computed, iw, ih, avail areatree.Length) (areatree.Length, areatree.Length, error) {
	if iw <= 0 || ih <= 0 {
		return 0, 0, tree.Errorf(n, "Bild hat keine Größe")
	}
	vw, vh := avail, areatree.Length(0)
	if p.Width != nil {
		vw = *p.Width
	}
	if p.Height != nil {
		vh = *p.Height
	}
	parse := func(name string, intrinsic, viewport areatree.Length) (areatree.Length, string, error) {
		v, ok := n.Attr(name)
		if !ok || v == "auto" {
			return 0, "auto", nil
		}
		switch v {
		case "scale-to-fit", "scale-down-to-fit", "scale-up-to-fit":
			return viewport, v, nil
		}
		l, err := props.ParseLength(v, p.FontSize, intrinsic)
		if err != nil {
			return 0, "", tree.Errorf(n, "%s: %v", name, err)
		}
		return l, "length", nil
	}
	cw, cwMode, err := parse("content-width", iw, vw)
	if err != nil {
		return 0, 0, err
	}
	ch, chMode, err := parse("content-height", ih, vh)
	if err != nil {
		return 0, 0, err
	}
	if chMode != "auto" && chMode != "length" && vh == 0 {
		chMode = "auto"
	}
	uniform := true
	if v, _ := n.Attr("scaling"); v == "non-uniform" {
		uniform = false
	}
	sx, sy := 1.0, 1.0
	if cwMode != "auto" {
		sx = float64(cw) / float64(iw)
	}
	if chMode != "auto" {
		sy = float64(ch) / float64(ih)
	}
	switch {
	case cwMode == "auto" && chMode == "auto":
	case cwMode == "auto":
		sx = sy
	case chMode == "auto":
		sy = sx
	case uniform:
		s := min(sx, sy)
		sx, sy = s, s
	}
	clamp := func(mode string, s float64) float64 {
		switch mode {
		case "scale-down-to-fit":
			return min(s, 1)
		case "scale-up-to-fit":
			return max(s, 1)
		}
		return s
	}
	sx, sy = clamp(cwMode, sx), clamp(chMode, sy)
	return areatree.Length(float64(iw) * sx), areatree.Length(float64(ih) * sy), nil
}
