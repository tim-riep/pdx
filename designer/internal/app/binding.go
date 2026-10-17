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

// Drop beschreibt, wohin ein Datenknoten gezogen wurde.
type Drop struct {
	// Path ist der Pfad des Datenknotens im Datenbaum.
	Path string `json:"path"`
	// Parent ist das Vorlagenelement, das den neuen Inhalt aufnimmt.
	Parent string `json:"parent"`
	// Index ist die Position unter den Kindern, -1 hängt an.
	Index int `json:"index"`
	// X und Y sind bei positionierten Eltern die Lage in Millimetern relativ
	// zum Elternelement.
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// positioned meldet, ob die Kinder eines Elements positioniert sind.
func positioned(n *template.Node) bool {
	switch n.Name {
	case "header", "footer":
		return n.Parent != nil && n.Parent.Name == "pageMaster"
	case "subform":
		return n.Get("layout", "") == "positioned"
	}
	return false
}

// buildDrop erzeugt den Befehl für einen gezogenen Datenknoten.
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
	// Namespaces, die die Vorlage noch nicht kennt, werden deklariert.
	if decl := nsCommands(t, s); len(decl) > 0 {
		cmd = model.Command{Op: "batch", Cmds: append(decl, cmd)}
	}
	return cmd, el.ID(), nil
}

// childContext ist der Datenkontext für neue Kinder eines Elements.
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

// builder erzeugt Elemente mit eindeutigen IDs.
type builder struct {
	t       *template.Template
	used    map[string]bool
	formats map[string]string // Art (number, date, dateTime) zu Name des Musters
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

// sanitize macht aus einem Datennamen eine gültige ID.
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

// maxColumns begrenzt die Spalten einer erzeugten Tabelle.
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

// simpleChildren liefert die Kinder mit einfachem Inhalt.
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

// title macht aus einem Datennamen eine Spaltenüberschrift.
func title(name string) string {
	s := sanitize(name)
	if s == "" {
		return name
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// formats liefert je Art das erste Formatmuster der Vorlage.
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

// nsCommands deklariert fehlende Namespaces in der Vorlage.
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

// fragment schreibt ein Element als XML im Vorlagenformat ohne Namespace.
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
