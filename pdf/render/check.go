package render

import (
	"fmt"
	"strings"

	"github.com/tim-riep/pdx/pdf/profile"
)

// CheckError ist ein Befund der Vorprüfung für Tagged PDF mit der
// Vorlagen-ID des Elements, leer für das ganze Dokument.
type CheckError struct {
	ID  string
	Msg string
}

func (e *CheckError) Error() string {
	if e.ID == "" {
		return e.Msg
	}
	return "[" + e.ID + "] " + e.Msg
}

// CheckErrors sind alle Befunde der Vorprüfung in Dokumentreihenfolge.
type CheckErrors []*CheckError

func (es CheckErrors) Error() string {
	parts := make([]string, len(es))
	for i, e := range es {
		parts[i] = e.Error()
	}
	return strings.Join(parts, "\n")
}

// check prüft vor dem Schreiben, was PDF/A Stufe A und PDF/UA verlangen und
// nicht aufgefüllt werden darf. Beide verlangen Sprache und
// Alternativtexte, PDF/UA zusätzlich Titel, Kopfzellen und getaggte
// Verweise, PDF/UA-1 außerdem lückenlose Überschriften und Verweise mit
// Text. Alle Befunde kommen auf einmal.
func (t *tagger) check(c profile.Claim) error {
	var errs CheckErrors
	seen := map[string]bool{}
	page := -1
	add := func(id, format string, args ...any) {
		e := &CheckError{ID: id, Msg: fmt.Sprintf(format, args...)}
		// Ohne Vorlagen-ID hilft die Seite beim Finden.
		if id == "" && page >= 0 {
			e.Msg += fmt.Sprintf(" (Seite %d)", page+1)
		}
		// Ein Element über mehrere Seiten meldet sich nur einmal.
		if id != "" {
			if seen[e.Error()] {
				return
			}
			seen[e.Error()] = true
		}
		errs = append(errs, e)
	}
	// clause nennt die Fundstellen im beanspruchten Standard: ua1 und ua2
	// für PDF/UA-1 und PDF/UA-2, pdfa für PDF/A Stufe A. Leer heißt, der
	// beanspruchte Standard verlangt es nicht.
	clause := func(ua1, ua2, pdfa string) string {
		var cs []string
		switch {
		case c.UA == 1 && ua1 != "":
			cs = append(cs, "ISO 14289-1 "+ua1)
		case c.UA == 2 && ua2 != "":
			cs = append(cs, "ISO 14289-2 "+ua2)
		}
		if c.PDFA > 0 && c.Level == "A" && pdfa != "" {
			cs = append(cs, fmt.Sprintf("ISO 19005-%d %s", c.PDFA, pdfa))
		}
		return strings.Join(cs, ", ")
	}
	m := t.doc.Meta
	if cl := clause("7.1", "8.11.1", ""); cl != "" && strings.TrimSpace(m.Title) == "" {
		add("", "Dokumenttitel fehlt (template/@title oder fo:declarations), %s verlangt dc:title", cl)
	}
	if cl := clause("7.2", "8.4.4", "6.7"); cl != "" && strings.TrimSpace(m.Lang) == "" {
		add("", "Dokumentsprache fehlt (template/@lang oder xml:lang am fo:root), %s", cl)
	}
	level := 0
	var walk func(se *structElem)
	walk = func(se *structElem) {
		page = -1
		if se.located {
			page = se.page
		}
		switch se.typ {
		case "Figure", "Formula":
			if cl := clause("7.3", "8.2.5.28.2", "6.7"); cl != "" && strings.TrimSpace(se.alt) == "" {
				add(se.id, "%s ohne Alternativtext (alt oder altBind), %s", typeLabel[se.typ], cl)
			}
		case "H1", "H2", "H3", "H4", "H5", "H6":
			// Nur PDF/UA-1 verlangt lückenlose Überschriften.
			cl := clause("7.4.2", "", "")
			if cl == "" {
				break
			}
			n := int(se.typ[1] - '0')
			switch {
			case level == 0 && n != 1:
				add(se.id, "erste Überschrift ist %s statt H1, %s", se.typ, cl)
			case n > level+1 && level > 0:
				add(se.id, "Überschrift %s folgt auf H%d, Ebene H%d fehlt, %s", se.typ, level, level+1, cl)
			}
			level = n
		case "Table":
			if cl := clause("7.5", "8.2.5.26", ""); cl != "" && !hasType(se, "TH") {
				add(se.id, "Tabelle ohne Kopfzellen (Kopfbereich oder scope an Zellen), %s", cl)
			}
		case "Link":
			if cl := clause("7.18.5", "", ""); cl != "" && !hasText(se) {
				add(se.id, "Verweis ohne Text, %s verlangt eine Beschreibung", cl)
			}
		}
		for _, k := range se.kids {
			if k.elem != nil {
				walk(k.elem)
			}
		}
	}
	walk(t.root)
	page = -1
	if cl := clause("7.18.1", "8.2.5.20", ""); cl != "" {
		for _, l := range t.artifactLinks {
			add(l, "Verweis in Kopf, Fuß oder Artefakt, %s verlangt, dass jede Annotation im Strukturbaum steht", cl)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

var typeLabel = map[string]string{"Figure": "Abbildung", "Formula": "Formel"}

func hasType(se *structElem, typ string) bool {
	for _, k := range se.kids {
		if k.elem != nil && (k.elem.typ == typ || hasType(k.elem, typ)) {
			return true
		}
	}
	return false
}

func hasText(se *structElem) bool {
	for _, k := range se.kids {
		if k.elem == nil && k.annot == nil {
			return true
		}
		if k.elem != nil && hasText(k.elem) {
			return true
		}
	}
	return false
}
