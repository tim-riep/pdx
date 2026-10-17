package app

import (
	"strings"
	"testing"

	"github.com/tim-riep/pdx/designer/internal/model"
	"github.com/tim-riep/pdx/template"
)

func TestTokens(t *testing.T) {
	c := NewCore(nil, nil, t.TempDir())
	if _, err := NewProjectService(c).Open(muster); err != nil {
		t.Fatal(err)
	}
	ts := NewTemplateService(c)
	tpl := func() *template.Template { return c.doc.Pkg.Template }
	if tpl().Root.First("tokens") != nil {
		t.Fatal("Musterrechnung hat schon Tokens")
	}

	// Erstes Token legt tokens nach styles an.
	res, err := ts.Tokens(TokenOp{Op: "add", Kind: "color"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Select != "farbe" {
		t.Fatalf("Auswahl %q", res.Select)
	}
	var names []string
	for _, ch := range tpl().Root.Elements("") {
		names = append(names, ch.Name)
	}
	if !strings.Contains(strings.Join(names, " "), "styles tokens formats") {
		t.Fatalf("Reihenfolge %v", names)
	}
	if _, err := ts.Tokens(TokenOp{Op: "add", Kind: "length", ID: "abstand-m"}); err != nil {
		t.Fatal(err)
	}

	// Verwenden: Farbe und Länge in Stil und Element, auch doppelt.
	for _, cmd := range []model.Command{
		{Op: "setAttr", Ref: "st-titel", Name: "color", Value: "$farbe"},
		{Op: "setAttr", Ref: "titel", Name: "padding", Value: "$abstand-m $abstand-m"},
		{Op: "setAttr", Ref: "titel", Name: "border", Value: "0.5pt solid $farbe"},
		{Op: "setAttr", Ref: "titel", Name: "presence", Value: "not($farbe)"},
	} {
		if _, err := ts.Apply("test", cmd); err != nil {
			t.Fatalf("%+v: %v", cmd, err)
		}
	}
	use, _ := ts.TokenUsage()
	if use["farbe"] != 2 || use["abstand-m"] != 2 {
		t.Fatalf("Verwendung %v (XPath zählt nicht)", use)
	}

	// Entfernen geht nicht, solange verwendet.
	if _, err := ts.Tokens(TokenOp{Op: "remove", ID: "farbe"}); err == nil || !strings.Contains(err.Error(), "2 Stellen") {
		t.Fatalf("Entfernen trotz Verwendung: %v", err)
	}

	// Umbenennen zieht alle Verweise mit, XPath bleibt, als ein Schritt.
	if _, err := ts.Tokens(TokenOp{Op: "rename", ID: "farbe", NewID: "primaer"}); err != nil {
		t.Fatal(err)
	}
	if v := tpl().ByID("st-titel").Get("color", ""); v != "$primaer" {
		t.Fatalf("Stil: %q", v)
	}
	if v := tpl().ByID("titel").Get("border", ""); v != "0.5pt solid $primaer" {
		t.Fatalf("Rahmen: %q", v)
	}
	if v := tpl().ByID("titel").Get("presence", ""); v != "not($farbe)" {
		t.Fatalf("XPath verändert: %q", v)
	}
	if _, err := ts.Tokens(TokenOp{Op: "rename", ID: "abstand-m", NewID: "rand"}); err != nil {
		t.Fatal(err)
	}
	if v := tpl().ByID("titel").Get("padding", ""); v != "$rand $rand" {
		t.Fatalf("doppelter Verweis: %q", v)
	}
	if _, err := ts.Tokens(TokenOp{Op: "rename", ID: "rand", NewID: "titel"}); err == nil {
		t.Fatal("vergebener Name angenommen")
	}
	if _, err := ts.Undo(); err != nil {
		t.Fatal(err)
	}
	if v := tpl().ByID("titel").Get("padding", ""); v != "$abstand-m $abstand-m" {
		t.Fatalf("Undo: %q", v)
	}

	// Ohne Verweise lässt sich entfernen, das letzte nimmt tokens mit.
	for _, cmd := range []model.Command{
		{Op: "setAttr", Ref: "st-titel", Name: "color", Value: ""},
		{Op: "setAttr", Ref: "titel", Name: "padding", Value: ""},
		{Op: "setAttr", Ref: "titel", Name: "border", Value: ""},
		{Op: "setAttr", Ref: "titel", Name: "presence", Value: ""},
	} {
		if _, err := ts.Apply("test", cmd); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"primaer", "abstand-m"} {
		if _, err := ts.Tokens(TokenOp{Op: "remove", ID: id}); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
	}
	if tpl().Root.First("tokens") != nil {
		t.Fatal("leeres tokens bleibt")
	}
}

func TestTokensRender(t *testing.T) {
	c := NewCore(nil, nil, t.TempDir())
	if _, err := NewProjectService(c).Open(muster); err != nil {
		t.Fatal(err)
	}
	ts := NewTemplateService(c)
	if _, err := ts.Tokens(TokenOp{Op: "add", Kind: "font", ID: "gross"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ts.Apply("Schrift", model.Command{Op: "setAttr", Ref: "titel", Name: "font", Value: "$gross"}); err != nil {
		t.Fatal(err)
	}
	ps := runChecks(template.Write(c.doc.Pkg.Template), c.doc.Pkg.Files)
	for _, p := range ps {
		if p.Level == "Fehler" {
			t.Fatalf("Fehler mit Schrift-Token: %+v", p)
		}
	}
	// Unbekanntes Token: der Compiler meldet es am Element.
	if _, err := ts.Apply("Farbe", model.Command{Op: "setAttr", Ref: "titel", Name: "color", Value: "$fehlt"}); err != nil {
		t.Fatal(err)
	}
	ps = runChecks(template.Write(c.doc.Pkg.Template), c.doc.Pkg.Files)
	var hit bool
	for _, p := range ps {
		hit = hit || p.Ref == "titel" && strings.Contains(p.Msg, "$fehlt")
	}
	if !hit {
		t.Fatalf("unbekanntes Token nicht gemeldet: %+v", ps)
	}
}

func TestAddStyle(t *testing.T) {
	c := NewCore(nil, nil, t.TempDir())
	if _, err := NewProjectService(c).Open(muster); err != nil {
		t.Fatal(err)
	}
	ts := NewTemplateService(c)
	res, err := ts.AddStyle("st-basis")
	if err != nil {
		t.Fatal(err)
	}
	st := c.doc.Pkg.Template.ByID(res.Select)
	if st == nil || st.Name != "style" || st.Get("parent", "") != "st-basis" {
		t.Fatalf("Stil %+v", st)
	}
	if _, err := ts.AddStyle("titel"); err == nil {
		t.Fatal("Elternstil, der kein Stil ist, angenommen")
	}
}
