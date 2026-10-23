package dataxml

import (
	"encoding/xml"
	"strings"
	"testing"
)

const ns = "https://github.com/tim-riep/pdx/ns/examples/antrag"

func TestRoundTrip(t *testing.T) {
	src := `<?xml version="1.0" encoding="UTF-8"?>
<!-- Kopf -->
<a:antrag xmlns:a="` + ns + `" xmlns:x="urn:x" x:id="1"><a:name>Erika &amp; Max</a:name><a:leer/></a:antrag>
`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Root.Name != (xml.Name{Space: ns, Local: "antrag"}) {
		t.Fatalf("Wurzel %v", doc.Root.Name)
	}
	if v, ok := doc.Root.Attr(xml.Name{Space: "urn:x", Local: "id"}); !ok || v != "1" {
		t.Errorf("Attribut %q", v)
	}
	name := doc.Root.Elements(xml.Name{Space: ns, Local: "name"})[0]
	if name.Text() != "Erika & Max" {
		t.Errorf("Text %q", name.Text())
	}
	name.SetText("Anna <B>")
	n := &Node{Kind: Element, Name: xml.Name{Space: ns, Local: "email"}}
	n.SetText("a@b.example")
	doc.Root.Insert(n, 1)
	z := &Node{Kind: Element, Name: xml.Name{Space: "urn:neu", Local: "z"}}
	doc.Root.Insert(z, 99)
	got := string(doc.Bytes(map[string]string{"urn:neu": "n"}))
	want := `<?xml version="1.0" encoding="UTF-8"?>
<!-- Kopf -->
<a:antrag xmlns:a="` + ns + `" xmlns:x="urn:x" x:id="1"><a:name>Anna &lt;B&gt;</a:name><a:email>a@b.example</a:email><a:leer/><n:z xmlns:n="urn:neu"/></a:antrag>
`
	if got != want {
		t.Errorf("\n%s\nerwartet\n%s", got, want)
	}
	// The default namespace stays without a prefix.
	doc, _ = Parse([]byte(`<antrag xmlns="` + ns + `"><name>x</name></antrag>`))
	if got := string(doc.Bytes(nil)); !strings.Contains(got, `<antrag xmlns="`+ns+`"><name>x</name></antrag>`) {
		t.Errorf("Standard-Namespace: %s", got)
	}
}
