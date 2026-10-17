package app

import (
	"fmt"
	"strings"

	"github.com/tim-riep/pdx/designer/internal/model"
	"github.com/tim-riep/pdx/template"
)

// Segment ist ein Stück gemischten Inhalts: Text (ein Zeilenumbruch wird zu
// br), ein vorhandenes eingebettetes Element (Ref) oder ein neues Feld
// (Bind, relativ zum Kontext des Elements).
type Segment struct {
	Text string `json:"text"`
	Ref  string `json:"ref"`
	Bind string `json:"bind"`
}

// ContentEdit ersetzt den Inhalt eines Elements mit gemischtem Inhalt.
type ContentEdit struct {
	Ref      string    `json:"ref"`
	Segments []Segment `json:"segments"`
}

// SetContent ersetzt Text und eingebettete Elemente als ein Schritt.
// Eingebettete Elemente, die nicht mehr vorkommen, entfallen.
func (s *TemplateService) SetContent(ce ContentEdit) (Result, error) {
	cat, err := model.LoadCatalog()
	if err != nil {
		return Result{}, err
	}
	return s.change(func(d *model.Document) (string, error) {
		t := d.Pkg.Template
		n, err := model.Resolve(t, ce.Ref)
		if err != nil {
			return "", err
		}
		et := cat.TypeOf(n)
		if et == nil || !et.Text {
			return "", fmt.Errorf("%s hat keinen Text", n.Name)
		}
		// Nur Text, wenn das Element keine Kinder kennt (etwa string).
		textOnly := !et.Mixed || len(et.Children) == 0
		hasBr := false
		for _, c := range et.Children {
			hasBr = hasBr || c.Name == "br"
		}
		b := &builder{t: t, used: map[string]bool{}}
		var xml strings.Builder
		for _, seg := range ce.Segments {
			switch {
			case seg.Ref != "":
				if textOnly {
					return "", fmt.Errorf("%s nimmt nur Text auf", n.Name)
				}
				c, err := model.Resolve(t, seg.Ref)
				if err != nil || c.Parent != n {
					return "", fmt.Errorf("%s steht nicht in diesem Text", seg.Ref)
				}
				xml.WriteString(fragment(c.Clone()))
			case seg.Bind != "":
				if textOnly {
					return "", fmt.Errorf("%s nimmt nur Text auf", n.Name)
				}
				f := template.NewElement("field", template.Attr{Name: "id", Value: b.id(n.NearestID() + "-" + sanitize(lastStep(seg.Bind)))},
					template.Attr{Name: "bind", Value: seg.Bind})
				xml.WriteString(fragment(f))
			default:
				lines := strings.Split(seg.Text, "\n")
				for i, line := range lines {
					if i > 0 {
						if !hasBr {
							xml.WriteString(" ")
						} else {
							xml.WriteString(fragment(template.NewElement("br", template.Attr{Name: "id", Value: b.id(n.NearestID() + "-umbruch")})))
						}
					}
					xml.WriteString(escapeText(line))
				}
			}
		}
		cmd := model.Command{Op: "content", Ref: model.Ref(n), XML: xml.String()}
		if err := d.Apply("Text ändern", cmd); err != nil {
			return "", err
		}
		return model.Ref(n), nil
	})
}

func lastStep(path string) string {
	path = strings.TrimRight(path, "/")
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

func escapeText(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}
