package model

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"path/filepath"
	"testing"

	"github.com/tim-riep/pdx/template"
)

// gen erzeugt zufällige, teils ungültige Befehle für eine Vorlage.
type gen struct {
	r   *rand.Rand
	d   *Document
	cat Catalog
	n   int // Zähler für neue IDs
}

func (g *gen) elements() []*template.Node {
	var out []*template.Node
	var walk func(n *template.Node)
	walk = func(n *template.Node) {
		if n.Kind != template.Element {
			return
		}
		out = append(out, n)
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(g.d.Pkg.Template.Root)
	return out
}

func (g *gen) pick(list []*template.Node) *template.Node { return list[g.r.IntN(len(list))] }

func (g *gen) value(p Property) string {
	switch {
	case g.r.IntN(6) == 0:
		return "" // entfernen
	case p.Kind == "enum":
		return p.Enum[g.r.IntN(len(p.Enum))]
	case p.Kind == "length":
		return fmt.Sprintf("%dmm", g.r.IntN(80))
	case p.Kind == "lengths":
		return fmt.Sprintf("%dmm %dmm", g.r.IntN(20), g.r.IntN(20))
	case p.Kind == "color":
		return fmt.Sprintf("#%06x", g.r.IntN(1<<24))
	case p.Kind == "bool":
		return []string{"true", "false"}[g.r.IntN(2)]
	case p.Kind == "number":
		return fmt.Sprint(1 + g.r.IntN(4))
	case p.Kind == "id":
		g.n++
		return fmt.Sprintf("neu-%d", g.n)
	case p.Kind == "ref":
		return []string{"st-basis", "st-klein", "gibtsnicht"}[g.r.IntN(3)]
	}
	return []string{"Test", "inv:totals/inv:net", "", "§ äöü"}[g.r.IntN(4)]
}

// renamed kopiert einen Teilbaum mit neuen IDs.
func (g *gen) renamed(n *template.Node) *template.Node {
	c := n.Clone()
	var walk func(x *template.Node)
	walk = func(x *template.Node) {
		if x.ID() != "" {
			g.n++
			x.SetAttr("id", fmt.Sprintf("%s-k%d", x.ID(), g.n))
		}
		for _, ch := range x.Children {
			walk(ch)
		}
	}
	walk(c)
	return c
}

func (g *gen) command(depth int) Command {
	els := g.elements()
	switch op := g.r.IntN(10); {
	case op < 5:
		n := g.pick(els)
		et := g.cat.TypeOf(n)
		if et == nil || len(et.Properties) == 0 {
			return Command{Op: "setAttr", Ref: Ref(n), Name: "id", Value: "x"}
		}
		p := et.Properties[g.r.IntN(len(et.Properties))]
		return Command{Op: "setAttr", Ref: Ref(n), Name: p.Name, Value: g.value(p)}
	case op < 6:
		return Command{Op: "remove", Ref: Ref(g.pick(els[1:]))}
	case op < 8:
		n, target := g.pick(els[1:]), g.pick(els)
		return Command{Op: "move", Ref: Ref(n), Parent: Ref(target), Index: g.r.IntN(len(target.Children)+2) - 1}
	case op < 9:
		target, src := g.pick(els), g.pick(els[1:])
		var b bytes.Buffer
		tmp := &template.Template{Root: g.renamed(src)}
		w := template.Write(tmp)
		b.Write(w[bytes.IndexByte(w, '\n')+1:]) // ohne XML-Deklaration
		xml := bytes.ReplaceAll(b.Bytes(), []byte(` xmlns="`+template.NS+`"`), nil)
		return Command{Op: "insert", Parent: Ref(target), Index: g.r.IntN(len(target.Children)+2) - 1, XML: string(xml)}
	default:
		if depth > 1 {
			return g.command(depth + 1)
		}
		cmds := make([]Command, 1+g.r.IntN(3))
		for i := range cmds {
			cmds[i] = g.command(depth + 1)
		}
		return Command{Op: "batch", Cmds: cmds}
	}
}

// TestRandomCommands wendet zufällige Befehlsfolgen an. Fehlgeschlagene
// Befehle ändern nichts, vollständiges Undo ergibt die Ausgangsvorlage,
// Redo den Endstand, Speichern und Laden ist verlustfrei.
func TestRandomCommands(t *testing.T) {
	cat, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	seeds := 40
	if testing.Short() {
		seeds = 5
	}
	for seed := range seeds {
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			d := muster(t)
			g := &gen{r: rand.New(rand.NewPCG(uint64(seed), 7)), d: d, cat: cat}
			start := canon(d)
			applied := 0
			for range 30 {
				before := canon(d)
				c := g.command(0)
				if err := d.Apply("x", c); err != nil {
					if !bytes.Equal(canon(d), before) {
						t.Fatalf("fehlgeschlagener Befehl %+v hat die Vorlage verändert: %v", c, err)
					}
					continue
				}
				applied++
				if err := d.Pkg.Template.Validate(); err != nil {
					t.Fatalf("nach %+v ungültig: %v", c, err)
				}
			}
			end := canon(d)
			// Speichern und Laden.
			path := filepath.Join(t.TempDir(), "v.zip")
			if err := d.SaveAs(path); err != nil {
				t.Fatal(err)
			}
			re, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(canon(re), end) {
				t.Fatal("Speichern und Laden verändert die Vorlage")
			}
			for range applied {
				if _, err := d.Undo(); err != nil {
					t.Fatal(err)
				}
			}
			if !bytes.Equal(canon(d), start) {
				t.Fatalf("nach %d Undo nicht im Ausgangszustand", applied)
			}
			for range applied {
				if _, err := d.Redo(); err != nil {
					t.Fatal(err)
				}
			}
			if !bytes.Equal(canon(d), end) {
				t.Fatal("Redo stellt den Endstand nicht her")
			}
			t.Logf("%d von 30 Befehlen angewendet", applied)
		})
	}
}
