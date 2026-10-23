// Package pdx is the facade of the PDF engine: read XSL-FO, lay it out and
// write it as PDF under a conformance profile.
//
// Flow: fo/tree reads the FO document, fo/layout produces the area tree,
// pdf/render writes it under the profile from pdf/profile. The CLI in
// cmd/pdx, later the designer and the server mode use only this
// facade.
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

// Version is the program version. It appears in the Producer of the PDF files.
const Version = "0.3.1"

// DefaultProfile is the profile used when none is specified explicitly.
const DefaultProfile = "pdfa-3b"

// Options controls Render.
type Options struct {
	// Profile is the profile name, for example "pdfa-3b" or "pdf-1.7".
	Profile string
	// CreationDate is the creation date. The same input and the same
	// date produce byte-identical files.
	CreationDate time.Time
	// BaseDir is the directory for relative paths of images and attachments.
	BaseDir string
	// Resolver replaces file access relative to BaseDir, for example for
	// template packages. Optional.
	Resolver layout.Resolver
	// Attachments are embedded in addition to the attachments from the FO.
	Attachments []areatree.Attachment
	// Fonts is the font registry. nil means the built-in fonts.
	// A shared registry keeps loaded fonts across many runs.
	Fonts *font.Registry
	// Context cancels layout and rendering. nil means no cancellation.
	Context context.Context
	// Trace receives, if set, the mapping of the PDF operators to the
	// template elements (for findings from veraPDF).
	Trace *render.Trace
	// Tagged writes Tagged PDF with a structure tree, even if the profile
	// does not require it.
	Tagged bool
	// XMP are additional XMP schemas, for example fx of the e-invoice.
	XMP []areatree.XMPSchema
	// Flatten draws form fields as fixed content (pdx flatten).
	Flatten bool
}

// Result contains the PDF and the area tree.
type Result struct {
	PDF  []byte
	Tree *areatree.Document
}

// Render reads XSL-FO and produces the PDF.
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

// Layout reads XSL-FO and returns only the area tree, without PDF. Profile
// and creation date are not needed. For preview and designer.
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

// RenderFile reads an FO file. If BaseDir is empty, its directory is used.
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
