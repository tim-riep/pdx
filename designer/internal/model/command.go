package model

import (
	"bytes"
	"errors"
	"fmt"
	"slices"

	"github.com/tim-riep/pdx/template"
	"github.com/tim-riep/pdx/template/pkg"
)

// Command ist eine Änderung an der Vorlage. Die Oberfläche schickt Befehle
// als JSON, das Backend wendet sie an und merkt sich das Inverse für Undo.
//
//	setAttr  Ref, Name, Value  Attribut setzen, leerer Value entfernt es
//	insert   Parent, Index, XML  Elemente einfügen (XML im Vorlagenformat)
//	remove   Ref               Element entfernen
//	move     Ref, Parent, Index  Element verschieben
//	batch    Cmds              mehrere Befehle als ein Schritt
//
// Index zählt die Kinder des Elternelements, bei gemischtem Inhalt
// einschließlich der Texte. -1 hängt an.
type Command struct {
	Op     string    `json:"op"`
	Ref    string    `json:"ref,omitempty"`
	Name   string    `json:"name,omitempty"`
	Value  string    `json:"value,omitempty"`
	Parent string    `json:"parent,omitempty"`
	Index  int       `json:"index,omitempty"`
	XML    string    `json:"xml,omitempty"`
	Cmds   []Command `json:"cmds,omitempty"`

	// nodes trägt bei Inversen von remove die entfernten Knoten selbst.
	nodes []*template.Node
}

// entry ist ein Schritt der Historie.
type entry struct {
	label         string
	do, undo      Command
	before, after uint64 // Zustandskennungen
}

// Apply wendet einen Befehl an. Ist die Vorlage danach ungültig, wird der
// Befehl vollständig zurückgenommen und der Fehler geliefert.
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

// Undo nimmt den letzten Schritt zurück.
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

// Redo wiederholt den zuletzt zurückgenommenen Schritt.
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

// History beschreibt Undo und Redo für Menü und Werkzeugleiste.
type History struct {
	Undo string `json:"undo"`
	Redo string `json:"redo"`
	Can  [2]bool `json:"can"`
}

// History liefert die Beschriftungen der nächsten Schritte.
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

// apply wendet c an, prüft die Vorlage und liefert das Inverse. Bei einem
// Fehler ist die Vorlage unverändert.
func (d *Document) apply(c Command) (Command, error) {
	t := d.Pkg.Template
	inv, err := exec(t, c)
	if err != nil {
		return Command{}, err
	}
	t.Reindex()
	err = t.Validate()
	if err == nil {
		// Verweise auf Dateien im Paket (Schema, Bilder, Anhänge) prüfen,
		// sonst ließe sich das gespeicherte Paket nicht mehr öffnen.
		_, err = pkg.New(t, d.Pkg.Files)
	}
	if err != nil {
		if _, rerr := exec(t, inv); rerr != nil {
			return Command{}, fmt.Errorf("%v (Zurücknehmen fehlgeschlagen: %v)", err, rerr)
		}
		t.Reindex()
		return Command{}, err
	}
	return inv, nil
}

// exec führt einen Befehl ohne Prüfung aus und liefert das Inverse.
func exec(t *template.Template, c Command) (Command, error) {
	switch c.Op {
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
		// Das Inverse entfernt die Knoten in umgekehrter Folge, damit die
		// Pfade ohne ID gültig bleiben.
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
	case "batch":
		var inv []Command
		for _, sub := range c.Cmds {
			u, err := exec(t, sub)
			if err != nil {
				for k := len(inv) - 1; k >= 0; k-- {
					exec(t, inv[k])
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

// refAfter liefert die Kennung eines Elements nach dem Umbenennen der ID.
func refAfter(n *template.Node, id string) string {
	if id != "" {
		return id
	}
	return Ref(n)
}

// refAfterMove liefert die Kennung des alten Elternelements nach dem
// Verschieben. Hat es eine ID, bleibt sie, sonst wird der Pfad neu bestimmt.
func refAfterMove(from *template.Node, fromRef string, _ *template.Node, _ *template.Node) string {
	if from.ID() != "" {
		return fromRef
	}
	return Ref(from)
}

// parseFragment liest Elemente im Vorlagenformat ohne Wurzel.
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
