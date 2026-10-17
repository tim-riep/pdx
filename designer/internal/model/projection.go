package model

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/tim-riep/pdx/template"
)

// Item ist ein Kind in der Projektion: ein Element (Ref) oder bei gemischtem
// Inhalt ein Text.
type Item struct {
	Ref  string `json:"ref,omitempty"`
	Text string `json:"text,omitempty"`
}

// ViewNode ist ein Element der Vorlage, wie die Oberfläche es sieht. Die
// Oberfläche ändert es nie selbst, sie schickt Befehle.
type ViewNode struct {
	Ref      string            `json:"ref"`
	Name     string            `json:"name"`
	Type     string            `json:"type"`
	Parent   string            `json:"parent"`
	Attrs    map[string]string `json:"attrs"`
	Children []Item            `json:"children"`
}

// Snapshot ist die vollständige Projektion einer Revision.
type Snapshot struct {
	Rev   uint64              `json:"rev"`
	Root  string              `json:"root"`
	Nodes map[string]ViewNode `json:"nodes"`
}

// Ref liefert die Kennung eines Elements: seine ID oder, ohne ID, den Pfad
// über den nächsten Vorfahren mit ID, etwa "~seiten/2".
func Ref(n *template.Node) string {
	if id := n.ID(); id != "" {
		return id
	}
	if n.Parent == nil {
		return "~"
	}
	return "~" + strings.TrimPrefix(Ref(n.Parent), "~") + "/" + strconv.Itoa(n.Index())
}

// Resolve findet ein Element über seine Kennung.
func Resolve(t *template.Template, ref string) (*template.Node, error) {
	if !strings.HasPrefix(ref, "~") {
		if n := t.ByID(ref); n != nil {
			return n, nil
		}
		return nil, fmt.Errorf("Element %q nicht gefunden", ref)
	}
	parts := strings.Split(strings.TrimPrefix(ref, "~"), "/")
	var n *template.Node
	if parts[0] == "" {
		n = t.Root
	} else if n = t.ByID(parts[0]); n == nil {
		return nil, fmt.Errorf("Element %q nicht gefunden", ref)
	}
	for _, p := range parts[1:] {
		i, err := strconv.Atoi(p)
		if err != nil || i < 0 || i >= len(n.Children) {
			return nil, fmt.Errorf("Element %q nicht gefunden", ref)
		}
		n = n.Children[i]
	}
	return n, nil
}

// Project baut die Projektion einer Vorlage.
func Project(t *template.Template, rev uint64, cat Catalog) Snapshot {
	s := Snapshot{Rev: rev, Root: Ref(t.Root), Nodes: map[string]ViewNode{}}
	var walk func(n *template.Node)
	walk = func(n *template.Node) {
		vn := view(n, cat)
		s.Nodes[vn.Ref] = vn
		for _, c := range n.Children {
			if c.Kind == template.Element {
				walk(c)
			}
		}
	}
	walk(t.Root)
	return s
}

// View liefert die Projektion eines einzelnen Elements, etwa für Patches.
func View(n *template.Node, cat Catalog) ViewNode { return view(n, cat) }

func view(n *template.Node, cat Catalog) ViewNode {
	vn := ViewNode{Ref: Ref(n), Name: n.Name, Attrs: map[string]string{}, Children: []Item{}}
	if et := cat.TypeOf(n); et != nil {
		vn.Type = et.Key
	}
	if n.Parent != nil {
		vn.Parent = Ref(n.Parent)
	}
	for _, a := range n.Attrs {
		vn.Attrs[a.Name] = a.Value
	}
	mixed := vn.Type != "" && cat[vn.Type] != nil && cat[vn.Type].Mixed
	for _, c := range n.Children {
		switch {
		case c.Kind == template.Element:
			vn.Children = append(vn.Children, Item{Ref: Ref(c)})
		case mixed:
			vn.Children = append(vn.Children, Item{Text: c.Data})
		}
	}
	return vn
}

// Patch sind die Änderungen der Projektion von einer Revision zur nächsten.
type Patch struct {
	From    uint64     `json:"from"`
	Rev     uint64     `json:"rev"`
	Root    string     `json:"root"`
	Set     []ViewNode `json:"set"`
	Removed []string   `json:"removed"`
}

// Diff vergleicht zwei Projektionen.
func Diff(old, cur Snapshot) Patch {
	p := Patch{From: old.Rev, Rev: cur.Rev, Root: cur.Root, Set: []ViewNode{}, Removed: []string{}}
	for ref, n := range cur.Nodes {
		if o, ok := old.Nodes[ref]; !ok || !sameNode(o, n) {
			p.Set = append(p.Set, n)
		}
	}
	for ref := range old.Nodes {
		if _, ok := cur.Nodes[ref]; !ok {
			p.Removed = append(p.Removed, ref)
		}
	}
	sort.Slice(p.Set, func(i, j int) bool { return p.Set[i].Ref < p.Set[j].Ref })
	sort.Strings(p.Removed)
	return p
}

func sameNode(a, b ViewNode) bool {
	return a.Name == b.Name && a.Type == b.Type && a.Parent == b.Parent &&
		maps.Equal(a.Attrs, b.Attrs) && slices.Equal(a.Children, b.Children)
}
