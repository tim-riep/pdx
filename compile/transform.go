package compile

import (
	"fmt"

	"github.com/tim-riep/go-xslt/engine"
	"github.com/tim-riep/pdx/internal/xmlsafe"
	"github.com/tim-riep/pdx/template"
)

// check compiles the stylesheet. Errors, such as invalid XPath expressions in
// bindings, are thus reported with the template ID already at compile time.
// The compiled stylesheet serves all subsequent transformations.
func (r *Result) check() error {
	sheet, diags := engine.Compile(engine.Request{Stylesheet: string(r.XSLT)})
	var errs template.Errors
	for _, d := range diags {
		if d.Severity == engine.SeverityError {
			errs = append(errs, r.diagError(d, "XSLT"))
		}
	}
	if len(errs) > 0 || sheet == nil {
		if len(errs) == 0 {
			errs = append(errs, &template.Error{Msg: "XSLT ließ sich nicht kompilieren"})
		}
		return errs
	}
	r.sheet = sheet
	return nil
}

// Transform applies the stylesheet to the data and returns XSL-FO.
// Runtime errors carry the ID of the template element from which the
// location in the stylesheet originates. Transform may be called
// concurrently.
func (r *Result) Transform(data []byte) ([]byte, error) {
	if err := xmlsafe.Check(data); err != nil {
		return nil, fmt.Errorf("Daten: %w", err)
	}
	res := r.sheet.Transform(engine.TransformRequest{Source: string(data)})
	var errs template.Errors
	for _, d := range res.Diagnostics {
		if d.Severity != engine.SeverityError {
			continue
		}
		if d.Phase == "parse" {
			// The stylesheet is validated, read errors concern the data.
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
