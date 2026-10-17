// Package check prüft ein Vorlagenpaket wie pdx check: Schema der Vorlage,
// Verweise im Paket, Übersetzung, Bindungen gegen das Daten-XSD und die
// Beispieldaten. CLI und Designer nutzen dieselben Prüfungen.
package check

import (
	"errors"
	"fmt"
	"strings"

	"github.com/tim-riep/go-xslt/engine"
	"github.com/tim-riep/pdx/compile"
	"github.com/tim-riep/pdx/dataschema"
	"github.com/tim-riep/pdx/template"
	"github.com/tim-riep/pdx/template/pkg"
)

// Path öffnet ein Paket (ZIP oder Verzeichnis) und prüft es. Fehler beim
// Öffnen erscheinen als Befunde.
func Path(path string) []dataschema.Finding {
	p, err := pkg.Open(path)
	if err != nil {
		return FromError(err)
	}
	return Package(p)
}

// Package prüft ein geladenes Paket.
func Package(p *pkg.Package) []dataschema.Finding {
	var findings []dataschema.Finding
	if _, err := compile.Compile(p.Template, compile.Options{}); err != nil {
		findings = append(findings, FromError(err)...)
	}
	main, files := p.Schemas()
	if main == "" {
		return append(findings, dataschema.Finding{Level: dataschema.Warning, ID: p.Template.Root.ID(),
			Msg: "die Vorlage nennt kein Daten-XSD (template/@schema), Bindungen bleiben ungeprüft"})
	}
	m, err := dataschema.FromXSD(main, files)
	if err != nil {
		return append(findings, dataschema.Finding{Level: dataschema.Error, ID: p.Template.Root.ID(), Msg: err.Error()})
	}
	findings = append(findings, dataschema.Check(p.Template, m)...)
	return append(findings, Samples(p, main, files)...)
}

// FromError übersetzt Fehler aus Vorlage, Paket und Compiler in Befunde mit
// Vorlagen-ID, soweit bekannt.
func FromError(err error) []dataschema.Finding {
	var es template.Errors
	var e *template.Error
	switch {
	case errors.As(err, &es):
		out := make([]dataschema.Finding, 0, len(es))
		for _, e := range es {
			out = append(out, dataschema.Finding{Level: dataschema.Error, ID: e.ID, Line: e.Line, Col: e.Col, Msg: e.Msg})
		}
		return out
	case errors.As(err, &e):
		return []dataschema.Finding{{Level: dataschema.Error, ID: e.ID, Line: e.Line, Col: e.Col, Msg: e.Msg}}
	default:
		return []dataschema.Finding{{Level: dataschema.Error, Msg: err.Error()}}
	}
}

// Samples prüft die Beispieldaten unter samples/ gegen das Daten-XSD.
func Samples(p *pkg.Package, main string, files map[string][]byte) []dataschema.Finding {
	var out []dataschema.Finding
	docs := []string{string(files[main])}
	for _, n := range p.Names() {
		if n != main && strings.HasPrefix(n, "schema/") && strings.HasSuffix(n, ".xsd") {
			docs = append(docs, string(files[n]))
		}
	}
	for _, s := range p.Samples() {
		if !strings.HasSuffix(s, ".xml") {
			continue
		}
		b, _ := p.File(s)
		res := engine.Validate(engine.ValidateRequest{Schemas: docs, Instance: string(b)})
		if res.Valid {
			continue
		}
		for _, d := range res.Diagnostics {
			if d.Severity == engine.SeverityError {
				out = append(out, dataschema.Finding{Level: dataschema.Error,
					Msg: fmt.Sprintf("%s %d:%d entspricht nicht %s: %s", s, d.Line, d.Col, main, d.Message)})
			}
		}
	}
	return out
}
