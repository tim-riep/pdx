// Package profile contains the conformance profiles of pdx. A profile
// defines which PDF features are allowed and adds the required objects of
// its standard before writing. This gives conformance by construction:
// a forbidden feature leads to an error when writing instead of to a
// non-conforming file.
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

// Context passes to a profile the parts of the document that it may extend.
type Context struct {
	Doc      *pdf.Document
	Catalog  *pdf.Dict
	Info     *pdf.Dict
	Meta     areatree.Metadata
	Date     time.Time
	Producer string
}

// Profile is a conformance profile.
type Profile interface {
	pdf.Policy
	// Prepare adds required objects such as output intent and XMP. It is called
	// after all pages have been created and before writing.
	Prepare(ctx *Context) error
}

// Claim describes what a profile requires of the file and what the file
// claims about itself. A combination of profiles merges their claims.
type Claim struct {
	// Version is the PDF version of the header, "1.7" or "2.0".
	Version string
	// PDFA is the part of ISO 19005 (2, 3 or 4), 0 without PDF/A.
	PDFA int
	// Level is the conformance level of PDF/A: "B", "U", "A", "F", "E"
	// or empty (PDF/A-4 without a level).
	Level string
	// UA is the part of ISO 14289 (1 or 2), 0 without PDF/UA.
	UA int
}

// claimer returns the claim of a single profile.
type claimer interface{ claim() Claim }

var registry = map[string]func() Profile{}

func register(name string, f func() Profile) { registry[name] = f }

// Lookup returns the profile for a name like "pdfa-3b" or for a
// combination like "pdfa-3a,pdfua-1". It rejects invalid combinations with
// all reasons at once.
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

// Split splits a profile specification at commas and removes whitespace.
func Split(spec string) []string {
	var out []string
	for _, n := range strings.Split(spec, ",") {
		if n = strings.TrimSpace(n); n != "" {
			out = append(out, n)
		}
	}
	return out
}

// Parts returns the individual profiles of a combination or the profile
// itself.
func Parts(p Profile) []Profile {
	if s, ok := p.(*Set); ok {
		return s.parts
	}
	return []Profile{p}
}

// ClaimOf returns the merged claim of a profile or of a
// combination.
func ClaimOf(p Profile) Claim {
	c, _ := combine(Parts(p))
	return c
}

// combine merges the claims and checks whether they fit together. A
// PDF/A part and a PDF/UA part can be combined if they require the same
// PDF version: PDF/A-2 and PDF/A-3 are based on PDF 1.7 (ISO 32000-1),
// PDF/A-4 and PDF/UA-2 on PDF 2.0 (ISO 32000-2), PDF/UA-1 on PDF 1.7.
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
		if pc.PDFA == 0 && pc.UA == 0 {
			plain = append(plain, p.Name())
		}
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

// Set is a valid combination of profiles, for example PDF/A-3a with
// PDF/UA-1. A feature is allowed if every profile allows it.
type Set struct{ parts []Profile }

// Name returns the profile names separated by commas, for example "pdfa-3a,pdfua-1".
func (s *Set) Name() string {
	names := make([]string, len(s.parts))
	for i, p := range s.parts {
		names[i] = p.Name()
	}
	return strings.Join(names, ",")
}

// Allow checks a feature against all profiles and names the first one that
// refuses it.
func (s *Set) Allow(f pdf.Feature) error {
	for _, p := range s.parts {
		if err := p.Allow(f); err != nil {
			return err
		}
	}
	return nil
}

// CheckAttachment asks every profile that checks the content of attachments.
func (s *Set) CheckAttachment(name string, data []byte) error {
	for _, p := range s.parts {
		if c, ok := p.(AttachmentChecker); ok {
			if err := c.CheckAttachment(name, data); err != nil {
				return err
			}
		}
	}
	return nil
}

// AttachmentChecker checks the content of an attachment, for example PDF/A-2,
// which only allows PDF/A documents as attachments.
type AttachmentChecker interface {
	CheckAttachment(name string, data []byte) error
}

// Prepare adds the required objects once for the merged claim, so that
// output intent and XMP are not created twice.
func (s *Set) Prepare(ctx *Context) error { return prepareClaim(ctx, ClaimOf(s)) }

// Names returns all profile names sorted.
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
