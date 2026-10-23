package model

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Patterns from the XSD of the template format (Length, Lengths, Color), each
// also with a reference to a token such as $primaer.
const tokenRef = `\$[A-Za-z_][A-Za-z0-9_.\-]*`

var (
	lengthRe  = regexp.MustCompile(`^(-?[0-9]+(\.[0-9]+)?(pt|mm|cm|in|pc|px|%)|0|` + tokenRef + `)$`)
	columnRe  = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?(pt|mm|cm|in|pc|px|%|\*)$`)
	oneLength = `(-?[0-9]+(\.[0-9]+)?(pt|mm|cm|in|pc|px|%)?|` + tokenRef + `)`
	lengthsRe = regexp.MustCompile(`^` + oneLength + `( ` + oneLength + `){0,3}$`)
	colorRe   = regexp.MustCompile(`^(#[0-9a-fA-F]{3}([0-9a-fA-F]{3})?|[a-z]+|` + tokenRef + `)$`)
	tokenRe   = regexp.MustCompile(`^` + tokenRef + `$`)
	numberRe  = regexp.MustCompile(`^[0-9]+$`)
)

// checkValue checks a value against the kind of the property and returns an
// understandable message. The XSD check afterwards remains the binding one.
func checkValue(p Property, v string) error {
	if v == "" {
		if p.Required {
			return fmt.Errorf("%s ist eine Pflichtangabe", p.Name)
		}
		return nil
	}
	ok := true
	hint := ""
	switch p.Kind {
	case "length":
		ok = lengthRe.MatchString(v) || p.Name == "width" && columnRe.MatchString(v)
		hint = "eine Länge wie 20mm oder 12pt oder ein Token wie $abstand"
	case "lengths":
		ok = lengthsRe.MatchString(v)
		hint = "ein bis vier Längen wie 5mm oder 5mm 10mm"
	case "color":
		ok = colorRe.MatchString(v)
		hint = "eine Farbe wie #336699 oder ein Token wie $primaer"
	case "font":
		ok = tokenRe.MatchString(v)
		hint = "ein Schrift-Token wie $fliesstext"
	case "enum":
		ok = slices.Contains(p.Enum, v)
		hint = "einer der Werte " + strings.Join(p.Enum, ", ")
	case "bool":
		ok = v == "true" || v == "false"
		hint = "true oder false"
	case "number":
		ok = numberRe.MatchString(v)
		hint = "eine ganze Zahl"
	case "id", "ref":
		ok = idRe.MatchString(v)
		hint = "ein Name aus Buchstaben, Ziffern, Bindestrich, Punkt oder Unterstrich, beginnend mit einem Buchstaben"
	}
	if !ok {
		return fmt.Errorf("%s: „%s“ ist ungültig, erwartet ist %s", p.Name, v, hint)
	}
	return nil
}

var idRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.\-]*$`)
