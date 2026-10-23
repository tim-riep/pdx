package model

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/tim-riep/pdx/template"
	"github.com/tim-riep/pdx/template/pkg"
)

// Command is a change to the template. The UI sends commands
// as JSON, the backend applies them and remembers the inverse for Undo.
//
//	setAttr  Ref, Name, Value  set attribute, empty Value removes it
//	insert   Parent, Index, XML  insert elements (XML in the template format)
//	remove   Ref               remove element
//	move     Ref, Parent, Index  move element
//	content  Ref, XML          replace mixed content (text and elements)
//	batch    Cmds              several commands as one step
//	setFile  Name              set or remove a file in the package (only
//	                           in the backend, Data carries the data)
//
// Index counts the children of the parent element, with mixed content
// including the texts. -1 appends.
type Command struct {
	Op     string    `json:"op"`
	Ref    string    `json:"ref,omitempty"`
	Name   string    `json:"name,omitempty"`
	Value  string    `json:"value,omitempty"`
	Parent string    `json:"parent,omitempty"`
	Index  int       `json:"index,omitempty"`
	XML    string    `json:"xml,omitempty"`
	Cmds   []Command `json:"cmds,omitempty"`

	// nodes carries the removed nodes themselves for inverses of remove.
	nodes []*template.Node
	// Data is the content for setFile, nil removes the file. It is not
	// transferred as JSON.
	Data []byte `json:"-"`
}

// entry is a step of the history.
type entry struct {
	label         string
	do, undo      Command
	before, after uint64 // state IDs
}

// Apply applies a command. If the template is invalid afterwards, the
// command is fully reverted and the error is returned.
func (d *Document) Apply(label string, c Command) error {
	inv, err := d.apply(c)
	if err != nil {
		return err
	}
	before := d.state
	d.touchState()
	d.undo = append(d.undo, entry{label: label, do: c, undo: inv, before: before, after: d.state})
	d.redo = nil
	return nil
}

// Undo takes back the last step.
func (d *Document) Undo() (string, error) {
	if len(d.undo) == 0 {
		return "", errors.New("nichts zum Rückgängigmachen")
	}
	e := d.undo[len(d.undo)-1]
	redo, err := d.apply(e.undo)
	if err != nil {
		return "", err
	}
	d.undo = d.undo[:len(d.undo)-1]
	e.do = redo
	d.redo = append(d.redo, e)
	d.rev++
	d.state = e.before
	return e.label, nil
}

// Redo repeats the most recently undone step.
func (d *Document) Redo() (string, error) {
	if len(d.redo) == 0 {
		return "", errors.New("nichts zum Wiederholen")
	}
	e := d.redo[len(d.redo)-1]
	undo, err := d.apply(e.do)
	if err != nil {
		return "", err
	}
	d.redo = d.redo[:len(d.redo)-1]
	e.undo = undo
	d.undo = append(d.undo, e)
	d.rev++
	d.state = e.after
	return e.label, nil
}

// History describes Undo and Redo for the menu and the toolbar.
type History struct {
	Undo string  `json:"undo"`
	Redo string  `json:"redo"`
	Can  [2]bool `json:"can"`
}

// History returns the labels of the next steps.
func (d *Document) History() History {
	var h History
	if n := len(d.undo); n > 0 {
		h.Undo, h.Can[0] = d.undo[n-1].label, true
	}
	if n := len(d.redo); n > 0 {
		h.Redo, h.Can[1] = d.redo[n-1].label, true
	}
	return h
}

// apply applies c, checks the template and returns the inverse. On an
// error the template is unchanged.
func (d *Document) apply(c Command) (Command, error) {
	t := d.Pkg.Template
	inv, err := exec(t, d.Pkg.Files, c)
	if err != nil {
		return Command{}, err
	}
	t.Reindex()
	err = t.Validate()
	if err == nil {
		// Check references to files in the package (schema, images, attachments),
		// otherwise the saved package could no longer be opened.
		_, err = pkg.New(t, d.Pkg.Files)
	}
	if err != nil {
		if _, rerr := exec(t, d.Pkg.Files, inv); rerr != nil {
			return Command{}, fmt.Errorf("%v (Zurücknehmen fehlgeschlagen: %v)", err, rerr)
		}
		t.Reindex()
		return Command{}, err
	}
	return inv, nil
}

// exec executes a command without checking and returns the inverse.
func exec(t *template.Template, files map[string][]byte, c Command) (Command, error) {
	switch c.Op {
	case "setFile":
		if c.Name == "" || c.Name == "template.xml" {
			return Command{}, fmt.Errorf("Datei %q ist im Paket nicht zulässig", c.Name)
		}
		old, had := files[c.Name]
		if c.Data == nil {
			delete(files, c.Name)
		} else {
			files[c.Name] = c.Data
		}
		inv := Command{Op: "setFile", Name: c.Name}
		if had {
			inv.Data = old
		}
		return inv, nil
	case "setAttr":
		n, err := Resolve(t, c.Ref)
		if err != nil {
			return Command{}, err
		}
		old, had := n.Attr(c.Name)
		if c.Name == "" {
			return Command{}, errors.New("setAttr ohne Namen")
		}
		cat, err := LoadCatalog()
		if err != nil {
			return Command{}, err
		}
		if et := cat.TypeOf(n); et == nil {
			return Command{}, fmt.Errorf("%s hat keinen bekannten Typ", c.Ref)
		} else if p, ok := et.Property(c.Name); !ok {
			return Command{}, fmt.Errorf("%s hat keine Eigenschaft %s", c.Ref, c.Name)
		} else if err := checkValue(p, c.Value); err != nil {
			return Command{}, err
		}
		if c.Value == "" {
			n.RemoveAttr(c.Name)
		} else {
			n.SetAttr(c.Name, c.Value)
		}
		ref := c.Ref
		if c.Name == "id" {
			ref = refAfter(n, c.Value)
		}
		inv := Command{Op: "setAttr", Ref: ref, Name: c.Name}
		if had {
			inv.Value = old
		}
		return inv, nil
	case "insert":
		parent, err := Resolve(t, c.Parent)
		if err != nil {
			return Command{}, err
		}
		nodes := c.nodes
		if nodes == nil {
			if nodes, err = parseFragment(c.XML); err != nil {
				return Command{}, err
			}
		}
		i := c.Index
		if i < 0 || i > len(parent.Children) {
			i = len(parent.Children)
		}
		var inv []Command
		for k, n := range nodes {
			parent.InsertChild(i+k, n)
		}
		// The inverse removes the nodes in reverse order, so that the
		// paths without ID stay valid.
		for k := len(nodes) - 1; k >= 0; k-- {
			inv = append(inv, Command{Op: "remove", Ref: Ref(nodes[k])})
		}
		if len(inv) == 1 {
			return inv[0], nil
		}
		return Command{Op: "batch", Cmds: inv}, nil
	case "remove":
		n, err := Resolve(t, c.Ref)
		if err != nil {
			return Command{}, err
		}
		if n.Parent == nil {
			return Command{}, errors.New("die Wurzel lässt sich nicht entfernen")
		}
		parent := n.Parent
		parentRef := Ref(parent)
		i := parent.RemoveChild(n)
		return Command{Op: "insert", Parent: parentRef, Index: i, nodes: []*template.Node{n}}, nil
	case "move":
		n, err := Resolve(t, c.Ref)
		if err != nil {
			return Command{}, err
		}
		target, err := Resolve(t, c.Parent)
		if err != nil {
			return Command{}, err
		}
		for p := target; p != nil; p = p.Parent {
			if p == n {
				return Command{}, errors.New("ein Element lässt sich nicht in sich selbst verschieben")
			}
		}
		from := n.Parent
		if from == nil {
			return Command{}, errors.New("die Wurzel lässt sich nicht verschieben")
		}
		fromRef := Ref(from)
		oldIndex := from.RemoveChild(n)
		i := c.Index
		if i < 0 || i > len(target.Children) {
			i = len(target.Children)
		}
		target.InsertChild(i, n)
		return Command{Op: "move", Ref: Ref(n), Parent: refAfterMove(from, fromRef, target, n), Index: oldIndex}, nil
	case "content":
		// Replace mixed content: text and embedded elements.
		n, err := Resolve(t, c.Ref)
		if err != nil {
			return Command{}, err
		}
		nodes, err := parseContent(c.XML)
		if err != nil {
			return Command{}, err
		}
		old := contentXML(n)
		for len(n.Children) > 0 {
			n.RemoveChild(n.Children[0])
		}
		for _, x := range nodes {
			n.InsertChild(-1, x)
		}
		return Command{Op: "content", Ref: Ref(n), XML: old}, nil
	case "batch":
		var inv []Command
		for _, sub := range c.Cmds {
			u, err := exec(t, files, sub)
			if err != nil {
				for k := len(inv) - 1; k >= 0; k-- {
					exec(t, files, inv[k])
				}
				return Command{}, err
			}
			inv = append(inv, u)
			t.Reindex()
		}
		slices.Reverse(inv)
		return Command{Op: "batch", Cmds: inv}, nil
	}
	return Command{}, fmt.Errorf("Befehl %q unbekannt", c.Op)
}

// refAfter returns the ID of an element after its ID was renamed.
func refAfter(n *template.Node, id string) string {
	if id != "" {
		return id
	}
	return Ref(n)
}

// refAfterMove returns the ID of the old parent element after the
// move. If it has an ID, it stays, otherwise the path is determined anew.
func refAfterMove(from *template.Node, fromRef string, _ *template.Node, _ *template.Node) string {
	if from.ID() != "" {
		return fromRef
	}
	return Ref(from)
}

// parseFragment reads elements in the template format without a root.
// parseContent reads mixed content. The inner wrapper is text, so that
// the parser keeps whitespace and text exactly as they are.
func parseContent(xml string) ([]*template.Node, error) {
	t, err := template.ParseTree([]byte(`<template xmlns="` + template.NS + `"><text>` + xml + `</text></template>`))
	if err != nil {
		return nil, fmt.Errorf("Inhalt: %v", err)
	}
	wrap := t.Root.First("text")
	out := slices.Clone(wrap.Children)
	for _, c := range out {
		wrap.RemoveChild(c)
	}
	return out, nil
}

// contentXML writes the content of an element as a fragment, the way
// parseContent reads it.
func contentXML(n *template.Node) string {
	wrap := template.NewElement("text")
	for _, c := range n.Children {
		wrap.InsertChild(-1, c.Clone())
	}
	out := string(template.Write(&template.Template{Root: wrap}))
	start := strings.Index(out, "<text")
	if start < 0 {
		return ""
	}
	open := out[start:]
	if strings.HasPrefix(open[strings.Index(open, ">")-1:], "/>") {
		return "" // empty
	}
	body := open[strings.Index(open, ">")+1:]
	return body[:strings.LastIndex(body, "</text>")]
}

func parseFragment(xml string) ([]*template.Node, error) {
	var b bytes.Buffer
	b.WriteString(`<template xmlns="` + template.NS + `">`)
	b.WriteString(xml)
	b.WriteString(`</template>`)
	t, err := template.ParseTree(b.Bytes())
	if err != nil {
		return nil, fmt.Errorf("Einfügen: %v", err)
	}
	var out []*template.Node
	for _, c := range slices.Clone(t.Root.Children) {
		if c.Kind == template.Element {
			t.Root.RemoveChild(c)
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("Einfügen: kein Element")
	}
	return out, nil
}
