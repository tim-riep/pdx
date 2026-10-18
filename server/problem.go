package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/tim-riep/pdx/pdf/render"
	"github.com/tim-riep/pdx/template"
)

// Finding ist ein einzelner Befund, mit Vorlagen-ID, wenn bekannt.
type Finding struct {
	ID      string `json:"id,omitempty"`
	Line    int    `json:"line,omitempty"`
	Col     int    `json:"col,omitempty"`
	Message string `json:"message"`
}

// Problem ist eine Fehlerantwort nach RFC 9457 mit allen Befunden.
type Problem struct {
	Type     string    `json:"type"`
	Title    string    `json:"title"`
	Status   int       `json:"status"`
	Detail   string    `json:"detail,omitempty"`
	Findings []Finding `json:"findings,omitempty"`
}

// Typen der Probleme, als URI unter dem Dienst dokumentiert.
const (
	typeBase = "https://github.com/tim-riep/pdx/problems/"
)

func writeProblem(w http.ResponseWriter, p Problem) {
	if p.Type == "" {
		p.Type = "about:blank"
	}
	if p.Status == http.StatusServiceUnavailable {
		w.Header().Set("Retry-After", "1")
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(p.Status)
	json.NewEncoder(w).Encode(p)
}

// findings zerlegt Fehler von Vorlage und Rendern in Befunde.
func findings(err error) []Finding {
	var te template.Errors
	if errors.As(err, &te) {
		out := make([]Finding, len(te))
		for i, e := range te {
			out[i] = Finding{ID: e.ID, Line: e.Line, Col: e.Col, Message: e.Msg}
		}
		return out
	}
	var one *template.Error
	if errors.As(err, &one) {
		return []Finding{{ID: one.ID, Line: one.Line, Col: one.Col, Message: one.Msg}}
	}
	var ce render.CheckErrors
	if errors.As(err, &ce) {
		out := make([]Finding, len(ce))
		for i, e := range ce {
			out[i] = Finding{ID: e.ID, Message: e.Msg}
		}
		return out
	}
	return []Finding{{Message: err.Error()}}
}
