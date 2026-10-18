package dataschema

import "github.com/tim-riep/pdx/template/pkg"

// Facets sind die Angaben aus dem Daten-XSD, die der Compiler für
// Formularfelder braucht, je Vorlagen-ID.
type Facets struct {
	// Enums sind die Werte der Enumeration, für Auswahlfelder ohne
	// option-Elemente.
	Enums map[string][]string
	// MaxLengths sind die Höchstlängen, für Textfelder ohne maxLength.
	MaxLengths map[string]int
}

// PackageFacets liefert die Facetten der gebundenen Elemente eines
// Vorlagenpakets. Ohne XSD oder bei Fehlern im XSD ist das Ergebnis leer,
// pdx check meldet das.
func PackageFacets(p *pkg.Package) Facets {
	out := Facets{Enums: map[string][]string{}, MaxLengths: map[string]int{}}
	main, files := p.Schemas()
	if main == "" {
		return out
	}
	m, err := FromXSD(main, files)
	if err != nil {
		return out
	}
	for id, d := range Bindings(p.Template, m) {
		if len(d.Enum) > 0 {
			out.Enums[id] = d.Enum
		}
		if d.MaxLength > 0 {
			out.MaxLengths[id] = d.MaxLength
		}
	}
	return out
}

// PackageEnums liefert nur die Enumerationen, siehe PackageFacets.
func PackageEnums(p *pkg.Package) map[string][]string { return PackageFacets(p).Enums }
