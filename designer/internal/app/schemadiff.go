package app

import (
	"errors"
	"fmt"
	"maps"
	"sort"
	"strings"

	"github.com/tim-riep/pdx/designer/internal/model"
	"github.com/tim-riep/pdx/designer/internal/schema"
	"github.com/tim-riep/pdx/designer/internal/xpath"
	"github.com/tim-riep/pdx/template"
	"github.com/tim-riep/pdx/template/pkg"
)

// SchemaChange ist eine Änderung zwischen altem und neuem Daten-XSD.
type SchemaChange struct {
	// Kind ist removed, added, changed oder renamed.
	Kind string `json:"kind"`
	Path string `json:"path"`
	// To ist bei renamed der neue Pfad.
	To string `json:"to"`
	// Detail beschreibt changed, etwa „Typ decimal statt string“.
	Detail string `json:"detail"`
	// Required meldet bei added einen Pflichtknoten.
	Required bool `json:"required"`
	// Refs sind die Vorlagenelemente, die den Knoten oder etwas darunter
	// verwenden.
	Refs []string `json:"refs"`
}

// SchemaDiff vergleicht das Daten-XSD der Vorlage mit einem neuen.
type SchemaDiff struct {
	Main    string         `json:"main"`
	Changes []SchemaChange `json:"changes"`
}

// Rename stellt Bindungen von einem Datenknoten auf einen anderen um.
type Rename struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// Diff liest ein neues Daten-XSD und vergleicht es mit dem der Vorlage,
// ohne etwas zu ändern. Für entfernte Knoten schlägt es Umbenennungen vor,
// wenn genau ein neuer Knoten gleicher Art passt.
func (s *SchemaService) Diff(path string) (SchemaDiff, error) {
	s.c.mu.Lock()
	if s.c.doc == nil {
		s.c.mu.Unlock()
		return SchemaDiff{}, errors.New("keine Vorlage geöffnet")
	}
	t := s.c.doc.Pkg.Template
	oldSc, err := s.c.schemaLocked()
	files := maps.Clone(s.c.doc.Pkg.Files)
	canon := template.Write(t)
	s.c.mu.Unlock()
	if err != nil {
		return SchemaDiff{}, err
	}
	if oldSc == nil {
		return SchemaDiff{}, errors.New("die Vorlage hat noch kein Daten-XSD, es lässt sich direkt laden")
	}
	x, err := readXSD(path, rootLocal(t))
	if err != nil {
		return SchemaDiff{}, err
	}
	tc, err := template.ParseTree(canon)
	if err != nil {
		return SchemaDiff{}, err
	}
	newSc, err := x.schemaFor(tc, files)
	if err != nil {
		return SchemaDiff{}, err
	}
	cat, err := model.LoadCatalog()
	if err != nil {
		return SchemaDiff{}, err
	}
	return SchemaDiff{Main: x.main, Changes: compareSchemas(oldSc, newSc, usage(cat, tc, oldSc))}, nil
}

// Replace lädt ein neues Daten-XSD, entfernt alte XSD-Dateien und stellt
// einfache Bindungen auf umbenannte Knoten um, alles als ein Schritt.
// Ausdrücke, die mehr als ein Pfad sind, bleiben für die Aufgabenliste.
func (s *SchemaService) Replace(path string, renames []Rename) (Result, error) {
	s.c.mu.Lock()
	if s.c.doc == nil {
		s.c.mu.Unlock()
		return Result{}, errors.New("keine Vorlage geöffnet")
	}
	root := rootLocal(s.c.doc.Pkg.Template)
	oldSc, err := s.c.schemaLocked()
	s.c.mu.Unlock()
	if err != nil {
		return Result{}, err
	}
	x, err := readXSD(path, root)
	if err != nil {
		return Result{}, err
	}
	cat, err := model.LoadCatalog()
	if err != nil {
		return Result{}, err
	}
	return NewTemplateService(s.c).change(func(d *model.Document) (string, error) {
		t := d.Pkg.Template
		var cmds []model.Command
		for _, name := range d.Pkg.Names() {
			if _, keep := x.files[name]; !keep && strings.HasPrefix(name, "schema/") && strings.HasSuffix(name, ".xsd") {
				cmds = append(cmds, model.Command{Op: "setFile", Name: name})
			}
		}
		cmds = append(cmds, x.commands(t)...)
		if oldSc != nil {
			prefix, _ := ensurePrefix(t, x.root.Name.Space)
			cmds = append(cmds, rewriteBindings(cat, t, oldSc, x.rootName(prefix), renames)...)
		}
		return "", d.Apply("Daten-XSD ersetzen", model.Command{Op: "batch", Cmds: cmds})
	})
}

func rootLocal(t *template.Template) string {
	r := t.Root.Get("dataRoot", "")
	return r[strings.LastIndex(r, ":")+1:]
}

// schemaFor baut den Datenbaum, den die Vorlage mit dem neuen XSD hätte.
func (x *xsdImport) schemaFor(t *template.Template, files map[string][]byte) (*schema.Schema, error) {
	f := map[string][]byte{}
	for name, b := range files {
		if !strings.HasPrefix(name, "schema/") {
			f[name] = b
		}
	}
	maps.Copy(f, x.files)
	prefix, nsCmd := ensurePrefix(t, x.root.Name.Space)
	if nsCmd != nil {
		ns := t.Root.First("namespaces")
		if ns == nil {
			ns = template.NewElement("namespaces")
			t.Root.InsertChild(0, ns)
		}
		ns.InsertChild(-1, template.NewElement("ns", template.Attr{Name: "prefix", Value: prefix}, template.Attr{Name: "uri", Value: x.root.Name.Space}))
	}
	t.Root.SetAttr("schema", x.main)
	t.Root.SetAttr("dataRoot", x.rootName(prefix))
	t.Reindex()
	return schema.Load(&pkg.Package{Template: t, Files: f})
}

// eachXPath ruft f für jedes XPath-Attribut mit dem Datenkontext seines
// Elements, in Dokumentreihenfolge.
func eachXPath(cat model.Catalog, t *template.Template, f func(n *template.Node, attr, expr, ctx string)) {
	var walk func(n *template.Node)
	walk = func(n *template.Node) {
		if et := cat.TypeOf(n); et != nil {
			for _, p := range et.Properties {
				if p.Kind != "xpath" {
					continue
				}
				if v, ok := n.Attr(p.Name); ok && v != "" {
					if ctx, err := schema.Context(t, n); err == nil {
						f(n, p.Name, v, ctx)
					}
				}
			}
		}
		for _, c := range n.Elements("") {
			walk(c)
		}
	}
	walk(t.Root)
}

// usage ordnet Datenknoten die Vorlagenelemente zu, die sie verwenden.
func usage(cat model.Catalog, t *template.Template, sc *schema.Schema) map[string][]string {
	out := map[string][]string{}
	eachXPath(cat, t, func(n *template.Node, _, expr, ctx string) {
		for _, p := range xpath.Targets(sc, ctx, expr) {
			id := n.NearestID()
			if l := out[p]; len(l) == 0 || l[len(l)-1] != id {
				out[p] = append(l, id)
			}
		}
	})
	return out
}

func parentPath(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[:i]
	}
	return ""
}

func localName(p string) string {
	p = p[strings.LastIndex(p, "/")+1:]
	return p[strings.LastIndex(p, ":")+1:]
}

// compareSchemas vergleicht zwei Datenbäume. Entfernte und neue Teilbäume
// erscheinen nur mit ihrer Wurzel.
func compareSchemas(oldSc, newSc *schema.Schema, used map[string][]string) []SchemaChange {
	oldIdx, newIdx := map[string]*schema.Node{}, map[string]*schema.Node{}
	var oldOrder, newOrder []string
	for _, n := range oldSc.Nodes() {
		oldIdx[n.Path] = n
		oldOrder = append(oldOrder, n.Path)
	}
	for _, n := range newSc.Nodes() {
		newIdx[n.Path] = n
		newOrder = append(newOrder, n.Path)
	}
	refs := func(p string) []string {
		seen := map[string]bool{}
		out := []string{}
		for target, ids := range used {
			if target == p || strings.HasPrefix(target, p+"/") {
				for _, id := range ids {
					if !seen[id] {
						seen[id] = true
						out = append(out, id)
					}
				}
			}
		}
		sort.Strings(out)
		return out
	}
	var removed, added []string
	var changes []SchemaChange
	for _, p := range oldOrder {
		if _, ok := newIdx[p]; ok {
			o, n := oldIdx[p], newIdx[p]
			var d []string
			if o.Type != n.Type {
				d = append(d, fmt.Sprintf("Typ %s statt %s", typeName(n.Type), typeName(o.Type)))
			}
			if o.Repeated() != n.Repeated() {
				d = append(d, map[bool]string{true: "jetzt wiederholt", false: "nicht mehr wiederholt"}[n.Repeated()])
			}
			if (o.Min == 0) != (n.Min == 0) {
				d = append(d, map[bool]string{true: "jetzt optional", false: "jetzt Pflicht"}[n.Min == 0])
			}
			if len(d) > 0 {
				changes = append(changes, SchemaChange{Kind: "changed", Path: p, Detail: strings.Join(d, ", "), Refs: refs(p)})
			}
			continue
		}
		if parent := parentPath(p); parent == "" || newIdx[parent] != nil {
			removed = append(removed, p)
		}
	}
	for _, p := range newOrder {
		if _, ok := oldIdx[p]; !ok {
			if parent := parentPath(p); parent == "" || oldIdx[parent] != nil {
				added = append(added, p)
			}
		}
	}
	// Umbenennungen: genau ein neuer Knoten gleicher Art unter demselben
	// Eltern oder mit demselben Namen woanders.
	claimed := map[string]bool{}
	var out []SchemaChange
	for _, r := range removed {
		o := oldIdx[r]
		var cand []string
		for _, a := range added {
			n := newIdx[a]
			if claimed[a] || n.Type != o.Type || n.Attr != o.Attr || n.Repeated() != o.Repeated() {
				continue
			}
			if parentPath(a) == parentPath(r) || localName(a) == localName(r) {
				cand = append(cand, a)
			}
		}
		if len(cand) == 1 {
			claimed[cand[0]] = true
			out = append(out, SchemaChange{Kind: "renamed", Path: r, To: cand[0], Refs: refs(r)})
			continue
		}
		out = append(out, SchemaChange{Kind: "removed", Path: r, Refs: refs(r)})
	}
	out = append(out, changes...)
	for _, a := range added {
		if !claimed[a] {
			out = append(out, SchemaChange{Kind: "added", Path: a, Required: newIdx[a].Min > 0, Refs: []string{}})
		}
	}
	return out
}

func typeName(t string) string {
	if t == "" {
		return "Gruppe"
	}
	return t
}

// rewriteBindings stellt einfache Pfade auf umbenannte Knoten um und passt
// relative Pfade an, deren Kontext sich dadurch ändert.
func rewriteBindings(cat model.Catalog, t *template.Template, oldSc *schema.Schema, newRoot string, renames []Rename) []model.Command {
	mapPath := func(p string) string {
		for _, r := range renames {
			if p == r.From || strings.HasPrefix(p, r.From+"/") {
				return r.To + p[len(r.From):]
			}
		}
		return p
	}
	// Neue Bindungen von Teilformularen und Tabellen bestimmen den neuen
	// Kontext der Elemente darin.
	newBind := map[*template.Node]string{}
	newCtx := func(n *template.Node) string {
		var chain []*template.Node
		for x := n.Parent; x != nil; x = x.Parent {
			chain = append(chain, x)
		}
		ctx := newRoot
		for i := len(chain) - 1; i >= 0; i-- {
			x := chain[i]
			if x.Name != "subform" && x.Name != "table" {
				continue
			}
			b, ok := newBind[x]
			if !ok {
				b = x.Get("bind", "")
			}
			if b != "" {
				ctx = schema.Join(ctx, b)
			}
		}
		return ctx
	}
	var cmds []model.Command
	eachXPath(cat, t, func(n *template.Node, attr, expr, ctx string) {
		_, target := xpath.Paths(oldSc, ctx, expr)
		if target == "" {
			return // kein einfacher Pfad: bleibt für die Aufgabenliste
		}
		nc := newCtx(n)
		mapped := mapPath(target)
		if mapped == target && nc == ctx {
			return
		}
		v := schema.Relative(nc, mapped)
		if attr == "bind" && (n.Name == "subform" || n.Name == "table") {
			newBind[n] = v
		}
		if v != expr {
			cmds = append(cmds, model.Command{Op: "setAttr", Ref: model.Ref(n), Name: attr, Value: v})
		}
	})
	return cmds
}
