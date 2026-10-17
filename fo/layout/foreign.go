package layout

import (
	"strings"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/barcode"
	"github.com/tim-riep/pdx/fo/props"
	"github.com/tim-riep/pdx/fo/tree"
)

// codeArt ist ein Barcode, der als Pfad aus Rechtecken gezeichnet wird.
type codeArt struct {
	m      *barcode.Matrix
	color  areatree.Color
	source string
}

// foreign nimmt ein fo:instream-foreign-object auf. Unterstützt wird
// pdx:barcode. fo:instream-foreign-object ist in XSL 1.1 (6.6.6) der Ort für
// Inhalte fremder Namespaces. Apache FOP 2.11 erzeugt das PDF ohne den Barcode
// und meldet einen Fehler (SEVERE), siehe testdata/suites/DEVIATIONS.md.
//
//	<fo:instream-foreign-object content-width="30mm">
//	  <pdx:barcode type="girocode" name="…" iban="…" amount="1190.00" text="…"/>
//	</fo:instream-foreign-object>
func (pa *paragraph) foreign(n *tree.Node, parent *props.Computed) error {
	avail := pa.g.contentW(pa.p)
	p, err := props.Compute(parent, n, avail)
	if err != nil {
		return err
	}
	var bn *tree.Node
	for _, c := range n.Children {
		if c.Kind == tree.Element {
			if !c.Is(tree.NSPDX, "barcode") {
				return tree.Errorf(c, "in fo:instream-foreign-object wird nur pdx:barcode unterstützt")
			}
			bn = c
		}
	}
	if bn == nil {
		return tree.Errorf(n, "fo:instream-foreign-object ist leer")
	}
	val := attrOr(bn, "value", "")
	if val == "" {
		var b strings.Builder
		for _, c := range bn.Children {
			if c.Kind == tree.Text {
				b.WriteString(c.Data)
			}
		}
		val = strings.TrimSpace(b.String())
	}
	spec := barcode.Spec{Type: attrOr(bn, "type", ""), Value: val, ECC: attrOr(bn, "ecc", ""),
		Giro: barcode.Giro{
			Name: attrOr(bn, "name", ""), IBAN: attrOr(bn, "iban", ""), BIC: attrOr(bn, "bic", ""),
			Amount: attrOr(bn, "amount", ""), Purpose: attrOr(bn, "purpose", ""),
			Reference: attrOr(bn, "reference", ""), Text: attrOr(bn, "text", ""),
		}}
	m, err := barcode.Generate(spec)
	if err != nil {
		return tree.Errorf(bn, "%v", err)
	}
	// Natürliche Größe aus Modulbreite und Ruhezone.
	defModule := "0.5mm"
	if m.Linear {
		defModule = "0.33mm"
	}
	mw, err := props.EvalLength(attrOr(bn, "module-width", defModule), p.FontSize, -1, nil)
	if err != nil || mw <= 0 {
		return tree.Errorf(bn, "module-width ist ungültig")
	}
	iw := areatree.Length(m.Cols+2*m.Quiet) * mw
	ih := areatree.Length(m.Rows+2*m.Quiet) * mw
	if m.Linear {
		if ih, err = props.EvalLength(attrOr(bn, "bar-height", "15mm"), p.FontSize, -1, nil); err != nil || ih <= 0 {
			return tree.Errorf(bn, "bar-height ist ungültig")
		}
	}
	w, h, err := graphicSize(n, p, iw, ih, avail)
	if err != nil {
		return err
	}
	src := spec.Type
	if spec.Type != "girocode" {
		src += ":" + val
	}
	pa.pieces = append(pa.pieces, piece{kind: pGraphic, width: w, height: h,
		code: &codeArt{m: m, color: p.Color, source: src}})
	return nil
}

// path liefert die Module als Pfad im Rechteck x, y, w, h.
func (c *codeArt) path(x, y, w, h areatree.Length) *areatree.Path {
	m := c.m
	cols := m.Cols + 2*m.Quiet
	rows := m.Rows + 2*m.Quiet
	qy := m.Quiet
	if m.Linear {
		rows, qy = 1, 0
	}
	cw := float64(w) / float64(cols)
	ch := float64(h) / float64(rows)
	p := &areatree.Path{Rect: areatree.Rect{X: x, Y: y, W: w, H: h}, Fill: c.color, Source: c.source}
	for _, r := range m.Rects() {
		x0 := x + areatree.Length(float64(m.Quiet+r[0])*cw)
		x1 := x + areatree.Length(float64(m.Quiet+r[0]+r[2])*cw)
		y0 := y + areatree.Length(float64(qy+r[1])*ch)
		y1 := y + areatree.Length(float64(qy+r[1]+r[3])*ch)
		p.Rects = append(p.Rects, areatree.Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0})
	}
	return p
}
