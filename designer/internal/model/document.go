// Package model holds the template model of the designer: the open package,
// its revision and later the commands with Undo and Redo. It is the single
// source of truth, the UI only sees projections of it.
package model

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/tim-riep/pdx/template"
	"github.com/tim-riep/pdx/template/pkg"
)

// Format is the storage form of a package.
type Format string

const (
	// Zip is a package as a ZIP file.
	Zip Format = "zip"
	// Dir is a package as a directory with template.xml.
	Dir Format = "dir"
)

// Document is an open template package.
type Document struct {
	Pkg    *pkg.Package
	Path   string // empty for a new template that has not been saved yet
	Format Format

	// rev increases with every change, also through Undo, and serves caches.
	rev uint64
	// state identifies the content. Undo returns to an earlier ID,
	// savedState is the one of the saved state.
	state, savedState, nextState uint64
	undo, redo                   []entry
}

// newTemplate is the template for "New": an A4 page and an empty body.
const newTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<template xmlns="https://github.com/tim-riep/pdx/ns/template/1" id="vorlage" lang="de" version="1">
  <pageSet id="seiten">
    <pageMaster id="seite" margin="20mm" size="A4"/>
  </pageSet>
  <body id="inhalt"/>
</template>
`

// New creates a new, empty template.
func New() *Document {
	t, err := template.Parse([]byte(newTemplate))
	if err != nil {
		panic("Vorlage für Neu ungültig: " + err.Error())
	}
	p, err := pkg.New(t, nil)
	if err != nil {
		panic("Paket für Neu ungültig: " + err.Error())
	}
	return &Document{Pkg: p, Format: Zip}
}

// Open loads a package from a ZIP file or a directory.
func Open(path string) (*Document, error) {
	p, err := pkg.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("Vorlage %s nicht gefunden", path)
	}
	if err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	return &Document{Pkg: p, Path: abs, Format: formatOf(abs)}, nil
}

// formatOf derives the format from the path: a directory or a
// file without .zip is a directory, otherwise ZIP.
func formatOf(path string) Format {
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		return Dir
	}
	if strings.EqualFold(filepath.Ext(path), ".zip") {
		return Zip
	}
	return Dir
}

// Name is the display name, for example the file name without .zip.
func (d *Document) Name() string {
	if d.Path == "" {
		if t, ok := d.Pkg.Template.Root.Attr("title"); ok && t != "" {
			return t
		}
		return ""
	}
	return strings.TrimSuffix(filepath.Base(d.Path), filepath.Ext(d.Path))
}

// Rev is the revision, it increases with every change.
func (d *Document) Rev() uint64 { return d.rev }

// Touch records a change outside the command model.
func (d *Document) Touch() { d.touchState() }

func (d *Document) touchState() {
	d.rev++
	d.nextState++
	d.state = d.nextState
}

// Dirty reports unsaved changes.
func (d *Document) Dirty() bool { return d.state != d.savedState || d.Path == "" && d.state != 0 }

// Save writes the package to its path in the previous format.
func (d *Document) Save() error {
	if d.Path == "" {
		return fmt.Errorf("die Vorlage hat noch keinen Speicherort")
	}
	return d.SaveAs(d.Path)
}

// SaveAs writes the package to a new path. If it ends in .zip, the result
// is a ZIP file, otherwise a directory.
func (d *Document) SaveAs(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	f := Dir
	if strings.EqualFold(filepath.Ext(abs), ".zip") {
		f = Zip
	}
	if f == Zip {
		b, err := d.Pkg.Bytes()
		if err != nil {
			return err
		}
		if err := writeAtomic(abs, b); err != nil {
			return err
		}
	} else if err := d.Pkg.WriteDir(abs); err != nil {
		return err
	}
	d.Path, d.Format, d.savedState = abs, f, d.state
	return nil
}

// Bytes returns the package as a ZIP, for example for the automatic backup.
func (d *Document) Bytes() ([]byte, error) { return d.Pkg.Bytes() }

// writeAtomic writes a file next to the target first and then renames it.
func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".pdx-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}
