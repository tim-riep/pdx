package render

import (
	"fmt"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/pdf"
)

// dest liefert ein explizites Ziel /XYZ für eine ID.
func (r *renderer) dest(id string) (pdf.Array, error) {
	d, ok := r.doc.Dests[id]
	if !ok || d.Page < 0 || d.Page >= len(r.pageRefs) {
		return nil, fmt.Errorf("Sprungziel %q fehlt", id)
	}
	h := r.doc.Pages[d.Page].Height.Pt()
	return pdf.Array{r.pageRefs[d.Page], pdf.Name("XYZ"), pdf.Real(d.At.X.Pt()), pdf.Real(h - d.At.Y.Pt()), pdf.Null{}}, nil
}

// link legt eine Link-Annotation an. PDF/A verlangt das Druckflag (/F 4),
// Link-Annotationen brauchen kein Erscheinungsbild (ISO 19005-3 6.3.3).
func (pc *pageCtx) link(l *areatree.Link) error {
	a := pdf.D(
		"Type", pdf.Name("Annot"),
		"Subtype", pdf.Name("Link"),
		"Rect", pc.box(l.Rect),
		"Border", pdf.Array{pdf.Int(0), pdf.Int(0), pdf.Int(0)},
		"F", pdf.Int(4),
	)
	switch {
	case l.URI != "":
		a.Set("A", pdf.D("S", pdf.Name("URI"), "URI", pdf.String(l.URI)))
	case l.Dest != "":
		d, err := pc.r.dest(l.Dest)
		if err != nil {
			return err
		}
		a.Set("Dest", d)
	default:
		return fmt.Errorf("Link ohne Ziel")
	}
	pc.annots = append(pc.annots, pc.r.d.Add(a))
	return nil
}

// outlines schreibt die Lesezeichen als Gliederung (ISO 32000-1 12.3.3).
func (r *renderer) outlines() (pdf.Ref, error) {
	root := r.d.Reserve()
	first, last, count, err := r.outlineItems(r.doc.Bookmarks, root)
	if err != nil {
		return pdf.Ref{}, err
	}
	r.d.Set(root, pdf.D("Type", pdf.Name("Outlines"), "First", first, "Last", last, "Count", pdf.Int(count)))
	return root, nil
}

// outlineItems schreibt eine Ebene und liefert erstes und letztes Element
// sowie die Zahl der sichtbaren Nachfahren.
func (r *renderer) outlineItems(bms []areatree.Bookmark, parent pdf.Ref) (pdf.Ref, pdf.Ref, int, error) {
	refs := make([]pdf.Ref, len(bms))
	for i := range bms {
		refs[i] = r.d.Reserve()
	}
	visible := len(bms)
	for i, bm := range bms {
		item := pdf.D("Title", pdf.TextString(bm.Title), "Parent", parent)
		if i > 0 {
			item.Set("Prev", refs[i-1])
		}
		if i+1 < len(bms) {
			item.Set("Next", refs[i+1])
		}
		switch {
		case bm.URI != "":
			item.Set("A", pdf.D("S", pdf.Name("URI"), "URI", pdf.String(bm.URI)))
		default:
			d, err := r.dest(bm.Dest)
			if err != nil {
				return pdf.Ref{}, pdf.Ref{}, 0, err
			}
			item.Set("Dest", d)
		}
		if len(bm.Children) > 0 {
			f, l, n, err := r.outlineItems(bm.Children, refs[i])
			if err != nil {
				return pdf.Ref{}, pdf.Ref{}, 0, err
			}
			item.Set("First", f)
			item.Set("Last", l)
			if bm.Open {
				item.Set("Count", pdf.Int(n))
				visible += n
			} else {
				item.Set("Count", pdf.Int(-n))
			}
		}
		r.d.Set(refs[i], item)
	}
	return refs[0], refs[len(refs)-1], visible, nil
}
