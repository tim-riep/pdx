package app

import (
	"errors"

	"github.com/tim-riep/pdx/designer/internal/model"
)

// TemplateService returns the projection of the template and applies commands.
type TemplateService struct{ c *Core }

// NewTemplateService returns the view on the shared state.
func NewTemplateService(c *Core) *TemplateService { return &TemplateService{c} }

// Snapshot returns the complete projection of the current revision.
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

// Result is the response to a command. Select suggests the element
// that the UI selects afterwards, for example an inserted one.
type Result struct {
	Rev     uint64        `json:"rev"`
	Select  string        `json:"select"`
	History model.History `json:"history"`
}

// Apply applies a command. label appears in the history, for example
// "Breite ändern". Invalid commands change nothing and return the error.
func (s *TemplateService) Apply(label string, cmd model.Command) (Result, error) {
	return s.change(func(d *model.Document) (string, error) {
		if err := d.Apply(label, cmd); err != nil {
			return "", err
		}
		return inserted(d, cmd), nil
	})
}

// Undo takes back the last step.
func (s *TemplateService) Undo() (Result, error) {
	return s.change(func(d *model.Document) (string, error) {
		_, err := d.Undo()
		return "", err
	})
}

// Redo repeats the most recently undone step.
func (s *TemplateService) Redo() (Result, error) {
	return s.change(func(d *model.Document) (string, error) {
		_, err := d.Redo()
		return "", err
	})
}

// History returns the labels for Undo and Redo.
func (s *TemplateService) History() model.History {
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	if s.c.doc == nil {
		return model.History{}
	}
	return s.c.doc.History()
}

// change performs a change and reports the patch of the projection.
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

// inserted returns the ID of the first inserted element.
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

// Catalog returns the element types with their properties for the inspector.
func (s *TemplateService) Catalog() (model.Catalog, error) { return model.LoadCatalog() }

// Drop inserts the matching bound element for a dragged data node:
// date as a date field with format, decimal number as an amount field,
// repeated node as a table with columns from the child nodes, optional
// node with a presence condition.
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
