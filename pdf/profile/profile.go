// Package profile enthält die Konformitätsprofile von pdx. Ein Profil legt
// fest, welche PDF-Funktionen erlaubt sind, und ergänzt vor dem Schreiben die
// Pflichtobjekte seines Standards. So entsteht Konformität durch Konstruktion:
// eine verbotene Funktion führt zu einem Fehler beim Schreiben statt zu einer
// nicht konformen Datei.
package profile

import (
	"fmt"
	"sort"
	"time"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/pdf"
)

// Context übergibt einem Profil die Teile des Dokuments, die es ergänzen darf.
type Context struct {
	Doc      *pdf.Document
	Catalog  *pdf.Dict
	Info     *pdf.Dict
	Meta     areatree.Metadata
	Date     time.Time
	Producer string
}

// Profile ist ein Konformitätsprofil.
type Profile interface {
	pdf.Policy
	// Prepare ergänzt Pflichtobjekte wie Output Intent und XMP. Es wird nach
	// dem Anlegen aller Seiten und vor dem Schreiben aufgerufen.
	Prepare(ctx *Context) error
}

var registry = map[string]func() Profile{}

func register(name string, f func() Profile) { registry[name] = f }

// Lookup liefert das Profil zu einem Namen wie "pdfa-3b".
func Lookup(name string) (Profile, error) {
	f, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unbekanntes Profil %q, verfügbar: %v", name, Names())
	}
	return f(), nil
}

// Names liefert alle Profilnamen sortiert.
func Names() []string {
	var ns []string
	for n := range registry {
		ns = append(ns, n)
	}
	sort.Strings(ns)
	return ns
}

func deny(p pdf.Policy, f pdf.Feature, reason string) error {
	return &pdf.FeatureError{Feature: f, Profile: p.Name(), Reason: reason}
}
