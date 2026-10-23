package app

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// changedXSD writes a changed version of the invoice XSD: seller becomes
// vendor, tax is dropped, taxRate becomes text and orderRef is new and required.
func changedXSD(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(muster, "schema", "invoice.xsd"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, r := range [][2]string{
		{`<xs:element name="seller" type="Party"/>`, `<xs:element name="vendor" type="Party"/>`},
		{`<xs:element name="tax" type="Amount"/>`, ``},
		{`<xs:element name="taxRate" type="xs:decimal"/>`, `<xs:element name="taxRate" type="xs:string"/>`},
		{`<xs:element name="number" type="xs:string"/>`, `<xs:element name="number" type="xs:string"/><xs:element name="orderRef" type="xs:string"/>`},
	} {
		if !strings.Contains(s, r[0]) {
			t.Fatalf("XSD ohne %s", r[0])
		}
		s = strings.Replace(s, r[0], r[1], 1)
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "rechnung-v2.xsd")
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSchemaDiffAndReplace(t *testing.T) {
	c := NewCore(nil, nil, t.TempDir())
	if _, err := NewProjectService(c).Open(muster); err != nil {
		t.Fatal(err)
	}
	ss := NewSchemaService(c)
	p := changedXSD(t)
	d, err := ss.Diff(p)
	if err != nil {
		t.Fatal(err)
	}
	byKind := map[string][]SchemaChange{}
	for _, ch := range d.Changes {
		byKind[ch.Kind] = append(byKind[ch.Kind], ch)
	}
	ren := byKind["renamed"]
	if len(ren) != 1 || ren[0].Path != "inv:invoice/inv:seller" || ren[0].To != "inv:invoice/inv:vendor" {
		t.Fatalf("Umbenennung %+v", ren)
	}
	for _, id := range []string{"briefkopf-firma-name", "absender-name", "gc-name", "gc-iban"} {
		if !slices.Contains(ren[0].Refs, id) {
			t.Errorf("%s fehlt in %v", id, ren[0].Refs)
		}
	}
	if rm := byKind["removed"]; len(rm) != 1 || rm[0].Path != "inv:invoice/inv:totals/inv:tax" || len(rm[0].Refs) == 0 {
		t.Fatalf("entfernt %+v", rm)
	}
	if ch := byKind["changed"]; len(ch) != 1 || !strings.Contains(ch[0].Detail, "Typ string statt decimal") {
		t.Fatalf("geändert %+v", ch)
	}
	if ad := byKind["added"]; len(ad) != 1 || ad[0].Path != "inv:invoice/inv:orderRef" || !ad[0].Required {
		t.Fatalf("neu %+v", ad)
	}
	// Diff changes nothing.
	if c.doc.Pkg.Template.Root.Get("schema", "") != "schema/invoice.xsd" {
		t.Fatal("Diff hat die Vorlage geändert")
	}

	// Replace with switching: bindings to seller point to vendor, the
	// old XSD is gone, all in one step.
	if _, err := ss.Replace(p, []Rename{{From: ren[0].Path, To: ren[0].To}}); err != nil {
		t.Fatal(err)
	}
	tpl := c.doc.Pkg.Template
	if v := tpl.ByID("briefkopf-firma-name").Get("bind", ""); v != "inv:vendor/inv:name" {
		t.Fatalf("Bindung %q", v)
	}
	if _, ok := c.doc.Pkg.Files["schema/invoice.xsd"]; ok {
		t.Fatal("altes XSD bleibt")
	}
	if tpl.Root.Get("schema", "") != "schema/rechnung-v2.xsd" {
		t.Fatalf("schema=%s", tpl.Root.Get("schema", ""))
	}
	// What remains is the removed node tax as a finding.
	var taxProblem bool
	for _, pr := range runChecksFor(c) {
		if pr.Level == "Fehler" && strings.Contains(pr.Msg, "tax") {
			taxProblem = true
		}
		// The old sample data no longer fits, that is a separate task.
		if pr.Source == "vorlage" && strings.Contains(pr.Msg, "seller") {
			t.Errorf("seller noch gemeldet: %+v", pr)
		}
	}
	if !taxProblem {
		t.Fatal("tax nicht gemeldet")
	}
	// One Undo restores everything.
	if _, err := NewTemplateService(c).Undo(); err != nil {
		t.Fatal(err)
	}
	if v := c.doc.Pkg.Template.ByID("briefkopf-firma-name").Get("bind", ""); v != "inv:seller/inv:name" {
		t.Fatalf("Undo: %q", v)
	}
	if _, ok := c.doc.Pkg.Files["schema/invoice.xsd"]; !ok {
		t.Fatal("Undo bringt das alte XSD nicht zurück")
	}
}
