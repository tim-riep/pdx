// Package dssval prüft Signaturen mit dem EU DSS Validator in Docker. Er
// dient nur Tests: Das Image wird lokal aus dem gepinnten Quellstand gebaut
// (build.sh), der Container startet je Testlauf mit den Vertrauensankern
// der Test-PKI. Version, Quellstand und Image sind ausschließlich hier
// konfiguriert.
package dssval

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"software.sslmate.com/src/go-pkcs12"
)

const (
	// Version ist der Tag von esig/dss-demonstrations.
	Version = "6.5.1"
	// SourceCommit ist der Commit des Tags, build.sh prüft ihn.
	SourceCommit = "3c79488771671220542267129a9419586832ad55"
	// Image ist der Name des lokal gebauten Images.
	Image = "pdx-dss:6.5.1"
	// EnvEnable schaltet die Prüfung in Tests ein.
	EnvEnable = "PDX_DSS"
	// HostAlias ist der Name, unter dem der Container den Rechner des
	// Tests erreicht, etwa für OCSP und CRL der Test-PKI.
	HostAlias = "pdx-host"
)

// Enabled meldet, ob Tests mit DSS prüfen sollen.
func Enabled() bool { return os.Getenv(EnvEnable) == "1" }

// Server ist ein laufender DSS-Container.
type Server struct {
	URL    string
	id     string
	dir    string
	client *http.Client
}

// Start startet den Container mit trust als Vertrauensankern und wartet,
// bis die REST-Schnittstelle antwortet.
func Start(ctx context.Context, trust []*x509.Certificate) (*Server, error) {
	if out, err := exec.CommandContext(ctx, "docker", "image", "inspect", Image).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("Image %s fehlt, bitte dssval/build.sh ausführen (%s)", Image, strings.TrimSpace(string(out)))
	}
	dir, err := os.MkdirTemp("", "pdx-dss-")
	if err != nil {
		return nil, err
	}
	ts, err := pkcs12.Modern.EncodeTrustStore(trust, "pdx")
	if err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "trust.p12"), ts, 0o644); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	out, err := exec.CommandContext(ctx, "docker", "run", "-d", "--rm",
		"-p", "127.0.0.1::8080", "--add-host", HostAlias+":host-gateway",
		"-v", dir+":/usr/local/tomcat/webapps/ROOT/WEB-INF/classes/pdx:ro",
		Image).Output()
	if err != nil {
		os.RemoveAll(dir)
		return nil, fmt.Errorf("DSS-Container startet nicht: %v", err)
	}
	s := &Server{id: strings.TrimSpace(string(out)), dir: dir, client: &http.Client{Timeout: 2 * time.Minute}}
	port, err := exec.CommandContext(ctx, "docker", "port", s.id, "8080/tcp").Output()
	if err != nil {
		s.Stop()
		return nil, fmt.Errorf("Port des DSS-Containers unbekannt: %v", err)
	}
	addr := strings.TrimSpace(strings.Split(string(port), "\n")[0])
	s.URL = "http://" + addr
	// Der Portproxy von Docker nimmt Verbindungen schon vor Tomcat an, daher
	// ein kurzes Zeitlimit je Versuch.
	probe := &http.Client{Timeout: 2 * time.Second}
	var lastErr error
	deadline := time.Now().Add(3 * time.Minute)
	for {
		resp, err := probe.Get(s.URL + "/services/rest/validation")
		lastErr = err
		if err == nil {
			resp.Body.Close()
			// Tomcat öffnet den Port erst nach dem Start der Anwendung, jede
			// Antwort heißt bereit (CXF antwortet ohne Methode mit 500).
			return s, nil
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			logs, _ := exec.Command("docker", "logs", "--tail", "30", s.id).CombinedOutput()
			s.Stop()
			return nil, fmt.Errorf("DSS unter %s antwortet nicht (%v): %.300s", s.URL, lastErr, logs)
		}
		time.Sleep(time.Second)
	}
}

// Stop beendet den Container.
func (s *Server) Stop() {
	exec.Command("docker", "rm", "-f", s.id).Run()
	os.RemoveAll(s.dir)
}

// Signature ist das Ergebnis einer Signatur oder eines Zeitstempels im
// einfachen Bericht von DSS.
type Signature struct {
	ID            string
	Type          string // Signature oder Timestamp
	Format        string // etwa PAdES-BASELINE-LTA
	Indication    string // TOTAL_PASSED, INDETERMINATE, TOTAL_FAILED
	SubIndication string
	Errors        []string
	Warnings      []string
}

// Validate prüft ein signiertes Dokument mit der Standardrichtlinie.
func (s *Server) Validate(ctx context.Context, doc []byte) ([]Signature, error) {
	body, _ := json.Marshal(map[string]any{
		"signedDocument": map[string]any{"bytes": base64.StdEncoding.EncodeToString(doc), "name": "dokument.pdf"},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.URL+"/services/rest/validation/validateSignature", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DSS antwortet mit %s: %.500s", resp.Status, raw)
	}
	return parseReport(raw)
}

type message struct {
	Value string `json:"value"`
}

type token struct {
	ID            string `json:"Id"`
	Format        string `json:"SignatureFormat"`
	Indication    string `json:"Indication"`
	SubIndication string `json:"SubIndication"`
	AdES          *struct {
		Errors   []message `json:"Error"`
		Warnings []message `json:"Warning"`
	} `json:"AdESValidationDetails"`
	Timestamps *struct {
		Timestamp []token `json:"timestamp"`
	} `json:"Timestamps"`
}

// parseReport liest den einfachen Bericht aus der Antwort.
func parseReport(raw []byte) ([]Signature, error) {
	var r struct {
		Simple struct {
			Items []struct {
				Signature *token `json:"Signature"`
				Timestamp *token `json:"Timestamp"`
			} `json:"signatureOrTimestampOrEvidenceRecord"`
		} `json:"SimpleReport"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("Bericht von DSS nicht lesbar: %v", err)
	}
	var out []Signature
	add := func(typ string, t *token) {
		s := Signature{ID: t.ID, Type: typ, Format: t.Format, Indication: t.Indication, SubIndication: t.SubIndication}
		if t.AdES != nil {
			for _, m := range t.AdES.Errors {
				s.Errors = append(s.Errors, m.Value)
			}
			for _, m := range t.AdES.Warnings {
				s.Warnings = append(s.Warnings, m.Value)
			}
		}
		out = append(out, s)
	}
	for _, it := range r.Simple.Items {
		switch {
		case it.Signature != nil:
			add("Signature", it.Signature)
			if it.Signature.Timestamps != nil {
				for i := range it.Signature.Timestamps.Timestamp {
					add("Timestamp", &it.Signature.Timestamps.Timestamp[i])
				}
			}
		case it.Timestamp != nil:
			add("Timestamp", it.Timestamp)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("DSS findet keine Signatur")
	}
	return out, nil
}
