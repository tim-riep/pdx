package dataschema

import "github.com/tim-riep/pdx/template/pkg"

// PackageEnums liefert für die gebundenen Elemente eines Vorlagenpakets die
// Enumeration ihres Datenknotens aus dem Daten-XSD, je Vorlagen-ID. Der
// Compiler bietet sie in Auswahlfeldern ohne option-Elemente an. Ohne XSD
// oder bei Fehlern im XSD ist das Ergebnis leer, pdx check meldet das.
func PackageEnums(p *pkg.Package) map[string][]string {
	main, files := p.Schemas()
	if main == "" {
		return nil
	}
	m, err := FromXSD(main, files)
	if err != nil {
		return nil
	}
	out := map[string][]string{}
	for id, d := range Bindings(p.Template, m) {
		if len(d.Enum) > 0 {
			out[id] = d.Enum
		}
	}
	return out
}
