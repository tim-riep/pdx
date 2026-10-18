package pdf

import "fmt"

// Feature ist eine PDF-Funktion, deren Nutzung ein Konformitätsprofil
// erlauben muss. Der Writer fragt vor jeder Nutzung über Document.Use nach.
type Feature int

const (
	// Transparency steht für Soft Masks, Konstantalpha und Blend-Modi.
	Transparency Feature = iota + 1
	// Encryption steht für ein Encrypt-Dictionary.
	Encryption
	// LZW steht für den Filter LZWDecode.
	LZW
	// ExternalContent steht für Streams oder Dateien außerhalb des PDF.
	ExternalContent
	// JavaScript steht für JavaScript-Aktionen und Namensbäume.
	JavaScript
	// XFA steht für XFA-Formulare.
	XFA
	// Attachment steht für eingebettete Dateien.
	Attachment
	// UnembeddedFont steht für eine nicht eingebettete Schrift.
	UnembeddedFont
	// UnmappedGlyph steht für eine Glyphe ohne Unicode-Zuordnung in der
	// ToUnicode-CMap ihrer Schrift.
	UnmappedGlyph
	// NeedAppearances steht für den Eintrag NeedAppearances true im
	// AcroForm-Dictionary, der den Viewer Erscheinungsbilder erzeugen lässt.
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

// Policy entscheidet, ob eine Funktion genutzt werden darf. Konformitätsprofile
// in pdf/profile implementieren diese Schnittstelle.
type Policy interface {
	// Name ist der Profilname für Fehlermeldungen, etwa "pdfa-3b".
	Name() string
	// Allow liefert nil, wenn die Funktion erlaubt ist, sonst einen Fehler,
	// der die verletzte Regel nennt.
	Allow(f Feature) error
}

// FeatureError meldet eine vom Profil verweigerte Funktion.
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

// alwaysForbidden sind Funktionen, die pdx unabhängig vom Profil nie schreibt.
var alwaysForbidden = map[Feature]string{
	JavaScript: "pdx schreibt grundsätzlich kein JavaScript",
	XFA:        "pdx schreibt grundsätzlich kein XFA",
	Encryption: "pdx verschlüsselt nicht",
	// Jedes Feld bringt sein eigenes Erscheinungsbild mit. PDF/A verbietet
	// NeedAppearances (ISO 19005-2 6.4.1, ISO 19005-4 6.4.1).
	NeedAppearances: "pdx schreibt jedes Erscheinungsbild selbst und setzt NeedAppearances nie",
}

// ActionPolicy prüft Aktionen (ISO 32000-1 12.6). Profile, die Aktionen
// nicht einschränken, implementieren sie nicht. Jede Methode liefert nil
// oder einen Fehler mit der verletzten Klausel.
type ActionPolicy interface {
	// AllowAction prüft eine Aktion vom Typ s. named ist bei Named-Aktionen
	// der Wert von N, sonst leer.
	AllowAction(s, named Name) error
	// AllowAdditionalActions prüft ein AA-Dictionary. owner ist Catalog,
	// Page, Widget, Field oder Annot, keys sind seine Auslöser.
	AllowAdditionalActions(owner Name, keys []Name) error
	// AllowWidgetAction prüft den Eintrag A an einer Widget-Annotation.
	AllowWidgetAction() error
}
