package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tim-riep/pdx/designer/internal/model"
	"github.com/tim-riep/pdx/template"
)

// Building blocks of the bar above the canvas.
const (
	KindText       = "text"
	KindPositioned = "positioned"
	KindFlowed     = "flowed"
	KindLine       = "line"
	KindRect       = "rect"
	KindImage      = "image"
	KindBarcode    = "barcode"
	KindPageNumber = "pageNumber"
	KindTable      = "table"
	// Form fields.
	KindTextField      = "textField"
	KindCheckBox       = "checkBox"
	KindRadioGroup     = "radioGroup"
	KindComboBox       = "comboBox"
	KindListBox        = "listBox"
	KindSignatureField = "signatureField"
)

// Insert describes the insertion of a building block.
type Insert struct {
	Kind   string  `json:"kind"`
	Parent string  `json:"parent"`
	Index  int     `json:"index"`
	X      float64 `json:"x"` // mm, only for positioned parents
	Y      float64 `json:"y"`
	// File is the path of the image file for images.
	File string `json:"file"`
}

// InsertElement inserts a building block and selects it.
func (s *TemplateService) InsertElement(in Insert) (Result, error) {
	return s.change(func(d *model.Document) (string, error) {
		t := d.Pkg.Template
		parent, err := model.Resolve(t, in.Parent)
		if err != nil {
			return "", err
		}
		b := &builder{t: t, used: map[string]bool{}}
		var cmds []model.Command
		el, err := b.element(in, parent)
		if err != nil {
			return "", err
		}
		if in.Kind == KindImage {
			data, err := os.ReadFile(in.File)
			if err != nil {
				return "", fmt.Errorf("Bild %s nicht lesbar: %v", in.File, err)
			}
			name := "assets/" + sanitize(strings.TrimSuffix(filepath.Base(in.File), filepath.Ext(in.File))) + strings.ToLower(filepath.Ext(in.File))
			if old, ok := d.Pkg.Files[name]; !ok || string(old) != string(data) {
				cmds = append(cmds, model.Command{Op: "setFile", Name: name, Data: data})
			}
			el.SetAttr("src", name)
		}
		top := el
		if positioned(parent) {
			// Elements without placement attributes (such as tables) are placed in a
			// flowing subform with placement.
			if !hasPlacement(el) {
				top = template.NewElement("subform", template.Attr{Name: "id", Value: b.id(el.ID() + "-bereich")},
					template.Attr{Name: "layout", Value: "flowed"}, template.Attr{Name: "w", Value: "120mm"})
				top.InsertChild(-1, el)
			}
			top.SetAttr("x", mm(in.X))
			top.SetAttr("y", mm(in.Y))
		}
		cmds = append(cmds, model.Command{Op: "insert", Parent: in.Parent, Index: in.Index, XML: fragment(top)})
		cmd := cmds[0]
		if len(cmds) > 1 {
			cmd = model.Command{Op: "batch", Cmds: cmds}
		}
		if err := d.Apply(elementLabels[in.Kind]+" einfügen", cmd); err != nil {
			return "", err
		}
		return el.ID(), nil
	})
}

var elementLabels = map[string]string{
	KindText: "Text", KindPositioned: "Teilformular", KindFlowed: "Teilformular", KindLine: "Linie",
	KindRect: "Rechteck", KindImage: "Bild", KindBarcode: "Barcode", KindPageNumber: "Seitenzahl", KindTable: "Tabelle",
	KindTextField: "Textfeld", KindCheckBox: "Kontrollkästchen", KindRadioGroup: "Optionsfelder",
	KindComboBox: "Kombinationsfeld", KindListBox: "Auswahlliste", KindSignatureField: "Signaturfeld",
}

// fieldBorder is the border of new form fields.
const fieldBorder = "0.5pt solid #555555"

// element builds the building block with sensible defaults.
func (b *builder) element(in Insert, parent *template.Node) (*template.Node, error) {
	inPositioned := positioned(parent)
	attr := func(n, v string) template.Attr { return template.Attr{Name: n, Value: v} }
	var el *template.Node
	switch in.Kind {
	case KindText:
		el = template.NewElement("text", attr("id", b.id("text")))
		el.InsertChild(-1, template.NewText("Text"))
		if inPositioned {
			el.SetAttr("w", "60mm")
		}
	case KindPositioned:
		el = template.NewElement("subform", attr("id", b.id("teilformular")), attr("layout", "positioned"),
			attr("w", "80mm"), attr("h", "30mm"))
	case KindFlowed:
		el = template.NewElement("subform", attr("id", b.id("teilformular")), attr("layout", "flowed"))
		if inPositioned {
			el.SetAttr("w", "80mm")
		}
	case KindLine:
		el = template.NewElement("line", attr("id", b.id("linie")), attr("w", "60mm"))
	case KindRect:
		el = template.NewElement("rect", attr("id", b.id("rechteck")), attr("w", "40mm"), attr("h", "20mm"), attr("border", "0.5pt solid #000000"))
	case KindImage:
		el = template.NewElement("image", attr("id", b.id("bild")), attr("w", "40mm"))
	case KindBarcode:
		el = template.NewElement("barcode", attr("id", b.id("barcode")), attr("type", "qr"), attr("value", "Beispiel"), attr("w", "25mm"))
	case KindPageNumber:
		el = template.NewElement("pageNumber", attr("id", b.id("seitenzahl")), attr("pattern", "Seite {current} von {total}"))
		if inPositioned {
			el.SetAttr("w", "60mm")
		}
	case KindTable:
		id := b.id("tabelle")
		el = template.NewElement("table", attr("id", id), attr("bind", "."))
		head := template.NewElement("header", attr("id", b.id(id+"-kopf")), attr("repeat", "true"))
		hrow := template.NewElement("row", attr("id", b.id(id+"-kopfzeile")))
		rows := template.NewElement("rows", attr("id", b.id(id+"-zeilen")))
		row := template.NewElement("row", attr("id", b.id(id+"-zeile")))
		for i := 1; i <= 3; i++ {
			el.InsertChild(-1, template.NewElement("column", attr("id", b.id(id+"-sp")), attr("width", "1*")))
			hc := template.NewElement("cell", attr("id", b.id(fmt.Sprintf("%s-k%d", id, i))))
			ht := template.NewElement("text", attr("id", b.id(fmt.Sprintf("%s-kt%d", id, i))))
			ht.InsertChild(-1, template.NewText(fmt.Sprintf("Spalte %d", i)))
			hc.InsertChild(-1, ht)
			hrow.InsertChild(-1, hc)
			c := template.NewElement("cell", attr("id", b.id(fmt.Sprintf("%s-z%d", id, i))))
			row.InsertChild(-1, c)
		}
		head.InsertChild(-1, hrow)
		rows.InsertChild(-1, row)
		el.InsertChild(-1, head)
		el.InsertChild(-1, rows)
	case KindTextField:
		el = template.NewElement("textField", attr("id", b.id("textfeld")), attr("bind", "."), attr("label", "Textfeld"),
			attr("w", "60mm"), attr("h", "7mm"), attr("border", fieldBorder))
	case KindCheckBox:
		el = template.NewElement("checkBox", attr("id", b.id("kontrollkaestchen")), attr("bind", "."), attr("label", "Kontrollkästchen"),
			attr("w", "5mm"), attr("h", "5mm"), attr("border", fieldBorder))
	case KindRadioGroup, KindComboBox, KindListBox:
		name := map[string][2]string{KindRadioGroup: {"radioGroup", "optionen"}, KindComboBox: {"comboBox", "kombifeld"}, KindListBox: {"listBox", "liste"}}[in.Kind]
		id := b.id(name[1])
		el = template.NewElement(name[0], attr("id", id), attr("bind", "."), attr("label", elementLabels[in.Kind]))
		switch in.Kind {
		case KindComboBox:
			el.SetAttr("w", "40mm")
			el.SetAttr("h", "7mm")
			el.SetAttr("border", fieldBorder)
		case KindListBox:
			el.SetAttr("w", "40mm")
			el.SetAttr("h", "16mm")
			el.SetAttr("border", fieldBorder)
		}
		for i, v := range []string{"A", "B"} {
			el.InsertChild(-1, template.NewElement("option", attr("id", b.id(fmt.Sprintf("%s-o%d", id, i+1))), attr("value", v), attr("label", "Option "+v)))
		}
	case KindSignatureField:
		el = template.NewElement("signatureField", attr("id", b.id("signatur")), attr("label", "Unterschrift"),
			attr("w", "60mm"), attr("h", "15mm"), attr("border", fieldBorder))
	default:
		return nil, fmt.Errorf("Baustein %q unbekannt", in.Kind)
	}
	return el, nil
}

// hasPlacement reports whether an element has placement attributes (x, y).
func hasPlacement(n *template.Node) bool {
	cat, err := model.LoadCatalog()
	if err != nil {
		return false
	}
	if et := cat[n.Name]; et != nil {
		_, ok := et.Property("x")
		return ok
	}
	return false
}
