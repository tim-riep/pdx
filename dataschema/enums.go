package dataschema

import "github.com/tim-riep/pdx/template/pkg"

// Facets are the details from the data XSD that the compiler needs for
// form fields, per template ID.
type Facets struct {
	// Enums are the values of the enumeration, for choice fields without
	// option elements.
	Enums map[string][]string
	// MaxLengths are the maximum lengths, for text fields without maxLength.
	MaxLengths map[string]int
}

// PackageFacets returns the facets of the bound elements of a template
// package. Without an XSD or with errors in the XSD the result is empty,
// pdx check reports that.
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

// PackageEnums returns only the enumerations, see PackageFacets.
func PackageEnums(p *pkg.Package) map[string][]string { return PackageFacets(p).Enums }
