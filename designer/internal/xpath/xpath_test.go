package xpath

import (
	"reflect"
	"strings"
	"testing"

	"github.com/tim-riep/pdx/designer/internal/schema"
	"github.com/tim-riep/pdx/template/pkg"
)

func load(t *testing.T) (*pkg.Package, *schema.Schema) {
	t.Helper()
	p, err := pkg.ReadDir("../../../testdata/templates/musterrechnung")
	if err != nil {
		t.Fatal(err)
	}
	s, err := schema.Load(p)
	if err != nil || s == nil {
		t.Fatalf("Load: %v", err)
	}
	return p, s
}

const inv = "inv:invoice"

func TestPaths(t *testing.T) {
	_, sc := load(t)
	cases := []struct {
		ctx, expr string
		problems  []string // marked text of the findings
		target    string
	}{
		{inv, "inv:number", nil, inv + "/inv:number"},
		{inv, "inv:items/inv:item", nil, inv + "/inv:items/inv:item"},
		{inv, "inv:nummer", []string{"inv:nummer"}, ""},
		{inv, "inv:items/inv:position", []string{"inv:position"}, ""},
		{inv + "/inv:items/inv:item", "inv:quantity/@unit", nil, inv + "/inv:items/inv:item/inv:quantity/@unit"},
		{inv + "/inv:items/inv:item", "inv:quantity/@einheit", []string{"inv:quantity/@einheit"[13:]}, ""},
		{inv + "/inv:items/inv:item", "../../inv:number", nil, inv + "/inv:number"},
		{inv, "../inv:x", []string{".."}, ""},
		{inv, "/inv:invoice/inv:totals/inv:gross", nil, inv + "/inv:totals/inv:gross"},
		{inv, "exists(inv:paymentTerms) and inv:totals/inv:nett > 0", []string{"inv:nett"}, ""},
		{inv, "count(inv:items/inv:item[inv:amount > 100 and inv:menge])", []string{"inv:menge"}, ""},
		{inv, "for $i in inv:items/inv:item return $i/inv:egal", nil, ""},
		{inv, "inv:items/inv:item ! inv:egal", nil, ""},
		{inv, "'inv:kein/pfad' || inv:number", nil, ""},
		{inv, "inv:number cast as xs:string", nil, ""},
		{inv, "if (inv:paymentTerms) then 1 else 0", nil, ""},
		{inv, "inv:items//inv:egal", nil, ""},
		{inv, "sum(inv:items/inv:item/inv:amount) div 2", nil, ""},
		{inv, "inv:number and or", []string{"or"}, ""}, // or is an element name here
	}
	for _, c := range cases {
		probs, target := Paths(sc, c.ctx, c.expr)
		var got []string
		for _, p := range probs {
			got = append(got, c.expr[p.Pos:p.End])
		}
		if !reflect.DeepEqual(got, c.problems) || target != c.target {
			t.Errorf("%s: Befunde %q (%v), Ziel %q, erwartet %q, %q", c.expr, got, probs, target, c.problems, c.target)
		}
	}
}

func TestEval(t *testing.T) {
	p, sc := load(t)
	ns := map[string]string{"inv": "https://github.com/tim-riep/pdx/ns/examples/invoice"}
	data := p.Files[p.Samples()[0]]
	r := Eval("inv:items/inv:item", inv, ns, data)
	if r.Error != "" || r.Kind != "nodes" || r.Count < 2 || len(r.Values) == 0 {
		t.Fatalf("Positionen: %+v", r)
	}
	r = Eval("exists(inv:number)", inv, ns, data)
	if r.Kind != "boolean" || r.Truth != "true" || r.Values[0] != "true" {
		t.Fatalf("exists: %+v", r)
	}
	r = Eval("inv:amount", inv+"/inv:items/inv:item", ns, data)
	if r.Count != 1 || r.Values[0] == "" {
		t.Fatalf("Betrag im Kontext der Position: %+v", r)
	}
	for expr, want := range map[string]string{
		"inv:number +* )":       "Unerwartet: „)“",
		"count(":                "Der Ausdruck ist unvollständig",
		"inv:number = 'a":       "Die Zeichenkette ist nicht geschlossen",
		"unbekannt:x":           "Der Präfix unbekannt ist in der Vorlage nicht deklariert",
		"sum(inv:number, 1, 2)": "sum() nimmt keine 3 Argumente",
		"foo(1)":                "Die Funktion foo() gibt es nicht",
	} {
		if r := Eval(expr, inv, ns, data); r.Error != want {
			t.Errorf("%s: %q, erwartet %q", expr, r.Error, want)
		}
	}
	r = Eval("inv:number +* )", inv, ns, data)
	if r.Error == "" {
		t.Fatalf("Syntaxfehler nicht erkannt: %+v", r)
	}
	r = Eval("inv:nichts", inv, ns, data)
	if r.Error != "" || r.Kind != "empty" || r.Truth != "false" {
		t.Fatalf("leer: %+v", r)
	}
	r = Eval("unbekannt:x", inv, ns, data)
	if r.Error == "" {
		t.Fatalf("unbekannter Präfix nicht erkannt: %+v", r)
	}
	r = Eval("1", "inv:gibtsnicht", ns, data)
	if !r.NoContext {
		t.Fatalf("fehlender Kontext: %+v", r)
	}
	// Placeholder data works just as well.
	r = Eval("count(inv:items/inv:item)", inv, ns, sc.Placeholder())
	if r.Values[0] != "3" {
		t.Fatalf("Platzhalter: %+v", r)
	}
}

func labels(c Completion) []string {
	var out []string
	for _, i := range c.Items {
		out = append(out, i.Label)
	}
	return out
}

func TestComplete(t *testing.T) {
	_, sc := load(t)
	has := func(c Completion, want ...string) bool {
		got := strings.Join(labels(c), " ")
		for _, w := range want {
			if !strings.Contains(" "+got+" ", " "+w+" ") {
				return false
			}
		}
		return true
	}
	// Empty: all children of the context.
	c := Complete(sc, inv, "", 0)
	if !has(c, "inv:number", "inv:items", "inv:totals") || c.From != 0 || c.To != 0 {
		t.Fatalf("leer: %v %+v", labels(c), c)
	}
	// The start of the local name is enough, plus functions.
	c = Complete(sc, inv, "nu", 2)
	if !has(c, "inv:number", "number", "normalize-space") == has(c, "normalize-space") || !has(c, "inv:number", "number") || c.From != 0 {
		t.Fatalf("nu: %v", labels(c))
	}
	// After a step the children of the step, without functions.
	expr := "inv:items/inv:item/inv:am"
	c = Complete(sc, inv, expr, len(expr))
	if !reflect.DeepEqual(labels(c), []string{"inv:amount"}) || c.From != len("inv:items/inv:item/") || c.Context != inv+"/inv:items/inv:item" {
		t.Fatalf("Pfad: %v %+v", labels(c), c)
	}
	// Attributes
	expr = "inv:items/inv:item/inv:quantity/@"
	c = Complete(sc, inv, expr, len(expr))
	if !reflect.DeepEqual(labels(c), []string{"@unit"}) {
		t.Fatalf("Attribut: %v", labels(c))
	}
	// In a predicate the step before it applies.
	expr = "inv:items/inv:item[inv:a"
	c = Complete(sc, inv, expr, len(expr))
	if !reflect.DeepEqual(labels(c), []string{"inv:amount"}) {
		t.Fatalf("Prädikat: %v", labels(c))
	}
	// In function arguments the outer context.
	expr = "sum(inv:to"
	c = Complete(sc, inv, expr, len(expr))
	if !has(c, "inv:totals") {
		t.Fatalf("Argument: %v", labels(c))
	}
	// Nothing in a string.
	expr = "concat('inv:"
	if c = Complete(sc, inv, expr, len(expr)); len(c.Items) != 0 {
		t.Fatalf("Zeichenkette: %v", labels(c))
	}
	// In the middle of a word the suggestion replaces the whole word.
	expr = "inv:numXX"
	c = Complete(sc, inv, expr, 7)
	if c.From != 0 || c.To != len(expr) {
		t.Fatalf("Bereich: %+v", c)
	}
	// Parent step in a deeper context.
	c = Complete(sc, inv+"/inv:items/inv:item", ".", 1)
	if !has(c, "..") {
		t.Fatalf("Eltern: %v", labels(c))
	}
	// Absolute
	expr = "/inv:invoice/inv:sel"
	c = Complete(sc, inv+"/inv:items/inv:item", expr, len(expr))
	if !reflect.DeepEqual(labels(c), []string{"inv:seller"}) {
		t.Fatalf("absolut: %v", labels(c))
	}
}

func TestTargets(t *testing.T) {
	_, sc := load(t)
	got := Targets(sc, inv, "sum(inv:items/inv:item/inv:amount) > 0 and exists(inv:paymentTerms) and inv:items/inv:item[inv:quantity > 1]")
	want := []string{inv + "/inv:items/inv:item/inv:amount", inv + "/inv:paymentTerms", inv + "/inv:items/inv:item/inv:quantity", inv + "/inv:items/inv:item"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Ziele %q", got)
	}
	if got := Targets(sc, inv, "$x/inv:egal or inv:gibtsnicht"); len(got) != 0 {
		t.Fatalf("unbekannte Ziele %q", got)
	}
}
