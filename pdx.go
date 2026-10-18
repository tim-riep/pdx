// Package pdx ist die Fassade der PDF-Engine: XSL-FO lesen, setzen und unter
// einem Konformitätsprofil als PDF schreiben.
//
// Ablauf: fo/tree liest das FO-Dokument, fo/layout erzeugt den Area Tree,
// pdf/render schreibt ihn unter dem Profil aus pdf/profile. Die CLI in
// cmd/pdx, später der Designer und der Serverbetrieb nutzen nur diese
// Fassade.
package pdx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/fo/layout"
	"github.com/tim-riep/pdx/fo/tree"
	"github.com/tim-riep/pdx/font"
	"github.com/tim-riep/pdx/pdf/profile"
	"github.com/tim-riep/pdx/pdf/render"
)

// Version ist die Programmversion. Sie steht im Producer der PDF-Dateien.
const Version = "0.1.0-dev"

// DefaultProfile ist das Profil ohne ausdrückliche Angabe.
const DefaultProfile = "pdfa-3b"

// Options steuert Render.
type Options struct {
	// Profile ist der Profilname, etwa "pdfa-3b" oder "pdf-1.7".
	Profile string
	// CreationDate ist das Erstellungsdatum. Gleiche Eingabe und gleiches
	// Datum ergeben byte-identische Dateien.
	CreationDate time.Time
	// BaseDir ist das Verzeichnis für relative Pfade von Bildern und Anhängen.
	BaseDir string
	// Resolver ersetzt den Dateizugriff relativ zu BaseDir, etwa für
	// Vorlagenpakete. Optional.
	Resolver layout.Resolver
	// Attachments werden zusätzlich zu den Anhängen aus dem FO eingebettet.
	Attachments []areatree.Attachment
	// Fonts ist die Schriftregistry. nil bedeutet die eingebauten Schriften.
	// Eine gemeinsame Registry hält geladene Schriften über viele Läufe.
	Fonts *font.Registry
	// Context bricht Layout und Rendern ab. nil bedeutet ohne Abbruch.
	Context context.Context
	// Trace erhält, wenn gesetzt, die Zuordnung der PDF-Operatoren zu den
	// Vorlagenelementen (für Befunde von veraPDF).
	Trace *render.Trace
	// Tagged schreibt Tagged PDF mit Strukturbaum, auch wenn das Profil es
	// nicht verlangt.
	Tagged bool
	// XMP sind zusätzliche XMP-Schemata, etwa fx der E-Rechnung.
	XMP []areatree.XMPSchema
	// Flatten zeichnet Formularfelder als festen Inhalt (pdx flatten).
	Flatten bool
}

// Result enthält das PDF und den Area Tree.
type Result struct {
	PDF  []byte
	Tree *areatree.Document
}

// Render liest XSL-FO und erzeugt das PDF.
func Render(fo io.Reader, opts Options) (*Result, error) {
	if opts.Profile == "" {
		opts.Profile = DefaultProfile
	}
	if opts.CreationDate.IsZero() {
		return nil, fmt.Errorf("Erstellungsdatum fehlt")
	}
	prof, err := profile.Lookup(opts.Profile)
	if err != nil {
		return nil, err
	}
	doc, err := Layout(fo, opts)
	if err != nil {
		return nil, err
	}
	pdf, err := render.Render(doc, render.Options{
		Profile:      prof,
		CreationDate: opts.CreationDate,
		Producer:     "pdx " + Version,
		Trace:        opts.Trace,
		Tagged:       opts.Tagged,
		Flatten:      opts.Flatten,
	})
	if err != nil {
		return nil, err
	}
	return &Result{PDF: pdf, Tree: doc}, nil
}

// Layout liest XSL-FO und liefert nur den Area Tree, ohne PDF. Profil und
// Erstellungsdatum werden nicht gebraucht. Für Vorschau und Designer.
func Layout(fo io.Reader, opts Options) (*areatree.Document, error) {
	root, err := tree.Parse(fo)
	if err != nil {
		return nil, err
	}
	res := opts.Resolver
	if res == nil {
		res = layout.DirResolver{Dir: opts.BaseDir}
	}
	doc, err := layout.Layout(root, layout.Options{Fonts: opts.Fonts, Resolver: res, Context: opts.Context})
	if err != nil {
		return nil, err
	}
	doc.Attachments = append(doc.Attachments, opts.Attachments...)
	doc.Meta.XMP = append(doc.Meta.XMP, opts.XMP...)
	return doc, nil
}

// RenderFile liest eine FO-Datei. Ist BaseDir leer, gilt ihr Verzeichnis.
func RenderFile(path string, opts Options) (*Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if opts.BaseDir == "" {
		opts.BaseDir = filepath.Dir(path)
	}
	res, err := Render(bytes.NewReader(data), opts)
	var pe *tree.Error
	if errors.As(err, &pe) {
		return nil, fmt.Errorf("%s:%w", path, err)
	} else if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return res, nil
}
