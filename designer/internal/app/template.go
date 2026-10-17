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
	return model.Project(s.c.doc.Pkg.Template, s.c.doc.Rev(), cat), nil
}

// Catalog liefert die Elementtypen mit ihren Eigenschaften für den Inspektor.
func (s *TemplateService) Catalog() (model.Catalog, error) { return model.LoadCatalog() }
