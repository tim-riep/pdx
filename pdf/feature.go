package pdf

import "fmt"

// Feature is a PDF feature whose use a conformance profile must
// allow. The writer asks through Document.Use before every use.
type Feature int

const (
	// Transparency stands for soft masks, constant alpha and blend modes.
	Transparency Feature = iota + 1
	// Encryption stands for an Encrypt dictionary.
	Encryption
	// LZW stands for the LZWDecode filter.
	LZW
	// ExternalContent stands for streams or files outside the PDF.
	ExternalContent
	// JavaScript stands for JavaScript actions and name trees.
	JavaScript
	// XFA stands for XFA forms.
	XFA
	// Attachment stands for embedded files.
	Attachment
	// UnembeddedFont stands for a font that is not embedded.
	UnembeddedFont
	// UnmappedGlyph stands for a glyph without a Unicode mapping in the
	// ToUnicode CMap of its font.
	UnmappedGlyph
	// NeedAppearances stands for the entry NeedAppearances true in the
	// AcroForm dictionary, which lets the viewer generate appearances.
	NeedAppearances
)

var featureNames = map[Feature]string{
	Transparency:    "Transparenz",
	Encryption:      "Verschlüsselung",
	LZW:             "LZW-Kompression",
	ExternalContent: "externe Inhalte",
	JavaScript:      "JavaScript",
	XFA:             "XFA",
	Attachment:      "Dateianhänge",
	UnembeddedFont:  "nicht eingebettete Schriften",
	UnmappedGlyph:   "Glyphen ohne Unicode-Zuordnung",
	NeedAppearances: "NeedAppearances",
}

func (f Feature) String() string {
	if s, ok := featureNames[f]; ok {
		return s
	}
	return fmt.Sprintf("Feature(%d)", int(f))
}

// Policy decides whether a feature may be used. Conformance profiles
// in pdf/profile implement this interface.
type Policy interface {
	// Name is the profile name for error messages, for example "pdfa-3b".
	Name() string
	// Allow returns nil if the feature is allowed, otherwise an error
	// that names the violated rule.
	Allow(f Feature) error
}

// FeatureError reports a feature refused by the profile.
type FeatureError struct {
	Feature Feature
	Profile string
	Reason  string
}

func (e *FeatureError) Error() string {
	msg := fmt.Sprintf("Profil %s verbietet %s", e.Profile, e.Feature)
	if e.Reason != "" {
		msg += ": " + e.Reason
	}
	return msg
}

// alwaysForbidden are features that pdx never writes, regardless of profile.
var alwaysForbidden = map[Feature]string{
	JavaScript: "pdx schreibt grundsätzlich kein JavaScript",
	XFA:        "pdx schreibt grundsätzlich kein XFA",
	Encryption: "pdx verschlüsselt nicht",
	// Every field brings its own appearance. PDF/A forbids
	// NeedAppearances (ISO 19005-2 6.4.1, ISO 19005-4 6.4.1).
	NeedAppearances: "pdx schreibt jedes Erscheinungsbild selbst und setzt NeedAppearances nie",
}

// ActionPolicy checks actions (ISO 32000-1 12.6). Profiles that do not
// restrict actions do not implement it. Every method returns nil
// or an error with the violated clause.
type ActionPolicy interface {
	// AllowAction checks an action of type s. For named actions, named is
	// the value of N, otherwise empty.
	AllowAction(s, named Name) error
	// AllowAdditionalActions checks an AA dictionary. owner is Catalog,
	// Page, Widget, Field or Annot, keys are its triggers.
	AllowAdditionalActions(owner Name, keys []Name) error
	// AllowWidgetAction checks the entry A on a widget annotation.
	AllowWidgetAction() error
}
