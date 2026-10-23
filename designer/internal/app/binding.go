package app

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/tim-riep/pdx/designer/internal/model"
	"github.com/tim-riep/pdx/designer/internal/schema"
	"github.com/tim-riep/pdx/template"
)

// Drop describes where a data node was dragged to.
type Drop struct {
	// Path is the path of the data node in the data tree.
	Path string `json:"path"`
	// Parent is the template element that receives the new content.
	Parent string `json:"parent"`
	// Index is the position among the children, -1 appends.
	Index int `json:"index"`
	// X and Y are the position in millimeters relative to the parent element
	// when the parent is positioned.
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// positioned reports whether the children of an element are positioned.
func positioned(n *template.Node) bool {
	switch n.Name {
	case "header", "footer":
		return n.Parent != nil && n.Parent.Name == "pageMaster"
	case "subform":
		return n.Get("layout", "") == "positioned"
	}
	return false
}

// buildDrop creates the command for a dragged data node.
func buildDrop(t *template.Template, s *schema.Schema, drop Drop) (model.Command, string, error) {
	if s == nil {
		return model.Command{}, "", errors.New("die Vorlage hat kein Daten-XSD")
	}
	node := s.Find(drop.Path)
	if node == nil {
		return model.Command{}, "", fmt.Errorf("Datenknoten %s nicht gefunden", drop.Path)
	}
	parent, err := model.Resolve(t, drop.Parent)
	if err != nil {
		return model.Command{}, "", err
	}
	ctx, err := childContext(t, parent)
	if err != nil {
		return model.Command{}, "", err
	}
	b := &builder{t: t, used: map[string]bool{}, formats: formats(t)}
	rel := schema.Relative(ctx, node.Path)
	var el *template.Node
	switch {
	case parent.Name == "text" || parent.Name == "span":
		if node.Type == "" {
			return model.Command{}, "", errors.New("in einen Text lassen sich nur einfache Werte einfügen")
		}
		el = b.field(node, rel)
	case node.Repeated() && !node.Attr:
		el = b.table(node, rel)
	case node.Type == "" && !node.Attr:
		el = b.subform(node, rel)
	default:
		el = b.field(node, rel)
	}
	if node.Min == 0 && !node.Repeated() && parent.Name != "text" && parent.Name != "span" {
		el.SetAttr("presence", rel)
	}
	top := el
	if positioned(parent) {
		if !hasPlacement(el) {
			top = template.NewElement("subform", template.Attr{Name: "id", Value: b.id(el.ID() + "-bereich")},
				template.Attr{Name: "layout", Value: "flowed"}, template.Attr{Name: "w", Value: "120mm"})
			top.InsertChild(-1, el)
		}
		top.SetAttr("x", mm(drop.X))
		top.SetAttr("y", mm(drop.Y))
		if _, ok := top.Attr("w"); !ok {
			top.SetAttr("w", "60mm")
		}
	}
	xml := fragment(top)
	cmd := model.Command{Op: "insert", Parent: drop.Parent, Index: drop.Index, XML: xml}
	// Namespaces the template does not know yet are declared.
	if decl := nsCommands(t, s); len(decl) > 0 {
		cmd = model.Command{Op: "batch", Cmds: append(decl, cmd)}
	}
	return cmd, el.ID(), nil
}

// childContext is the data context for new children of an element.
func childContext(t *template.Template, parent *template.Node) (string, error) {
	probe := template.NewElement("probe")
	parent.InsertChild(-1, probe)
	defer parent.RemoveChild(probe)
	return schema.Context(t, probe)
}

func mm(v float64) string {
	if v < 0 {
		v = 0
	}
	return strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%.1f", v), "0"), ".") + "mm"
}

// builder creates elements with unique IDs.
type builder struct {
	t       *template.Template
	used    map[string]bool
	formats map[string]string // kind (number, date, dateTime) to name of the pattern
	numeric func(*template.Node) bool
}

func (b *builder) id(base string) string {
	base = sanitize(base)
	id := base
	for i := 2; b.t.ByID(id) != nil || b.used[id]; i++ {
		id = fmt.Sprintf("%s-%d", base, i)
	}
	b.used[id] = true
	return id
}

// sanitize turns a data name into a valid ID.
func sanitize(s string) string {
	if i := strings.LastIndex(s, ":"); i >= 0 {
		s = s[i+1:]
	}
	s = strings.TrimPrefix(s, "@")
	var out strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			out.WriteRune(r)
		} else {
			out.WriteRune('-')
		}
	}
	res := out.String()
	if res == "" || !(res[0] >= 'a' && res[0] <= 'z' || res[0] >= 'A' && res[0] <= 'Z' || res[0] == '_') {
		res = "f-" + res
	}
	return res
}

func (b *builder) field(n *schema.Node, bind string) *template.Node {
	f := template.NewElement("field", template.Attr{Name: "id", Value: b.id(n.Name)}, template.Attr{Name: "bind", Value: bind})
	switch n.Type {
	case "date":
		if p := b.formats["date"]; p != "" {
			f.SetAttr("format", p)
		}
	case "dateTime":
		if p := b.formats["dateTime"]; p != "" {
			f.SetAttr("format", p)
		}
	case "decimal", "double", "float":
		if p := b.formats["number"]; p != "" {
			f.SetAttr("format", p)
		}
	}
	return f
}

func (b *builder) subform(n *schema.Node, bind string) *template.Node {
	sf := template.NewElement("subform", template.Attr{Name: "id", Value: b.id(n.Name)},
		template.Attr{Name: "bind", Value: bind}, template.Attr{Name: "layout", Value: "flowed"})
	for _, c := range simpleChildren(n) {
		sf.InsertChild(-1, b.field(c, localStep(c)))
	}
	return sf
}

// maxColumns limits the columns of a generated table.
const maxColumns = 8

func (b *builder) table(n *schema.Node, bind string) *template.Node {
	id := b.id(n.Name)
	tbl := template.NewElement("table", template.Attr{Name: "id", Value: id}, template.Attr{Name: "bind", Value: bind})
	cols := simpleChildren(n)
	if len(cols) > maxColumns {
		cols = cols[:maxColumns]
	}
	if len(cols) == 0 {
		cols = []*schema.Node{n}
	}
	for range cols {
		tbl.InsertChild(-1, template.NewElement("column", template.Attr{Name: "id", Value: b.id(id + "-sp")}, template.Attr{Name: "width", Value: "1*"}))
	}
	head := template.NewElement("header", template.Attr{Name: "id", Value: b.id(id + "-kopf")}, template.Attr{Name: "repeat", Value: "true"})
	hrow := template.NewElement("row", template.Attr{Name: "id", Value: b.id(id + "-kopfzeile")})
	rows := template.NewElement("rows", template.Attr{Name: "id", Value: b.id(id + "-zeilen")})
	row := template.NewElement("row", template.Attr{Name: "id", Value: b.id(id + "-zeile")})
	for _, c := range cols {
		hc := template.NewElement("cell", template.Attr{Name: "id", Value: b.id(id + "-k-" + sanitize(c.Name))})
		txt := template.NewElement("text", template.Attr{Name: "id", Value: b.id(id + "-kt-" + sanitize(c.Name))})
		txt.InsertChild(-1, template.NewText(title(c.Name)))
		hc.InsertChild(-1, txt)
		hrow.InsertChild(-1, hc)
		cell := template.NewElement("cell", template.Attr{Name: "id", Value: b.id(id + "-z-" + sanitize(c.Name))})
		bind := localStep(c)
		if c == n {
			bind = "."
		}
		cell.InsertChild(-1, b.field(c, bind))
		row.InsertChild(-1, cell)
	}
	head.InsertChild(-1, hrow)
	rows.InsertChild(-1, row)
	tbl.InsertChild(-1, head)
	tbl.InsertChild(-1, rows)
	return tbl
}

// simpleChildren returns the children with simple content.
func simpleChildren(n *schema.Node) []*schema.Node {
	var out []*schema.Node
	for _, c := range n.Children {
		if c.Type != "" && !c.Repeated() {
			out = append(out, c)
		}
	}
	return out
}

func localStep(n *schema.Node) string {
	if i := strings.LastIndex(n.Path, "/"); i >= 0 {
		return n.Path[i+1:]
	}
	return n.Path
}

// title turns a data name into a column heading.
func title(name string) string {
	s := sanitize(name)
	if s == "" {
		return name
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// formats returns the first format pattern of the template for each kind.
func formats(t *template.Template) map[string]string {
	out := map[string]string{}
	f := t.Root.First("formats")
	if f == nil {
		return out
	}
	for _, loc := range f.Elements("locale") {
		for _, p := range loc.Elements("pattern") {
			if k := p.Get("kind", ""); out[k] == "" {
				out[k] = p.Get("name", "")
			}
		}
	}
	return out
}

// nsCommands declares missing namespaces in the template.
func nsCommands(t *template.Template, s *schema.Schema) []model.Command {
	if len(s.Missing) == 0 {
		return nil
	}
	uris := make([]string, 0, len(s.Missing))
	for u := range s.Missing {
		uris = append(uris, u)
	}
	sort.Strings(uris)
	var xml strings.Builder
	for _, u := range uris {
		xml.WriteString(fragment(template.NewElement("ns", template.Attr{Name: "prefix", Value: s.Missing[u]}, template.Attr{Name: "uri", Value: u})))
	}
	if ns := t.Root.First("namespaces"); ns != nil {
		return []model.Command{{Op: "insert", Parent: model.Ref(ns), Index: -1, XML: xml.String()}}
	}
	return []model.Command{{Op: "insert", Parent: model.Ref(t.Root), Index: 0, XML: "<namespaces>" + xml.String() + "</namespaces>"}}
}

// fragment writes an element as XML in the template format without namespace.
func fragment(n *template.Node) string {
	root := template.NewElement("template")
	root.InsertChild(-1, n)
	out := string(template.Write(&template.Template{Root: root}))
	root.RemoveChild(n)
	open := strings.Index(out, "<template")
	if open < 0 {
		return ""
	}
	start := open + strings.Index(out[open:], ">") + 1
	end := strings.LastIndex(out, "</template>")
	if end < start {
		return ""
	}
	return strings.TrimSpace(out[start:end])
}
