// Package dssval validates signatures with the EU DSS validator in Docker.
// It serves only tests: the image is built locally from the pinned source
// (build.sh), the container starts for each test run with the trust anchors
// of the test PKI. Version, source and image are configured exclusively
// here.
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
	// Version is the tag of esig/dss-demonstrations.
	Version = "6.5.1"
	// SourceCommit is the commit of the tag, build.sh checks it.
	SourceCommit = "3c79488771671220542267129a9419586832ad55"
	// Image is the name of the locally built image.
	Image = "pdx-dss:6.5.1"
	// EnvEnable enables the validation in tests.
	EnvEnable = "PDX_DSS"
	// HostAlias is the name under which the container reaches the host of
	// the test, for example for OCSP and CRL of the test PKI.
	HostAlias = "pdx-host"
)

// Enabled reports whether tests should validate with DSS.
func Enabled() bool { return os.Getenv(EnvEnable) == "1" }

// Server is a running DSS container.
type Server struct {
	URL    string
	id     string
	dir    string
	client *http.Client
}

// Start starts the container with trust as trust anchors and waits until
// the REST interface responds.
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
	// The Docker port proxy accepts connections before Tomcat does, hence
	// a short timeout per attempt.
	probe := &http.Client{Timeout: 2 * time.Second}
	var lastErr error
	deadline := time.Now().Add(3 * time.Minute)
	for {
		resp, err := probe.Get(s.URL + "/services/rest/validation")
		lastErr = err
		if err == nil {
			resp.Body.Close()
			// Tomcat opens the port only after the application has started, any
			// response means ready (CXF answers 500 without a method).
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

// Stop stops the container.
func (s *Server) Stop() {
	exec.Command("docker", "rm", "-f", s.id).Run()
	os.RemoveAll(s.dir)
}

// Signature is the result of a signature or a timestamp in the simple
// report of DSS.
type Signature struct {
	ID            string
	Type          string // Signature or Timestamp
	Format        string // such as PAdES-BASELINE-LTA
	Indication    string // TOTAL_PASSED, INDETERMINATE, TOTAL_FAILED
	SubIndication string
	Errors        []string
	Warnings      []string
}

// Validate checks a signed document with the default policy.
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

// parseReport reads the simple report from the response.
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
