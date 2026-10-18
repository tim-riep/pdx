package profile

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/tim-riep/pdx/pdf"
)

// Erlaubte Aktionen je PDF/A-Teil. Quelle sind ISO 19005-2 und 19005-3
// Abschnitt 6.5 sowie ISO 19005-4 Abschnitt 6.6, in der Fassung der Regeln von
// veraPDF 1.30.3 (Profile PDFA-2B bis PDFA-4F). JavaScript erlaubt PDF/A-4
// zwar, pdx schreibt es aber nie (pdf.alwaysForbidden).
var (
	actionsA23 = []pdf.Name{"GoTo", "GoToR", "GoToE", "Thread", "URI", "Named", "SubmitForm"}
	actionsA4  = []pdf.Name{"GoTo", "GoToR", "GoToE", "Thread", "URI", "Named", "SubmitForm", "JavaScript", "RichMediaExecute", "GoToDp"}
	// namedActions sind die erlaubten Werte von N (ISO 19005-2 6.5.1,
	// ISO 19005-4 6.6.1).
	namedActions = []pdf.Name{"NextPage", "PrevPage", "FirstPage", "LastPage"}
	// aaKeysA4 sind die erlaubten Auslöser in AA an Katalog, Seite und
	// Annotationen außer Widgets (ISO 19005-4 6.6.3).
	aaKeysA4 = []pdf.Name{"E", "X", "D", "U", "Fo", "Bl"}
)

func names(ns []pdf.Name) string {
	s := make([]string, len(ns))
	for i, n := range ns {
		s[i] = string(n)
	}
	return strings.Join(s, ", ")
}

// pdfaAllowAction prüft eine Aktion gegen den PDF/A-Teil.
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

// pdfaAllowAA prüft ein AA-Dictionary gegen den PDF/A-Teil.
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

// pdfaAllowWidgetAction verbietet den Eintrag A an Widgets (ISO 19005-2 6.4.1,
// ISO 19005-4 6.4.1).
func pdfaAllowWidgetAction(name string, part int) error {
	return fmt.Errorf("Profil %s: ISO 19005-%d 6.4.1 verbietet Aktionen (A) an Formularfeldern", name, part)
}

var ownerLabel = map[pdf.Name]string{
	"Catalog": "Katalog", "Page": "Seiten", "Widget": "Formularfeldern", "Field": "Formularfeldern", "Annot": "Annotationen",
}

// AllowAction prüft eine Aktion.
func (p PDFA) AllowAction(s, named pdf.Name) error {
	return pdfaAllowAction(p.Name(), p.Part, s, named)
}

// AllowAdditionalActions prüft ein AA-Dictionary.
func (p PDFA) AllowAdditionalActions(owner pdf.Name, keys []pdf.Name) error {
	return pdfaAllowAA(p.Name(), p.Part, owner, keys)
}

// AllowWidgetAction verbietet Aktionen an Formularfeldern.
func (p PDFA) AllowWidgetAction() error { return pdfaAllowWidgetAction(p.Name(), p.Part) }

// AllowAction prüft eine Aktion.
func (p PDFA3B) AllowAction(s, named pdf.Name) error { return pdfaAllowAction(p.Name(), 3, s, named) }

// AllowAdditionalActions prüft ein AA-Dictionary.
func (p PDFA3B) AllowAdditionalActions(owner pdf.Name, keys []pdf.Name) error {
	return pdfaAllowAA(p.Name(), 3, owner, keys)
}

// AllowWidgetAction verbietet Aktionen an Formularfeldern.
func (p PDFA3B) AllowWidgetAction() error { return pdfaAllowWidgetAction(p.Name(), 3) }

// AllowAction fragt jedes Profil der Kombination, das Aktionen prüft.
func (s *Set) AllowAction(t, named pdf.Name) error {
	return s.each(func(a pdf.ActionPolicy) error { return a.AllowAction(t, named) })
}

// AllowAdditionalActions fragt jedes Profil der Kombination.
func (s *Set) AllowAdditionalActions(owner pdf.Name, keys []pdf.Name) error {
	return s.each(func(a pdf.ActionPolicy) error { return a.AllowAdditionalActions(owner, keys) })
}

// AllowWidgetAction fragt jedes Profil der Kombination.
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
