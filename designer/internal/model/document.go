// Package model hält das Vorlagenmodell des Designers: das geöffnete Paket,
// seine Revision und später die Befehle mit Undo und Redo. Es ist die einzige
// Wahrheit, die Oberfläche sieht nur Projektionen davon.
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

// Format ist die Ablage eines Pakets.
type Format string

const (
	// Zip ist ein Paket als ZIP-Datei.
	Zip Format = "zip"
	// Dir ist ein Paket als Verzeichnis mit template.xml.
	Dir Format = "dir"
)

// Document ist ein geöffnetes Vorlagenpaket.
type Document struct {
	Pkg    *pkg.Package
	Path   string // leer bei einer neuen, noch nicht gespeicherten Vorlage
	Format Format

	// rev steigt mit jeder Änderung, auch durch Undo, und dient Caches.
	rev uint64
	// state kennzeichnet den Inhalt. Undo kehrt zu einer früheren Kennung
	// zurück, savedState ist die des gespeicherten Stands.
	state, savedState, nextState uint64
	undo, redo                   []entry
}

// newTemplate ist die Vorlage für „Neu“: eine A4-Seite und ein leerer Rumpf.
const newTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<template xmlns="https://github.com/tim-riep/pdx/ns/template/1" id="vorlage" lang="de" version="1">
  <pageSet id="seiten">
    <pageMaster id="seite" margin="20mm" size="A4"/>
  </pageSet>
  <body id="inhalt"/>
</template>
`

// New legt eine neue, leere Vorlage an.
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

// Open lädt ein Paket aus einer ZIP-Datei oder einem Verzeichnis.
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

// formatOf leitet das Format aus dem Pfad ab: ein Verzeichnis oder eine
// Datei ohne .zip ist ein Verzeichnis, sonst ZIP.
func formatOf(path string) Format {
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		return Dir
	}
	if strings.EqualFold(filepath.Ext(path), ".zip") {
		return Zip
	}
	return Dir
}

// Name ist der Anzeigename, etwa der Dateiname ohne .zip.
func (d *Document) Name() string {
	if d.Path == "" {
		if t, ok := d.Pkg.Template.Root.Attr("title"); ok && t != "" {
			return t
		}
		return ""
	}
	return strings.TrimSuffix(filepath.Base(d.Path), filepath.Ext(d.Path))
}

// Rev ist die Revision, sie steigt mit jeder Änderung.
func (d *Document) Rev() uint64 { return d.rev }

// Touch vermerkt eine Änderung außerhalb des Befehlsmodells.
func (d *Document) Touch() { d.touchState() }

func (d *Document) touchState() {
	d.rev++
	d.nextState++
	d.state = d.nextState
}

// Dirty meldet ungespeicherte Änderungen.
func (d *Document) Dirty() bool { return d.state != d.savedState || d.Path == "" && d.state != 0 }

// Save schreibt das Paket an seinen Pfad im bisherigen Format.
func (d *Document) Save() error {
	if d.Path == "" {
		return fmt.Errorf("die Vorlage hat noch keinen Speicherort")
	}
	return d.SaveAs(d.Path)
}

// SaveAs schreibt das Paket an einen neuen Pfad. Endet er auf .zip, entsteht
// eine ZIP-Datei, sonst ein Verzeichnis.
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

// Bytes liefert das Paket als ZIP, etwa für die automatische Sicherung.
func (d *Document) Bytes() ([]byte, error) { return d.Pkg.Bytes() }

// writeAtomic schreibt eine Datei erst neben das Ziel und benennt sie dann um.
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
