package profile

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/tim-riep/pdx/pdf"
)

// Allowed actions per PDF/A part. Sources are ISO 19005-2 and 19005-3
// section 6.5 and ISO 19005-4 section 6.6, as interpreted by the rules of
// veraPDF 1.30.3 (profiles PDFA-2B to PDFA-4F). PDF/A-4 does allow
// JavaScript, but pdx never writes it (pdf.alwaysForbidden).
var (
	actionsA23 = []pdf.Name{"GoTo", "GoToR", "GoToE", "Thread", "URI", "Named", "SubmitForm"}
	actionsA4  = []pdf.Name{"GoTo", "GoToR", "GoToE", "Thread", "URI", "Named", "SubmitForm", "JavaScript", "RichMediaExecute", "GoToDp"}
	// namedActions are the allowed values of N (ISO 19005-2 6.5.1,
	// ISO 19005-4 6.6.1).
	namedActions = []pdf.Name{"NextPage", "PrevPage", "FirstPage", "LastPage"}
	// aaKeysA4 are the allowed triggers in AA on catalog, page and
	// annotations other than widgets (ISO 19005-4 6.6.3).
	aaKeysA4 = []pdf.Name{"E", "X", "D", "U", "Fo", "Bl"}
)

func names(ns []pdf.Name) string {
	s := make([]string, len(ns))
	for i, n := range ns {
		s[i] = string(n)
	}
	return strings.Join(s, ", ")
}

// pdfaAllowAction checks an action against the PDF/A part.
func pdfaAllowAction(name string, part int, s, named pdf.Name) error {
	allowed, clause := actionsA23, fmt.Sprintf("ISO 19005-%d 6.5.1", part)
	if part == 4 {
		allowed, clause = actionsA4, "ISO 19005-4 6.6.1"
	}
	if !slices.Contains(allowed, s) {
		return fmt.Errorf("Profil %s: Aktion %s ist verboten, %s erlaubt nur %s", name, s, clause, names(allowed))
	}
	if s == "Named" && !slices.Contains(namedActions, named) {
		return fmt.Errorf("Profil %s: Named-Aktion %s ist verboten, %s erlaubt nur %s", name, named, clause, names(namedActions))
	}
	return nil
}

// pdfaAllowAA checks an AA dictionary against the PDF/A part.
func pdfaAllowAA(name string, part int, owner pdf.Name, keys []pdf.Name) error {
	if part != 4 {
		switch owner {
		case "Catalog", "Page":
			return fmt.Errorf("Profil %s: ISO 19005-%d 6.5.2 verbietet Zusatzaktionen (AA) an %s", name, part, ownerLabel[owner])
		case "Widget", "Field":
			return fmt.Errorf("Profil %s: ISO 19005-%d 6.4.1 verbietet Zusatzaktionen (AA) an %s", name, part, ownerLabel[owner])
		}
		return nil
	}
	if owner == "Widget" || owner == "Field" {
		return nil
	}
	var bad []pdf.Name
	for _, k := range keys {
		if !slices.Contains(aaKeysA4, k) {
			bad = append(bad, k)
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("Profil %s: Auslöser %s in AA an %s sind verboten, ISO 19005-4 6.6.3 erlaubt nur %s", name, names(bad), ownerLabel[owner], names(aaKeysA4))
	}
	return nil
}

// pdfaAllowWidgetAction forbids the entry A on widgets (ISO 19005-2 6.4.1,
// ISO 19005-4 6.4.1).
func pdfaAllowWidgetAction(name string, part int) error {
	return fmt.Errorf("Profil %s: ISO 19005-%d 6.4.1 verbietet Aktionen (A) an Formularfeldern", name, part)
}

var ownerLabel = map[pdf.Name]string{
	"Catalog": "Katalog", "Page": "Seiten", "Widget": "Formularfeldern", "Field": "Formularfeldern", "Annot": "Annotationen",
}

// AllowAction checks an action.
func (p PDFA) AllowAction(s, named pdf.Name) error {
	return pdfaAllowAction(p.Name(), p.Part, s, named)
}

// AllowAdditionalActions checks an AA dictionary.
func (p PDFA) AllowAdditionalActions(owner pdf.Name, keys []pdf.Name) error {
	return pdfaAllowAA(p.Name(), p.Part, owner, keys)
}

// AllowWidgetAction forbids actions on form fields.
func (p PDFA) AllowWidgetAction() error { return pdfaAllowWidgetAction(p.Name(), p.Part) }

// AllowAction checks an action.
func (p PDFA3B) AllowAction(s, named pdf.Name) error { return pdfaAllowAction(p.Name(), 3, s, named) }

// AllowAdditionalActions checks an AA dictionary.
func (p PDFA3B) AllowAdditionalActions(owner pdf.Name, keys []pdf.Name) error {
	return pdfaAllowAA(p.Name(), 3, owner, keys)
}

// AllowWidgetAction forbids actions on form fields.
func (p PDFA3B) AllowWidgetAction() error { return pdfaAllowWidgetAction(p.Name(), 3) }

// AllowAction asks every profile of the combination that checks actions.
func (s *Set) AllowAction(t, named pdf.Name) error {
	return s.each(func(a pdf.ActionPolicy) error { return a.AllowAction(t, named) })
}

// AllowAdditionalActions asks every profile of the combination.
func (s *Set) AllowAdditionalActions(owner pdf.Name, keys []pdf.Name) error {
	return s.each(func(a pdf.ActionPolicy) error { return a.AllowAdditionalActions(owner, keys) })
}

// AllowWidgetAction asks every profile of the combination.
func (s *Set) AllowWidgetAction() error {
	return s.each(func(a pdf.ActionPolicy) error { return a.AllowWidgetAction() })
}

func (s *Set) each(f func(pdf.ActionPolicy) error) error {
	var errs []error
	for _, p := range s.parts {
		if a, ok := p.(pdf.ActionPolicy); ok {
			if err := f(a); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}
