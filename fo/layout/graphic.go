package layout

import (
	"strings"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/fo/props"
	"github.com/tim-riep/pdx/fo/tree"
)

// srcURI removes url(...) and quotation marks from a src attribute.
func srcURI(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "url(") && strings.HasSuffix(s, ")") {
		s = strings.TrimSpace(s[4 : len(s)-1])
	}
	return strings.Trim(s, `"'`)
}

// graphic takes an fo:external-graphic as an inline box. The baseline
// lies at the bottom edge of the image.
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
	// width and height define the viewport (XSL 1.1 6.6.5), the content
	// lies inside it at the top start (display-align and text-align auto).
	vw, vh := w, h
	if p.Width != nil {
		vw = *p.Width
	}
	if p.Height != nil {
		vh = *p.Height
	}
	// Border and padding belong to the inline area of the image (XSL 1.1
	// 4.2.2). The bottom edge of the border box lies on the baseline.
	pc := piece{kind: pGraphic, img: id, width: vw, height: vh}
	if vw != w || vh != h {
		pc.box = &graphicBox{p: p, n: n, imgW: w, imgH: h}
	}
	if hasDeco(p) {
		pc.box = &graphicBox{p: p, n: n, start: edgeWidth(p, areatree.Left), before: edgeWidth(p, areatree.Top), imgW: w, imgH: h}
		pc.width += edgeWidth(p, areatree.Left) + edgeWidth(p, areatree.Right)
		pc.height += edgeWidth(p, areatree.Top) + edgeWidth(p, areatree.Bottom)
	}
	pa.pieces = append(pa.pieces, pc)
	return nil
}

// graphicBox describes border, padding and background of an image.
type graphicBox struct {
	p             *props.Computed
	n             *tree.Node
	start, before areatree.Length
	// imgW and imgH are the size of the image in the viewport.
	imgW, imgH areatree.Length
}

// graphicSize determines the size from content-width, content-height and
// scaling (XSL 1.1 7.15). Percentages refer to the natural
// size, scale-to-fit to width and height of the viewport.
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
