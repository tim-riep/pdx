package tree

import (
	"errors"
	"strings"
	"testing"
)

const minimal = `<?xml version="1.0"?>
<fo:root xmlns:fo="http://www.w3.org/1999/XSL/Format" xmlns:pdx="urn:pdx:extensions" xml:lang="de">
  <fo:layout-master-set>
    <fo:simple-page-master master-name="A4" page-width="210mm" page-height="297mm">
      <fo:region-body margin="20mm"/>
    </fo:simple-page-master>
  </fo:layout-master-set>
  <fo:declarations>
    <pdx:attachment src="a.xml" name="a.xml" mime-type="text/xml"/>
  </fo:declarations>
  <fo:page-sequence master-reference="A4">
    <fo:flow flow-name="xsl-region-body">
      <fo:block font-size="12pt">Hallo <fo:inline font-weight="bold">Welt</fo:inline><foreign xmlns="urn:x"><b>weg</b></foreign></fo:block>
    </fo:flow>
  </fo:page-sequence>
</fo:root>`

func TestParse(t *testing.T) {
	root, err := ParseBytes([]byte(minimal))
	if err != nil {
		t.Fatal(err)
	}
	if root.Attrs["xml:lang"] != "de" || root.Line != 2 {
		t.Errorf("Wurzel: %+v", root)
	}
	decl := root.Children[1]
	if !decl.IsFO("declarations") || len(decl.Children) != 1 || !decl.Children[0].Is(NSPDX, "attachment") {
		t.Fatalf("Erweiterung nicht übernommen: %+v", decl.Children)
	}
	block := root.Children[2].Children[0].Children[0]
	if !block.IsFO("block") || block.Attrs["font-size"] != "12pt" || block.Label() != "fo:block@13:7" {
		t.Errorf("Block: %s %v", block.Label(), block.Attrs)
	}
	if len(block.Children) != 2 || block.Children[0].Data != "Hallo " || !block.Children[1].IsFO("inline") {
		t.Errorf("Kinder des Blocks: %d", len(block.Children))
	}
}

func TestErrors(t *testing.T) {
	cases := map[string]string{
		`<x/>`: "fo:root",
		`<fo:root xmlns:fo="http://www.w3.org/1999/XSL/Format"><fo:table/></fo:root>`:            "fo:table wird in Phase 0 nicht unterstützt",
		`<fo:root xmlns:fo="http://www.w3.org/1999/XSL/Format"><fo:block/></fo:root>`:            "fo:block ist in fo:root nicht erlaubt",
		`<fo:root xmlns:fo="http://www.w3.org/1999/XSL/Format">Text</fo:root>`:                   "keinen Text",
		`<fo:root xmlns:fo="http://www.w3.org/1999/XSL/Format"><fo:layout-master-set></fo:root>`: "nicht wohlgeformt",
		``: "leer",
	}
	for in, want := range cases {
		_, err := ParseBytes([]byte(in))
		var pe *Error
		if !errors.As(err, &pe) || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: erwartet %q, erhalten %v", in, want, err)
		}
	}
}

func FuzzParse(f *testing.F) {
	f.Add([]byte(minimal))
	f.Add([]byte(`<fo:root xmlns:fo="http://www.w3.org/1999/XSL/Format"/>`))
	f.Add([]byte(`<?xml version="1.0"?><!DOCTYPE x [<!ENTITY a "b">]><fo:root xmlns:fo="http://www.w3.org/1999/XSL/Format">&a;</fo:root>`))
	f.Fuzz(func(t *testing.T, data []byte) {
		root, err := ParseBytes(data)
		if err != nil {
			return
		}
		if !root.IsFO("root") {
			t.Fatal("Wurzel ist nicht fo:root")
		}
	})
}
