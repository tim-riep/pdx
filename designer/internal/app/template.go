package app

import (
	"errors"

	"github.com/tim-riep/pdx/designer/internal/model"
)

// TemplateService liefert die Projektion der Vorlage und wendet Befehle an.
type TemplateService struct{ c *Core }

// NewTemplateService liefert die Sicht auf den gemeinsamen Zustand.
func NewTemplateService(c *Core) *TemplateService { return &TemplateService{c} }

// Snapshot liefert die vollständige Projektion der aktuellen Revision.
func (s *TemplateService) Snapshot() (model.Snapshot, error) {
	cat, err := model.LoadCatalog()
	if err != nil {
		return model.Snapshot{}, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	if s.c.doc == nil {
		return model.Snapshot{}, errors.New("keine Vorlage geöffnet")
	}
	s.c.view = model.Project(s.c.doc.Pkg.Template, s.c.doc.Rev(), cat)
	return s.c.view, nil
}

// Result ist die Antwort auf einen Befehl. Select schlägt das Element vor,
// das die Oberfläche danach auswählt, etwa ein eingefügtes.
type Result struct {
	Rev     uint64        `json:"rev"`
	Select  string        `json:"select"`
	History model.History `json:"history"`
}

// Apply wendet einen Befehl an. label erscheint in der Historie, etwa
// „Breite ändern“. Ungültige Befehle ändern nichts und liefern den Fehler.
func (s *TemplateService) Apply(label string, cmd model.Command) (Result, error) {
	return s.change(func(d *model.Document) (string, error) {
		if err := d.Apply(label, cmd); err != nil {
			return "", err
		}
		return inserted(d, cmd), nil
	})
}

// Undo nimmt den letzten Schritt zurück.
func (s *TemplateService) Undo() (Result, error) {
	return s.change(func(d *model.Document) (string, error) {
		_, err := d.Undo()
		return "", err
	})
}

// Redo wiederholt den zuletzt zurückgenommenen Schritt.
func (s *TemplateService) Redo() (Result, error) {
	return s.change(func(d *model.Document) (string, error) {
		_, err := d.Redo()
		return "", err
	})
}

// History liefert die Beschriftungen für Undo und Redo.
func (s *TemplateService) History() model.History {
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	if s.c.doc == nil {
		return model.History{}
	}
	return s.c.doc.History()
}

// change führt eine Änderung aus und meldet den Patch der Projektion.
func (s *TemplateService) change(f func(*model.Document) (string, error)) (Result, error) {
	cat, err := model.LoadCatalog()
	if err != nil {
		return Result{}, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	d := s.c.doc
	if d == nil {
		return Result{}, errors.New("keine Vorlage geöffnet")
	}
	if s.c.view.Nodes == nil {
		s.c.view = model.Project(d.Pkg.Template, d.Rev(), cat)
	}
	sel, err := f(d)
	if err != nil {
		return Result{Rev: d.Rev(), History: d.History()}, err
	}
	cur := model.Project(d.Pkg.Template, d.Rev(), cat)
	s.c.emit.Emit(EventTemplateChanged, model.Diff(s.c.view, cur))
	s.c.view = cur
	s.c.afterChangeLocked()
	return Result{Rev: d.Rev(), Select: sel, History: d.History()}, nil
}

// inserted liefert die Kennung des ersten eingefügten Elements.
func inserted(d *model.Document, cmd model.Command) string {
	switch cmd.Op {
	case "insert":
		if p, err := model.Resolve(d.Pkg.Template, cmd.Parent); err == nil {
			i := cmd.Index
			if i < 0 || i >= len(p.Children) {
				i = len(p.Children) - 1
			}
			if i >= 0 {
				return model.Ref(p.Children[i])
			}
		}
	case "setAttr":
		if cmd.Name == "id" && cmd.Value != "" {
			return cmd.Value
		}
		return cmd.Ref
	case "move":
		return cmd.Ref
	}
	return ""
}

// Catalog liefert die Elementtypen mit ihren Eigenschaften für den Inspektor.
func (s *TemplateService) Catalog() (model.Catalog, error) { return model.LoadCatalog() }

// Drop fügt für einen gezogenen Datenknoten das passende gebundene Element
// ein: Datum als Datumsfeld mit Format, Dezimalzahl als Betragsfeld,
// wiederholter Knoten als Tabelle mit Spalten aus den Kindknoten, optionaler
// Knoten mit Präsenzbedingung.
func (s *TemplateService) Drop(drop Drop) (Result, error) {
	return s.change(func(d *model.Document) (string, error) {
		sc, err := s.c.schemaLocked()
		if err != nil {
			return "", err
		}
		cmd, id, err := buildDrop(d.Pkg.Template, sc, drop)
		if err != nil {
			return "", err
		}
		label := "Feld einfügen"
		if n := sc.Find(drop.Path); n != nil {
			label = n.Name + " einfügen"
		}
		if err := d.Apply(label, cmd); err != nil {
			return "", err
		}
		return id, nil
	})
}
