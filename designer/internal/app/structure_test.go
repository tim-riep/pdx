package app

import (
	"slices"
	"strings"
	"testing"

	"github.com/tim-riep/pdx/designer/internal/model"
)

func childKeys(cs []model.Child) []string {
	var out []string
	for _, c := range cs {
		out = append(out, c.Key)
	}
	return out
}

func TestInsertChildFromScratch(t *testing.T) {
	c := NewCore(nil, nil, t.TempDir())
	ps := NewProjectService(c)
	if st := ps.New(); !st.Open {
		t.Fatal("keine neue Vorlage")
	}
	ts := NewTemplateService(c)
	root := c.doc.Pkg.Template.Root.ID()

	ins, err := ts.Insertable(root)
	if err != nil {
		t.Fatal(err)
	}
	keys := childKeys(ins)
	for _, want := range []string{"namespaces", "styles", "tokens", "formats", "texts", "attachments"} {
		if !slices.Contains(keys, want) {
			t.Errorf("%s fehlt in %v", want, keys)
		}
	}
	if slices.Contains(keys, "body") || slices.Contains(keys, "pageSet") {
		t.Errorf("einmalige Kinder doppelt angeboten: %v", keys)
	}

	// In beliebiger Reihenfolge einfügen, das XSD bestimmt die Stelle.
	for _, k := range []string{"attachments", "texts", "formats", "namespaces", "styles"} {
		if _, err := ts.InsertChild(ChildInsert{Parent: root, Key: k, Index: -1}); err != nil {
			t.Fatalf("%s: %v", k, err)
		}
	}
	var names []string
	for _, ch := range c.doc.Pkg.Template.Root.Elements("") {
		names = append(names, ch.Name)
	}
	if got := strings.Join(names, " "); got != "namespaces styles formats texts pageSet body attachments" {
		t.Fatalf("Reihenfolge %s", got)
	}
	tpl := c.doc.Pkg.Template
	f := tpl.Root.First("formats")
	if f == nil || f.First("locale") == nil || f.First("locale").First("pattern") == nil {
		t.Fatalf("formats ohne Pflichtkinder")
	}
	if s := tpl.Root.First("texts").First("lang").First("string"); s == nil || s.Get("key", "") == "" {
		t.Fatal("string ohne key")
	}
	if a := tpl.Root.First("attachments").First("attachment"); a.Get("source", "") != "data" {
		t.Fatalf("Anhang %v", a.Attrs)
	}

	// Seitenvorlage mit Kopf.
	pm := tpl.Root.First("pageSet").First("pageMaster")
	res, err := ts.InsertChild(ChildInsert{Parent: pm.ID(), Key: "pageMaster/header", Index: -1})
	if err != nil {
		t.Fatal(err)
	}
	if h := tpl.ByID(res.Select); h == nil || h.Get("height", "") == "" {
		t.Fatalf("Kopf %v", h)
	}
	ins, _ = ts.Insertable(pm.ID())
	if slices.Contains(childKeys(ins), "pageMaster/header") {
		t.Fatal("zweiter Kopf angeboten")
	}
	// Bilder nur über die Bausteinleiste.
	if _, err := ts.InsertChild(ChildInsert{Parent: tpl.Root.First("body").ID(), Key: "image", Index: -1}); err == nil {
		t.Fatal("Bild ohne Datei eingefügt")
	}
}

func TestDuplicateAndMove(t *testing.T) {
	c := NewCore(nil, nil, t.TempDir())
	if _, err := NewProjectService(c).Open(muster); err != nil {
		t.Fatal(err)
	}
	ts := NewTemplateService(c)
	tpl := c.doc.Pkg.Template
	res, err := ts.Duplicate("rechnungsdaten")
	if err != nil {
		t.Fatal(err)
	}
	cp := tpl.ByID(res.Select)
	if cp == nil || cp.Index() != tpl.ByID("rechnungsdaten").Index()+1 {
		t.Fatalf("Kopie %v", res.Select)
	}
	// Alle IDs der Kopie sind neu.
	for _, ch := range cp.Elements("") {
		if strings.HasPrefix(ch.ID(), "rechnungsnummer") && !strings.Contains(ch.ID(), "kopie") {
			t.Fatalf("ID übernommen: %s", ch.ID())
		}
	}

	// Stile umsortieren: st-klein nach oben, dann wieder nach unten.
	styles := func() string {
		var ids []string
		for _, s := range tpl.Root.First("styles").Elements("style") {
			ids = append(ids, s.ID())
		}
		return strings.Join(ids[:3], " ")
	}
	if _, err := ts.Move("st-klein", -1); err != nil {
		t.Fatal(err)
	}
	if got := styles(); got != "st-klein st-basis st-titel" {
		t.Fatalf("nach oben: %s", got)
	}
	if _, err := ts.Move("st-klein", 1); err != nil {
		t.Fatal(err)
	}
	if got := styles(); got != "st-basis st-klein st-titel" {
		t.Fatalf("nach unten: %s", got)
	}
	if _, err := ts.Move("st-basis", -1); err == nil {
		t.Fatal("über den Anfang hinaus verschoben")
	}
	// Ungültige Reihenfolge lehnt das XSD ab: body vor pageSet.
	if _, err := ts.Move("inhalt", -1); err == nil {
		t.Fatal("body vor pageSet angenommen")
	}
}
