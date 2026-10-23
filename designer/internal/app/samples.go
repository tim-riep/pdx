package app

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/tim-riep/pdx/designer/internal/model"
	"github.com/tim-riep/pdx/designer/internal/schema"
)

// SampleDataService manages the sample data under samples/: selection for
// preview, PDF and conformance, import and generated edge cases.
type SampleDataService struct{ c *Core }

// NewSampleDataService returns the view on the shared state.
func NewSampleDataService(c *Core) *SampleDataService { return &SampleDataService{c} }

// Sample is a data set in the package.
type Sample struct {
	Path string `json:"path"` // samples/rechnung.xml
	Name string `json:"name"`
	Size int    `json:"size"`
	// Selected marks the data set of the preview.
	Selected bool `json:"selected"`
}

// Samples are the data sets and the selection.
type Samples struct {
	Items []Sample `json:"items"`
	// Placeholder reports that the preview shows placeholders because there
	// is no sample data.
	Placeholder bool `json:"placeholder"`
}

// Edge cases that can be generated.
var edgeCases = map[string]struct {
	file string
	gen  schema.Gen
}{
	"minimal": {"samples/rand-minimal.xml", schema.Gen{Repeat: 0, Optional: false}},
	"maximal": {"samples/rand-maximal.xml", schema.Gen{Repeat: 60, Optional: true}},
	"lang":    {"samples/rand-lange-texte.xml", schema.Gen{Repeat: 3, Optional: true, LongText: true}},
}

// List returns the data sets.
func (s *SampleDataService) List() (Samples, error) {
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	if s.c.doc == nil {
		return Samples{}, errors.New("keine Vorlage geöffnet")
	}
	_, current := s.c.previewDataLocked()
	out := Samples{Items: []Sample{}, Placeholder: current == ""}
	for _, p := range s.c.doc.Pkg.Samples() {
		out.Items = append(out.Items, Sample{Path: p, Name: strings.TrimPrefix(p, "samples/"), Size: len(s.c.doc.Pkg.Files[p]), Selected: p == current})
	}
	return out, nil
}

// Select chooses the data set for preview, PDF and conformance.
func (s *SampleDataService) Select(p string) error {
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	if s.c.doc == nil {
		return errors.New("keine Vorlage geöffnet")
	}
	if _, ok := s.c.doc.Pkg.Files[p]; !ok {
		return fmt.Errorf("Beispieldaten %s gibt es nicht", p)
	}
	s.c.r.sample = p
	s.c.scheduleLocked(0)
	return nil
}

// Import adds an XML file as sample data and selects it.
func (s *SampleDataService) Import(file string) (Result, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			return Result{}, fmt.Errorf("Datei %s nicht gefunden", file)
		}
		return Result{}, err
	}
	name := sampleName(filepath.Base(file))
	return s.add(name, b, "Beispieldaten importieren")
}

// ImportDialog asks for the file.
func (s *SampleDataService) ImportDialog() (Result, error) {
	if s.c.dialogs == nil {
		return Result{}, ErrNoDialogs
	}
	p, err := s.c.dialogs.OpenFile("Beispieldaten importieren", []Filter{{Name: "XML (*.xml)", Pattern: "*.xml"}}, false)
	if err != nil || p == "" {
		return Result{}, err
	}
	return s.Import(p)
}

// Generate creates an edge case from the data XSD: minimal (only
// required nodes, repetitions as rare as allowed), maximal (all
// optional nodes, 60 entries per repetition) or long (long texts).
func (s *SampleDataService) Generate(kind string) (Result, error) {
	ec, ok := edgeCases[kind]
	if !ok {
		return Result{}, fmt.Errorf("Randfall %q unbekannt", kind)
	}
	s.c.mu.Lock()
	if s.c.doc == nil {
		s.c.mu.Unlock()
		return Result{}, errors.New("keine Vorlage geöffnet")
	}
	sc, err := s.c.schemaLocked()
	s.c.mu.Unlock()
	if err != nil {
		return Result{}, err
	}
	if sc == nil {
		return Result{}, errors.New("die Vorlage hat noch kein Daten-XSD")
	}
	return s.add(ec.file, sc.Generate(ec.gen), "Randfall erzeugen")
}

// Remove removes sample data from the package.
func (s *SampleDataService) Remove(p string) (Result, error) {
	if !strings.HasPrefix(p, "samples/") {
		return Result{}, fmt.Errorf("%s liegt nicht unter samples/", p)
	}
	res, err := NewTemplateService(s.c).change(func(d *model.Document) (string, error) {
		if _, ok := d.Pkg.Files[p]; !ok {
			return "", fmt.Errorf("Beispieldaten %s gibt es nicht", p)
		}
		return "", d.Apply("Beispieldaten entfernen", model.Command{Op: "setFile", Name: p})
	})
	if err == nil {
		s.c.mu.Lock()
		if s.c.r.sample == p {
			s.c.r.sample = ""
			s.c.scheduleLocked(0)
		}
		s.c.mu.Unlock()
	}
	return res, err
}

// add creates a file as a command (Undo removes it again) and selects it.
func (s *SampleDataService) add(name string, b []byte, label string) (Result, error) {
	res, err := NewTemplateService(s.c).change(func(d *model.Document) (string, error) {
		return "", d.Apply(label, model.Command{Op: "setFile", Name: name, Data: b})
	})
	if err != nil {
		return res, err
	}
	s.c.mu.Lock()
	s.c.r.sample = name
	s.c.scheduleLocked(0)
	s.c.mu.Unlock()
	return res, nil
}

// sampleName forms a path under samples/ from a file name.
func sampleName(base string) string {
	base = strings.TrimSuffix(base, path.Ext(base))
	var b strings.Builder
	for _, r := range base {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	name := strings.Trim(b.String(), "-.")
	if name == "" {
		name = "daten"
	}
	return "samples/" + name + ".xml"
}
