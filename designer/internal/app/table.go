package app

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/tim-riep/pdx/designer/internal/model"
	"github.com/tim-riep/pdx/designer/internal/schema"
	"github.com/tim-riep/pdx/template"
)

// TableOp is a change to the structure of a table.
//
//	addColumn     Ref (table or cell), Index  insert column, -1 appends
//	removeColumn  Ref (table or cell), Index  remove column
//	addRow        Ref (section or row), Index  insert row
//	addSection    Ref (table), Kind header or footer
//	span          Ref (cell), Count  cell spans Count columns
//	sum           Ref (cell), Kind subtotal, carryOver or total
type TableOp struct {
	Op    string `json:"op"`
	Ref   string `json:"ref"`
	Index int    `json:"index"`
	Kind  string `json:"kind"`
	Count int    `json:"count"`
}

// Table changes the structure of a table as one step.
func (s *TemplateService) Table(op TableOp) (Result, error) {
	return s.change(func(d *model.Document) (string, error) {
		t := d.Pkg.Template
		n, err := model.Resolve(t, op.Ref)
		if err != nil {
			return "", err
		}
		b := &builder{t: t, used: map[string]bool{}, formats: formats(t)}
		sc, _ := s.c.schemaLocked()
		b.numeric = numericCheck(t, sc)
		cmd, sel, label, err := tableCommand(b, n, op)
		if err != nil {
			return "", err
		}
		if err := d.Apply(label, cmd); err != nil {
			return "", err
		}
		return sel, nil
	})
}

// tableOf returns the table for an element inside it.
func tableOf(n *template.Node) *template.Node {
	for x := n; x != nil; x = x.Parent {
		if x.Name == "table" {
			return x
		}
	}
	return nil
}

// rowsOf returns all rows of a table in header, group, body and footer.
func rowsOf(tbl *template.Node) []*template.Node {
	var out []*template.Node
	var walk func(n *template.Node)
	walk = func(n *template.Node) {
		for _, c := range n.Elements("") {
			switch c.Name {
			case "row":
				out = append(out, c)
			case "header", "footer", "rows", "group":
				walk(c)
			}
		}
	}
	walk(tbl)
	return out
}

func colspan(c *template.Node) int {
	n, err := strconv.Atoi(c.Get("colspan", "1"))
	if err != nil || n < 1 {
		return 1
	}
	return n
}

// cellAt returns the cell of a row that covers column col (from 0),
// and its first column.
func cellAt(row *template.Node, col int) (*template.Node, int) {
	pos := 0
	for _, c := range row.Elements("cell") {
		span := colspan(c)
		if col < pos+span {
			return c, pos
		}
		pos += span
	}
	return nil, pos
}

// columnOf returns the first column of a cell.
func columnOf(cell *template.Node) int {
	pos := 0
	for _, c := range cell.Parent.Elements("cell") {
		if c == cell {
			return pos
		}
		pos += colspan(c)
	}
	return pos
}

func (b *builder) cell(base string) *template.Node {
	return template.NewElement("cell", template.Attr{Name: "id", Value: b.id(base)})
}

func tableCommand(b *builder, n *template.Node, op TableOp) (model.Command, string, string, error) {
	tbl := tableOf(n)
	if tbl == nil {
		return model.Command{}, "", "", errors.New("das Element liegt in keiner Tabelle")
	}
	cols := tbl.Elements("column")
	col := op.Index
	if n.Name == "cell" && (op.Op == "addColumn" || op.Op == "removeColumn") {
		col = columnOf(n)
		if op.Op == "addColumn" {
			col += colspan(n)
		}
	}
	var cmds []model.Command
	switch op.Op {
	case "addColumn":
		if col < 0 || col > len(cols) {
			col = len(cols)
		}
		id := b.id(tbl.ID() + "-sp")
		// Column elements are at the start of the table.
		cmds = append(cmds, model.Command{Op: "insert", Parent: model.Ref(tbl), Index: col,
			XML: fragment(template.NewElement("column", template.Attr{Name: "id", Value: id}, template.Attr{Name: "width", Value: "1*"}))})
		for _, row := range rowsOf(tbl) {
			c, start := cellAt(row, col)
			switch {
			case c == nil:
				cmds = append(cmds, model.Command{Op: "insert", Parent: model.Ref(row), Index: -1, XML: fragment(b.cell(row.ID() + "-z"))})
			case start < col:
				// In the middle of a spanning cell: it becomes wider.
				cmds = append(cmds, model.Command{Op: "setAttr", Ref: model.Ref(c), Name: "colspan", Value: strconv.Itoa(colspan(c) + 1)})
			default:
				cmds = append(cmds, model.Command{Op: "insert", Parent: model.Ref(row), Index: c.Index(), XML: fragment(b.cell(row.ID() + "-z"))})
			}
		}
		return model.Command{Op: "batch", Cmds: cmds}, id, "Spalte einfügen", nil
	case "removeColumn":
		if len(cols) <= 1 {
			return model.Command{}, "", "", errors.New("eine Tabelle braucht mindestens eine Spalte")
		}
		if col < 0 || col >= len(cols) {
			col = len(cols) - 1
		}
		for _, row := range rowsOf(tbl) {
			c, _ := cellAt(row, col)
			switch {
			case c == nil:
			case colspan(c) > 1:
				v := ""
				if colspan(c) > 2 {
					v = strconv.Itoa(colspan(c) - 1)
				}
				cmds = append(cmds, model.Command{Op: "setAttr", Ref: model.Ref(c), Name: "colspan", Value: v})
			default:
				cmds = append(cmds, model.Command{Op: "remove", Ref: model.Ref(c)})
			}
		}
		cmds = append(cmds, model.Command{Op: "remove", Ref: model.Ref(cols[col])})
		return model.Command{Op: "batch", Cmds: cmds}, model.Ref(tbl), "Spalte entfernen", nil
	case "addRow":
		section := n
		index := op.Index
		if n.Name == "row" {
			section, index = n.Parent, n.Index()+1
		}
		if section.Name == "cell" {
			row := section.Parent
			section, index = row.Parent, row.Index()+1
		}
		if section.Name != "header" && section.Name != "footer" && section.Name != "rows" {
			return model.Command{}, "", "", fmt.Errorf("Zeilen gehören in Kopf, Fuß oder Zeilen der Tabelle, nicht in %s", section.Name)
		}
		row := template.NewElement("row", template.Attr{Name: "id", Value: b.id(section.ID() + "-zeile")})
		for range cols {
			row.InsertChild(-1, b.cell(row.ID()+"-z"))
		}
		return model.Command{Op: "insert", Parent: model.Ref(section), Index: index, XML: fragment(row)}, row.ID(), "Zeile einfügen", nil
	case "addSection":
		if op.Kind != "header" && op.Kind != "footer" {
			return model.Command{}, "", "", fmt.Errorf("Abschnitt %q unbekannt", op.Kind)
		}
		if tbl.First(op.Kind) != nil {
			return model.Command{}, "", "", fmt.Errorf("die Tabelle hat schon einen %s", map[string]string{"header": "Kopf", "footer": "Fuß"}[op.Kind])
		}
		sec := template.NewElement(op.Kind, template.Attr{Name: "id", Value: b.id(tbl.ID() + "-" + map[string]string{"header": "kopf", "footer": "fuss"}[op.Kind])},
			template.Attr{Name: "repeat", Value: "true"})
		row := template.NewElement("row", template.Attr{Name: "id", Value: b.id(sec.ID() + "-zeile")})
		for range cols {
			row.InsertChild(-1, b.cell(row.ID()+"-z"))
		}
		sec.InsertChild(-1, row)
		// Header after the columns, footer at the end.
		index := -1
		if op.Kind == "header" {
			index = len(cols)
		}
		return model.Command{Op: "insert", Parent: model.Ref(tbl), Index: index, XML: fragment(sec)}, sec.ID(),
			map[string]string{"header": "Tabellenkopf einfügen", "footer": "Tabellenfuß einfügen"}[op.Kind], nil
	case "span":
		if n.Name != "cell" {
			return model.Command{}, "", "", errors.New("verbinden lassen sich nur Zellen")
		}
		want := op.Count
		if want < 1 {
			want = 1
		}
		have := colspan(n)
		row := n.Parent
		if want > have {
			// Following cells merge into the joined one.
			need := want - have
			for i := n.Index() + 1; i < len(row.Children) && need > 0; i++ {
				c := row.Children[i]
				if c.Kind != template.Element || c.Name != "cell" {
					continue
				}
				if colspan(c) > need {
					return model.Command{}, "", "", errors.New("die folgende Zelle ist breiter als die Lücke")
				}
				need -= colspan(c)
				cmds = append(cmds, model.Command{Op: "remove", Ref: model.Ref(c)})
			}
			if need > 0 {
				return model.Command{}, "", "", errors.New("rechts fehlen Zellen zum Verbinden")
			}
		} else {
			for k := 0; k < have-want; k++ {
				cmds = append(cmds, model.Command{Op: "insert", Parent: model.Ref(row), Index: n.Index() + 1, XML: fragment(b.cell(row.ID() + "-z"))})
			}
		}
		v := strconv.Itoa(want)
		if want == 1 {
			v = ""
		}
		cmds = append([]model.Command{{Op: "setAttr", Ref: model.Ref(n), Name: "colspan", Value: v}}, cmds...)
		return model.Command{Op: "batch", Cmds: cmds}, model.Ref(n), "Zellen verbinden", nil
	case "sum":
		if n.Name != "cell" {
			return model.Command{}, "", "", errors.New("Summen stehen in Zellen")
		}
		if op.Kind != "subtotal" && op.Kind != "carryOver" && op.Kind != "total" {
			return model.Command{}, "", "", fmt.Errorf("Summe %q unbekannt", op.Kind)
		}
		of, format := sumSource(tbl, columnOf(n), b.numeric)
		if of == "" {
			return model.Command{}, "", "", errors.New("in dieser Spalte steht kein gebundenes Feld, das sich summieren lässt")
		}
		sum := template.NewElement(op.Kind, template.Attr{Name: "id", Value: b.id(tbl.ID() + "-" + map[string]string{"subtotal": "zwischensumme", "carryOver": "uebertrag", "total": "summe"}[op.Kind])},
			template.Attr{Name: "of", Value: of})
		if format != "" {
			sum.SetAttr("format", format)
		}
		return model.Command{Op: "insert", Parent: model.Ref(n), Index: -1, XML: fragment(sum)}, sum.ID(),
			map[string]string{"subtotal": "Zwischensumme einfügen", "carryOver": "Übertrag einfügen", "total": "Summe einfügen"}[op.Kind], nil
	}
	return model.Command{}, "", "", fmt.Errorf("Tabellenbefehl %q unbekannt", op.Op)
}

// sumSource returns the binding and format of the field in column col of the
// repeated rows, if it is a number.
func sumSource(tbl *template.Node, col int, numeric func(*template.Node) bool) (string, string) {
	rows := tbl.First("rows")
	if rows == nil {
		return "", ""
	}
	for _, row := range rows.Elements("row") {
		if c, _ := cellAt(row, col); c != nil {
			if f := c.First("field"); f != nil && numeric(f) {
				return f.Get("bind", ""), f.Get("format", "")
			}
		}
	}
	return "", ""
}

// numericCheck checks whether a field is bound to a number: via the
// data XSD, without XSD via a number format.
func numericCheck(t *template.Template, sc *schema.Schema) func(*template.Node) bool {
	numberFormats := map[string]bool{}
	if f := t.Root.First("formats"); f != nil {
		for _, loc := range f.Elements("locale") {
			for _, p := range loc.Elements("pattern") {
				if p.Get("kind", "") == "number" {
					numberFormats[p.Get("name", "")] = true
				}
			}
		}
	}
	return func(f *template.Node) bool {
		if sc != nil {
			if ctx, err := schema.Context(t, f); err == nil {
				if n := sc.Find(joinPath(ctx, f.Get("bind", ""))); n != nil {
					switch n.Type {
					case "decimal", "double", "float", "integer", "int", "long", "short", "nonNegativeInteger", "positiveInteger":
						return true
					}
					return false
				}
			}
		}
		return numberFormats[f.Get("format", "")]
	}
}

// joinPath joins a context with a relative binding.
func joinPath(ctx, rel string) string {
	if rel == "." || rel == "" {
		return ctx
	}
	steps := strings.Split(ctx, "/")
	for _, p := range strings.Split(rel, "/") {
		switch p {
		case ".", "":
		case "..":
			if len(steps) > 0 {
				steps = steps[:len(steps)-1]
			}
		default:
			steps = append(steps, p)
		}
	}
	return strings.Join(steps, "/")
}
