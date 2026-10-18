// Package profile enthält die Konformitätsprofile von pdx. Ein Profil legt
// fest, welche PDF-Funktionen erlaubt sind, und ergänzt vor dem Schreiben die
// Pflichtobjekte seines Standards. So entsteht Konformität durch Konstruktion:
// eine verbotene Funktion führt zu einem Fehler beim Schreiben statt zu einer
// nicht konformen Datei.
package profile

import (
	"errors"
	"fmt"
	"sort"
	"strings"
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

// Claim beschreibt, was ein Profil von der Datei verlangt und was die Datei
// über sich behauptet. Eine Kombination von Profilen vereint ihre Ansprüche.
type Claim struct {
	// Version ist die PDF-Version des Headers, "1.7" oder "2.0".
	Version string
	// PDFA ist der Teil von ISO 19005 (2, 3 oder 4), 0 ohne PDF/A.
	PDFA int
	// Level ist die Konformitätsstufe von PDF/A: "B", "U", "A", "F", "E"
	// oder leer (PDF/A-4 ohne Stufe).
	Level string
	// UA ist der Teil von ISO 14289 (1 oder 2), 0 ohne PDF/UA.
	UA int
}

// claimer liefert den Anspruch eines einzelnen Profils.
type claimer interface{ claim() Claim }

var registry = map[string]func() Profile{}

func register(name string, f func() Profile) { registry[name] = f }

// Lookup liefert das Profil zu einem Namen wie "pdfa-3b" oder zu einer
// Kombination wie "pdfa-3a,pdfua-1". Unzulässige Kombinationen lehnt es mit
// allen Gründen auf einmal ab.
func Lookup(spec string) (Profile, error) {
	names := Split(spec)
	if len(names) == 0 {
		return nil, fmt.Errorf("kein Profil angegeben, verfügbar: %v", Names())
	}
	var parts []Profile
	var errs []error
	seen := map[string]bool{}
	for _, n := range names {
		f, ok := registry[n]
		switch {
		case !ok:
			errs = append(errs, fmt.Errorf("unbekanntes Profil %q, verfügbar: %v", n, Names()))
		case seen[n]:
			errs = append(errs, fmt.Errorf("Profil %q ist doppelt angegeben", n))
		default:
			seen[n] = true
			parts = append(parts, f())
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	if len(parts) == 1 {
		return parts[0], nil
	}
	if _, err := combine(parts); err != nil {
		return nil, err
	}
	return &Set{parts: parts}, nil
}

// Split zerlegt eine Profilangabe an Kommas und entfernt Leerraum.
func Split(spec string) []string {
	var out []string
	for _, n := range strings.Split(spec, ",") {
		if n = strings.TrimSpace(n); n != "" {
			out = append(out, n)
		}
	}
	return out
}

// Parts liefert die einzelnen Profile einer Kombination oder das Profil
// selbst.
func Parts(p Profile) []Profile {
	if s, ok := p.(*Set); ok {
		return s.parts
	}
	return []Profile{p}
}

// ClaimOf liefert den vereinten Anspruch eines Profils oder einer
// Kombination.
func ClaimOf(p Profile) Claim {
	c, _ := combine(Parts(p))
	return c
}

// combine vereint die Ansprüche und prüft, ob sie zusammenpassen. Ein
// PDF/A-Teil und ein PDF/UA-Teil sind kombinierbar, wenn sie dieselbe
// PDF-Version verlangen: PDF/A-2 und PDF/A-3 beruhen auf PDF 1.7 (ISO 32000-1),
// PDF/A-4 und PDF/UA-2 auf PDF 2.0 (ISO 32000-2), PDF/UA-1 auf PDF 1.7.
func combine(parts []Profile) (Claim, error) {
	var c Claim
	var errs []error
	var pdfa, ua, plain []string
	for _, p := range parts {
		cl, ok := p.(claimer)
		if !ok {
			plain = append(plain, p.Name())
			continue
		}
		pc := cl.claim()
		if pc.PDFA > 0 {
			pdfa = append(pdfa, p.Name())
			c.PDFA, c.Level = pc.PDFA, pc.Level
		}
		if pc.UA > 0 {
			ua = append(ua, p.Name())
			c.UA = pc.UA
		}
		if pc.Version != "" {
			if c.Version != "" && c.Version != pc.Version {
				errs = append(errs, fmt.Errorf("%s verlangt PDF %s, %s verlangt PDF %s, beide lassen sich nicht kombinieren",
					parts[0].Name(), c.Version, p.Name(), pc.Version))
			} else {
				c.Version = pc.Version
			}
		}
	}
	if len(parts) > 1 && len(plain) > 0 {
		errs = append(errs, fmt.Errorf("%s steht für PDF ohne Konformitätsanspruch und lässt sich nicht kombinieren", strings.Join(plain, ", ")))
	}
	if len(pdfa) > 1 {
		errs = append(errs, fmt.Errorf("nur ein PDF/A-Profil je Datei, angegeben: %s", strings.Join(pdfa, ", ")))
	}
	if len(ua) > 1 {
		errs = append(errs, fmt.Errorf("nur ein PDF/UA-Profil je Datei, angegeben: %s", strings.Join(ua, ", ")))
	}
	if c.Version == "" {
		c.Version = "1.7"
	}
	return c, errors.Join(errs...)
}

// Set ist eine zulässige Kombination von Profilen, etwa PDF/A-3a mit
// PDF/UA-1. Eine Funktion ist erlaubt, wenn jedes Profil sie erlaubt.
type Set struct{ parts []Profile }

// Name liefert die Profilnamen mit Komma, etwa "pdfa-3a,pdfua-1".
func (s *Set) Name() string {
	names := make([]string, len(s.parts))
	for i, p := range s.parts {
		names[i] = p.Name()
	}
	return strings.Join(names, ",")
}

// Allow prüft eine Funktion gegen alle Profile und nennt das erste, das sie
// verweigert.
func (s *Set) Allow(f pdf.Feature) error {
	for _, p := range s.parts {
		if err := p.Allow(f); err != nil {
			return err
		}
	}
	return nil
}

// Prepare ergänzt die Pflichtobjekte einmal für den vereinten Anspruch, damit
// Output Intent und XMP nicht doppelt entstehen.
func (s *Set) Prepare(ctx *Context) error { return prepareClaim(ctx, ClaimOf(s)) }

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
