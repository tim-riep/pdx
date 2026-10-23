package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/tim-riep/pdx"
	"github.com/tim-riep/pdx/template/pkg"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// uploadTemplate stores a template package (ZIP in the body) as a new
// version: POST /v1/templates/{id}?activate=true. The package is read and
// compiled first, a faulty template does not get into the store.
func (s *Server) uploadTemplate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !ValidID(id) {
		writeProblem(w, Problem{Status: http.StatusBadRequest, Title: "Vorlagen-ID ungültig",
			Detail: "Erlaubt sind Kleinbuchstaben, Ziffern, - und _, bis 64 Zeichen."})
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, s.cfg.MaxTemplate))
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeProblem(w, Problem{Status: http.StatusRequestEntityTooLarge, Title: "Paket zu groß",
				Detail: fmt.Sprintf("Höchstens %d Bytes sind erlaubt.", s.cfg.MaxTemplate)})
			return
		}
		writeProblem(w, Problem{Status: http.StatusBadRequest, Title: "Rumpf nicht lesbar"})
		return
	}
	if !s.acquire(r.Context()) {
		writeProblem(w, Problem{Status: http.StatusServiceUnavailable, Title: "Ausgelastet"})
		return
	}
	p, err := pkg.Read(raw)
	if err == nil {
		_, err = pdx.CompileTemplate(p, "")
	}
	s.release()
	if err != nil {
		writeProblem(w, Problem{Type: typeBase + "vorlage", Status: http.StatusUnprocessableEntity, Title: "Vorlage fehlerhaft", Findings: findings(err)})
		return
	}
	v, created, err := s.store.Put(r.Context(), id, raw)
	if err != nil {
		s.storeError(w, err)
		return
	}
	if r.URL.Query().Get("activate") == "true" {
		if err := s.store.Activate(r.Context(), id, v.Version); err != nil {
			s.storeError(w, err)
			return
		}
		v.Active = true
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	w.Header().Set("Location", "/v1/templates/"+id+"/versions/"+v.Version)
	writeJSON(w, status, v)
}

func (s *Server) storeError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrNotFound) {
		writeProblem(w, Problem{Status: http.StatusNotFound, Title: "Vorlage oder Version unbekannt"})
		return
	}
	s.log.Error("Ablage", "fehler", err.Error())
	writeProblem(w, Problem{Status: http.StatusInternalServerError, Title: "Ablage nicht verfügbar"})
}

// listTemplates: GET /v1/templates.
func (s *Server) listTemplates(w http.ResponseWriter, r *http.Request) {
	ts, err := s.store.List(r.Context())
	if err != nil {
		s.storeError(w, err)
		return
	}
	if ts == nil {
		ts = []TemplateInfo{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"templates": ts})
}

// listVersions: GET /v1/templates/{id}/versions.
func (s *Server) listVersions(w http.ResponseWriter, r *http.Request) {
	vs, err := s.store.Versions(r.Context(), r.PathValue("id"))
	if err != nil {
		s.storeError(w, err)
		return
	}
	if len(vs) == 0 {
		s.storeError(w, ErrNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": r.PathValue("id"), "versions": vs})
}

// getVersion returns the package of a version as ZIP, for example for the
// comparison in the designer: GET /v1/templates/{id}/versions/{version}.
func (s *Server) getVersion(w http.ResponseWriter, r *http.Request) {
	raw, ver, err := s.store.Get(r.Context(), r.PathValue("id"), r.PathValue("version"))
	if err != nil {
		s.storeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Length", strconv.Itoa(len(raw)))
	w.Header().Set("X-Pdx-Template-Version", ver)
	w.Header().Set("ETag", `"`+digest(raw)+`"`)
	w.Write(raw)
}

// activate makes a version active: PUT /v1/templates/{id}/active with
// {"version": "2"}.
func (s *Server) activate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil || req.Version == "" {
		writeProblem(w, Problem{Status: http.StatusBadRequest, Title: `Rumpf muss {"version": "…"} sein`})
		return
	}
	if err := s.store.Activate(r.Context(), r.PathValue("id"), req.Version); err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, TemplateInfo{ID: r.PathValue("id"), Active: req.Version})
}
