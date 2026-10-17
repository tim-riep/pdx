package dataschema

import (
	"strings"
	"testing"

	"github.com/tim-riep/pdx/template"
	"github.com/tim-riep/pdx/template/pkg"
)

const musterDir = "../testdata/templates/musterrechnung"

func load(t *testing.T) (*pkg.Package, Model) {
	t.Helper()
	p, err := pkg.ReadDir(musterDir)
	if err != nil {
		t.Fatal(err)
	}
	main, files := p.Schemas()
	m, err := FromXSD(main, files)
	if err != nil {
		t.Fatal(err)
	}
	return p, m
}

func TestMusterrechnungClean(t *testing.T) {
	p, m := load(t)
	if fs := Check(p.Template, m); len(fs) > 0 {
		t.Fatalf("Befunde in der Musterrechnung: %v", fs)
	}
}

// TestBroken ändert die Musterrechnung gezielt und erwartet je einen Fehler
// mit der ID des geänderten Elements.
func TestBroken(t *testing.T) {
	p, m := load(t)
	src := template.Write(p.Template)
	cases := []struct {
		name, old, new, id, msg string
	}{
		{"pfad", `bind="inv:seller/inv:name"/></text>`, `bind="inv:seller/inv:nam"/></text>`, "briefkopf-firma-name", "inv:nam gibt es unter inv:seller nicht"},
		{"attribut", `bind="inv:quantity" format="menge"`, `bind="inv:quantity/@einheit" format="menge"`, "z-menge-f", "Attribut einheit"},
		{"zahlformat", `bind="inv:description"`, `bind="inv:description" format="betrag"`, "z-leistung-f", "Zahlenformat betrag"},
		{"datumsformat", `bind="inv:issueDate" format="datum"`, `bind="inv:totals/inv:net" format="datum"`, "rechnungsdatum-wert", "erwartet ist xs:date"},
		{"kardinalität", `bind="inv:number" fontWeight`, `bind="inv:items/inv:item/inv:amount" fontWeight`, "rechnungsnummer-wert", "kann mehrfach vorkommen"},
		{"ohne text", `bind="inv:number" fontWeight`, `bind="inv:seller" fontWeight`, "rechnungsnummer-wert", "keinen Textinhalt"},
		{"teilformular", `id="empfaenger" bind="inv:buyer"`, `id="empfaenger" bind="inv:items/inv:item"`, "empfaenger", "repeat"},
		{"summe", `id="u-carry" format="betrag" of="inv:amount"`, `id="u-carry" format="betrag" of="inv:description"`, "u-carry", "nur Zahlen"},
		{"datenwurzel", `dataRoot="inv:invoice"`, `dataRoot="inv:rechnung"`, "musterrechnung", "kein globales Element"},
		{"präfix", `bind="inv:position"`, `bind="x:position"`, "z-pos-f", "Präfix"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			changed := strings.Replace(string(src), tc.old, tc.new, 1)
			if changed == string(src) {
				t.Fatalf("%q nicht in der Vorlage", tc.old)
			}
			tpl, err := template.Parse([]byte(changed))
			if err != nil {
				t.Fatal(err)
			}
			fs := Check(tpl, m)
			if !HasErrors(fs) {
				t.Fatalf("kein Fehler, Befunde %v", fs)
			}
			for _, f := range fs {
				if f.Level == Error && f.ID == tc.id && strings.Contains(f.Msg, tc.msg) {
					return
				}
			}
			t.Fatalf("Befunde %v, erwartet [%s] mit %q", fs, tc.id, tc.msg)
		})
	}
}

func TestSplitPath(t *testing.T) {
	cases := []struct {
		in   string
		ok   bool
		want string
	}{
		{"inv:a/inv:b", true, "c:inv:a c:inv:b"},
		{"/inv:a/@x", true, "c:inv:a a:x"},
		{"../inv:b[1]", true, "p: c:inv:b[i]"},
		{"inv:a[@k = 'x/y']/inv:b", true, "c:inv:a[f] c:inv:b"},
		{".", true, "s:"},
		{"inv:a//inv:b", false, ""},
		{"count(inv:a)", false, ""},
		{"inv:a | inv:b", false, ""},
		{"*", false, ""},
		{"inv:a[", false, ""},
	}
	for _, tc := range cases {
		steps, abs, ok := splitPath(tc.in)
		if ok != tc.ok {
			t.Errorf("%q: ok=%v", tc.in, ok)
			continue
		}
		if !ok {
			continue
		}
		var parts []string
		for _, s := range steps {
			p := map[stepKind]string{stepChild: "c", stepAttr: "a", stepSelf: "s", stepParent: "p"}[s.kind] + ":" + s.name
			p += map[predKind]string{predNone: "", predIndex: "[i]", predFilter: "[f]"}[s.pred]
			parts = append(parts, p)
		}
		if got := strings.Join(parts, " "); got != tc.want {
			t.Errorf("%q: %q, erwartet %q", tc.in, got, tc.want)
		}
		if abs != strings.HasPrefix(tc.in, "/") {
			t.Errorf("%q: abs=%v", tc.in, abs)
		}
	}
}
