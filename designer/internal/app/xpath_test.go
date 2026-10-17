package app

import (
	"strings"
	"testing"
)

func TestXPathService(t *testing.T) {
	c := NewCore(nil, nil, t.TempDir())
	if _, err := NewProjectService(c).Open(muster); err != nil {
		t.Fatal(err)
	}
	ts := NewTemplateService(c)

	// Feld in der Tabellenzeile: Kontext ist die Position.
	info, err := ts.CheckXPath(XPathQuery{Ref: "z-betrag-f", Expr: "inv:amount"})
	if err != nil {
		t.Fatal(err)
	}
	if info.Context != "inv:invoice/inv:items/inv:item" || len(info.Problems) != 0 || info.Data != "samples/rechnung.xml" {
		t.Fatalf("Kontext %+v", info)
	}
	if info.Target == nil || info.Target.Type != "decimal" || info.Result == nil || info.Result.Count != 1 {
		t.Fatalf("Ziel und Wert %+v %+v %+v", info, info.Target, info.Result)
	}

	// Unbekannter Knoten mit Position in UTF-16.
	info, _ = ts.CheckXPath(XPathQuery{Ref: "z-betrag-f", Expr: "'ä' || inv:betrag"})
	if len(info.Problems) != 1 || info.Problems[0].Pos != 7 || info.Problems[0].End != 17 {
		t.Fatalf("Befund %+v", info.Problems)
	}

	// Vervollständigung mit Cursor in UTF-16.
	comp, err := ts.CompleteXPath(XPathQuery{Ref: "z-betrag-f", Expr: "'ä' || inv:am", Cursor: 13})
	if err != nil {
		t.Fatal(err)
	}
	if comp.From != 7 || comp.To != 13 || len(comp.Items) != 1 || comp.Items[0].Insert != "inv:amount" {
		t.Fatalf("Vorschläge %+v", comp)
	}

	// Felder für Präsenzbedingungen auf oberster Ebene.
	fields, err := ts.XPathFields("inhalt")
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, f := range fields {
		paths = append(paths, f.Path)
	}
	all := strings.Join(paths, " ")
	if !strings.Contains(all, "inv:paymentTerms") || !strings.Contains(all, "inv:seller/inv:name") || strings.Contains(all, "inv:item/inv:amount") {
		t.Fatalf("Felder %v", paths)
	}
}
