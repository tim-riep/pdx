package compile

import (
	"fmt"

	"github.com/tim-riep/go-xslt/engine"
	"github.com/tim-riep/pdx/template"
)

// check übersetzt das Stylesheet probeweise mit leeren Daten. Fehler beim
// Lesen und Übersetzen, etwa ungültige XPath-Ausdrücke in Bindungen, werden
// so schon beim Kompilieren mit Vorlagen-ID gemeldet.
func (r *Result) check() error {
	res := engine.Transform(engine.Request{Stylesheet: string(r.XSLT), Source: "<pdx-probe/>"})
	var errs template.Errors
	for _, d := range res.Diagnostics {
		if d.Severity != engine.SeverityError || d.Phase == "run" {
			continue
		}
		errs = append(errs, r.diagError(d, "XSLT"))
	}
	if len(errs) > 0 {
		return errs
	}
	return nil
}

// Transform wendet das Stylesheet auf die Daten an und liefert XSL-FO.
// Laufzeitfehler tragen die ID des Vorlagenelements, aus dem die Stelle im
// Stylesheet stammt.
func (r *Result) Transform(data []byte) ([]byte, error) {
	res := engine.Transform(engine.Request{Stylesheet: string(r.XSLT), Source: string(data)})
	var errs template.Errors
	for _, d := range res.Diagnostics {
		if d.Severity != engine.SeverityError {
			continue
		}
		if d.Phase == "parse" {
			// Das Stylesheet ist geprüft, Lesefehler betreffen die Daten.
			errs = append(errs, &template.Error{Msg: fmt.Sprintf("Daten %d:%d: %s", d.Line, d.Col, d.Message)})
			continue
		}
		errs = append(errs, r.diagError(d, "Transformation"))
	}
	if len(errs) > 0 {
		return nil, errs
	}
	return []byte(res.Output), nil
}

func (r *Result) diagError(d engine.Diagnostic, what string) *template.Error {
	msg := what + ": " + d.Message
	if d.Code != "" {
		msg = what + " (" + d.Code + "): " + d.Message
	}
	return &template.Error{ID: r.IDAt(d.Line), Msg: msg}
}
