package dataschema

import (
	"slices"
	"testing"
)

const enumXSD = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:simpleType name="Waehrung">
    <xs:restriction base="xs:string">
      <xs:enumeration value="EUR"/>
      <xs:enumeration value="USD"/>
    </xs:restriction>
  </xs:simpleType>
  <xs:element name="betrag">
    <xs:complexType>
      <xs:simpleContent>
        <xs:extension base="xs:decimal">
          <xs:attribute name="waehrung" type="Waehrung" use="required"/>
        </xs:extension>
      </xs:simpleContent>
    </xs:complexType>
  </xs:element>
  <xs:element name="art" type="Waehrung"/>
</xs:schema>`

// TestEnum checks that enumerations arrive at elements and attributes.
func TestEnum(t *testing.T) {
	m, err := FromXSD("a.xsd", map[string][]byte{"a.xsd": []byte(enumXSD)})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"EUR", "USD"}
	for _, r := range m.Roots() {
		switch r.Name.Local {
		case "art":
			if !slices.Equal(r.Enum, want) {
				t.Errorf("art: Enum %v, erwartet %v", r.Enum, want)
			}
		case "betrag":
			if r.Enum != nil {
				t.Errorf("betrag: Enum %v, erwartet keine", r.Enum)
			}
			kids, _ := m.Children(r)
			if len(kids) != 1 || !slices.Equal(kids[0].Enum, want) {
				t.Errorf("betrag/@waehrung: %+v", kids)
			}
		}
	}
}
