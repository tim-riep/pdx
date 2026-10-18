package server

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strconv"

	"github.com/tim-riep/pdx/sign"
	"github.com/tim-riep/pdx/verapdf"
)

// verifyReport ist das Ergebnis von veraPDF je Flavour.
type verifyReport struct {
	Profile   string          `json:"profil"`
	Compliant bool            `json:"konform"`
	Findings  []verifyFinding `json:"befunde,omitempty"`
	Exception string          `json:"ausnahme,omitempty"`
}

type verifyFinding struct {
	Specification string `json:"spezifikation"`
	Clause        string `json:"klausel"`
	Test          string `json:"test"`
	Description   string `json:"beschreibung"`
	FailedChecks  int    `json:"fehlschlaege"`
}

// verify prüft ein PDF mit veraPDF: POST /v1/verify?profile=pdfa-3b. Das
// PDF liegt nur für die Dauer der Prüfung in einer temporären Datei, die
// danach gelöscht wird.
func (s *Server) verify(w http.ResponseWriter, r *http.Request) {
	if s.Verifier == nil {
		writeProblem(w, Problem{Status: http.StatusNotImplemented, Title: "veraPDF ist nicht konfiguriert"})
		return
	}
	prof := r.URL.Query().Get("profile")
	if prof == "" {
		prof = "pdfa-3b"
	}
	flavours, err := verapdf.Flavours(prof)
	if err != nil {
		writeProblem(w, Problem{Status: http.StatusBadRequest, Title: "Profil ungültig", Detail: err.Error()})
		return
	}
	data, ok := readBody(w, r, s.cfg.MaxPDF)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.Timeout.Duration*4)
	defer cancel()
	if !s.acquire(ctx) {
		writeProblem(w, Problem{Status: http.StatusServiceUnavailable, Title: "Ausgelastet"})
		return
	}
	defer s.release()
	dir, err := os.MkdirTemp("", "pdx-verify-")
	if err != nil {
		writeProblem(w, Problem{Status: http.StatusInternalServerError, Title: "Temporäre Datei nicht möglich"})
		return
	}
	defer os.RemoveAll(dir)
	file := dir + "/dokument.pdf"
	if err := os.WriteFile(file, data, 0o600); err != nil {
		writeProblem(w, Problem{Status: http.StatusInternalServerError, Title: "Temporäre Datei nicht möglich"})
		return
	}
	all := true
	var reports []verifyReport
	for _, fl := range flavours {
		rep, err := s.Verifier.Validate(ctx, file, fl)
		if err != nil {
			writeProblem(w, Problem{Status: http.StatusBadGateway, Title: "veraPDF fehlgeschlagen"})
			return
		}
		vr := verifyReport{Profile: rep.Profile, Compliant: !rep.Failed(), Exception: rep.Exception}
		for _, f := range rep.Findings {
			vr.Findings = append(vr.Findings, verifyFinding{f.Specification, f.Clause, f.TestNumber, f.Description, f.FailedChecks})
		}
		all = all && vr.Compliant
		reports = append(reports, vr)
	}
	writeJSON(w, http.StatusOK, map[string]any{"konform": all, "berichte": reports})
}

// sign signiert ein PDF mit dem Signer des Dienstes nach PAdES:
// POST /v1/sign?level=B-T&field=&reason=&location=. Der Schlüssel bleibt
// im Dienst oder beim Signaturdienst, er verlässt nie den Speicher.
func (s *Server) sign(w http.ResponseWriter, r *http.Request) {
	if s.Signer == nil {
		writeProblem(w, Problem{Status: http.StatusNotImplemented, Title: "Signieren ist nicht konfiguriert"})
		return
	}
	q := r.URL.Query()
	data, ok := readBody(w, r, s.cfg.MaxPDF)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.Timeout.Duration*2)
	defer cancel()
	if !s.acquire(ctx) {
		writeProblem(w, Problem{Status: http.StatusServiceUnavailable, Title: "Ausgelastet"})
		return
	}
	defer s.release()
	opt := sign.PDFOptions{Level: sign.Level(q.Get("level")), Field: q.Get("field"), Reason: q.Get("reason"), Location: q.Get("location"), TSA: s.TSA}
	out, err := sign.SignPDF(ctx, data, s.Signer, opt)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			writeProblem(w, Problem{Status: http.StatusGatewayTimeout, Title: "Zeitlimit überschritten"})
			return
		}
		writeProblem(w, Problem{Type: typeBase + "signieren", Status: http.StatusUnprocessableEntity, Title: "Signieren fehlgeschlagen", Findings: splitErrors(err)})
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Length", strconv.Itoa(len(out)))
	w.Header().Set("Cache-Control", "no-store")
	w.Write(out)
}

// splitErrors zerlegt einen mit errors.Join verbundenen Fehler in Befunde.
func splitErrors(err error) []Finding {
	if j, ok := err.(interface{ Unwrap() []error }); ok {
		var out []Finding
		for _, e := range j.Unwrap() {
			out = append(out, splitErrors(e)...)
		}
		return out
	}
	return []Finding{{Message: err.Error()}}
}
