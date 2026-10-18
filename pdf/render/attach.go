package render

import (
	"crypto/md5"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/pdf"
	"github.com/tim-riep/pdx/pdf/profile"
)

var relationships = map[areatree.Relationship]bool{
	areatree.Source: true, areatree.Data: true, areatree.Alternative: true, areatree.Supplement: true,
	areatree.EncryptedPayload: true, areatree.FormData: true, areatree.Schema: true, areatree.Unspecified: true,
}

// attachments bettet die Dateianhänge ein. Jede Datei bekommt eine Filespec
// mit F, UF, Desc und AFRelationship, einen EmbeddedFile-Stream mit
// MIME-Subtyp und Params, einen Eintrag in /AF am Katalog und im
// EmbeddedFiles-Namensbaum (ISO 19005-3 6.8, ISO 32000-2 14.13).
func (r *renderer) attachments() error {
	atts := r.doc.Attachments
	if len(atts) == 0 {
		if r.claim.PDFA == 4 && r.claim.Level == "F" {
			return fmt.Errorf("Profil %s: ISO 19005-4 6.9 verlangt für PDF/A-4f mindestens einen Dateianhang", r.opts.Profile.Name())
		}
		return nil
	}
	if err := r.d.Use(pdf.Attachment); err != nil {
		return err
	}
	seen := map[string]bool{}
	type named struct {
		name string
		ref  pdf.Ref
	}
	var af pdf.Array
	var names []named
	date := pdf.Date(r.opts.CreationDate)
	// Alle Fehler der Anhänge auf einmal melden.
	var errs []error
	checker, _ := r.opts.Profile.(profile.AttachmentChecker)
	for _, a := range atts {
		switch {
		case a.Name == "":
			errs = append(errs, fmt.Errorf("Dateianhang ohne Namen"))
			continue
		case seen[a.Name]:
			errs = append(errs, fmt.Errorf("Dateianhang %q ist doppelt", a.Name))
			continue
		}
		seen[a.Name] = true
		if !validMIME(a.MIMEType) {
			errs = append(errs, fmt.Errorf("Dateianhang %q: MIME-Typ %q ist ungültig", a.Name, a.MIMEType))
		}
		rel := a.Relationship
		if rel == "" {
			rel = areatree.Unspecified
		}
		if !relationships[rel] {
			errs = append(errs, fmt.Errorf("Dateianhang %q: AFRelationship %q ist unbekannt", a.Name, rel))
		}
		if r.claim.UA == 2 && strings.TrimSpace(a.Description) == "" {
			errs = append(errs, fmt.Errorf("Dateianhang %q: ISO 14289-2 8.14.1 verlangt eine Beschreibung", a.Name))
		}
		if checker != nil {
			if err := checker.CheckAttachment(a.Name, a.Data); err != nil {
				errs = append(errs, err)
			}
		}
		if len(errs) > 0 {
			continue
		}
		sum := md5.Sum(a.Data)
		ef := r.d.Add(pdf.NewFlateStream(pdf.D(
			"Type", pdf.Name("EmbeddedFile"),
			"Subtype", pdf.Name(a.MIMEType),
			"Params", pdf.D("ModDate", date, "CreationDate", date, "Size", pdf.Int(len(a.Data)), "CheckSum", pdf.String(sum[:])),
		), a.Data))
		fs := pdf.D(
			"Type", pdf.Name("Filespec"),
			"F", fileSpecString(a.Name),
			"UF", pdf.TextString(a.Name),
			"EF", pdf.D("F", ef, "UF", ef),
			"AFRelationship", pdf.Name(rel),
		)
		if a.Description != "" {
			fs.Set("Desc", pdf.TextString(a.Description))
		}
		ref := r.d.Add(fs)
		af = append(af, ref)
		names = append(names, named{a.Name, ref})
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	// Der Namensbaum verlangt nach Bytes sortierte Schlüssel.
	sort.Slice(names, func(i, j int) bool {
		return string(keyBytes(names[i].name)) < string(keyBytes(names[j].name))
	})
	var arr pdf.Array
	for _, n := range names {
		arr = append(arr, pdf.TextString(n.name), n.ref)
	}
	r.catalog.Set("AF", af)
	r.catalog.Set("Names", pdf.D("EmbeddedFiles", pdf.D("Names", arr)))
	return nil
}

func keyBytes(s string) []byte { return []byte(pdf.TextString(s).(pdf.String)) }

// fileSpecString liefert für /F eine ASCII-Fassung des Namens. Nicht
// darstellbare Zeichen werden durch Unterstriche ersetzt, der volle Name
// steht in /UF.
func fileSpecString(name string) pdf.String {
	var b strings.Builder
	for _, r := range name {
		if r < 0x20 || r > 0x7e {
			b.WriteByte('_')
			continue
		}
		b.WriteRune(r)
	}
	return pdf.String(b.String())
}

// validMIME prüft grob die Form typ/subtyp ohne Leerzeichen.
func validMIME(s string) bool {
	i := strings.IndexByte(s, '/')
	if i <= 0 || i == len(s)-1 || strings.Count(s, "/") != 1 {
		return false
	}
	for _, c := range s {
		if c <= ' ' || c > '~' {
			return false
		}
	}
	return true
}
