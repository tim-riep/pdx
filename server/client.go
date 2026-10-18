package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client spricht mit einem pdx-server, etwa aus dem Designer oder dem
// Lasttest. Der Schlüssel bleibt im Speicher und steht in keiner Meldung.
type Client struct {
	URL  string
	key  string
	HTTP *http.Client
}

// NewClient legt einen Client an. Unverschlüsseltes HTTP ist nur für
// localhost erlaubt, sonst ginge der Schlüssel im Klartext über das Netz.
func NewClient(base, key string) (*Client, error) {
	base = strings.TrimSuffix(strings.TrimSpace(base), "/")
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, fmt.Errorf("Adresse %q ist keine HTTP-Adresse", base)
	}
	host := u.Hostname()
	if u.Scheme == "http" && host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return nil, fmt.Errorf("Adresse %s: außer für localhost ist nur HTTPS erlaubt", base)
	}
	return &Client{URL: base, key: key, HTTP: &http.Client{Timeout: 60 * time.Second}}, nil
}

// ProblemError ist eine Fehlerantwort des Dienstes mit allen Befunden.
type ProblemError struct{ Problem Problem }

func (e *ProblemError) Error() string {
	var b strings.Builder
	b.WriteString(e.Problem.Title)
	if e.Problem.Detail != "" {
		b.WriteString(": " + e.Problem.Detail)
	}
	for _, f := range e.Problem.Findings {
		b.WriteString("\n")
		if f.ID != "" {
			b.WriteString("[" + f.ID + "] ")
		}
		b.WriteString(f.Message)
	}
	return b.String()
}

func (c *Client) do(ctx context.Context, method, path, ctype string, body []byte) ([]byte, http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.URL+path, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	if c.key != "" {
		req.Header.Set("Authorization", "Bearer "+c.key)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("pdx-server nicht erreichbar: %w", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 256<<20))
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode >= 300 {
		var p Problem
		if json.Unmarshal(b, &p) != nil || p.Title == "" {
			p = Problem{Status: resp.StatusCode, Title: resp.Status}
		}
		return nil, nil, &ProblemError{p}
	}
	return b, resp.Header, nil
}

func (c *Client) getJSON(ctx context.Context, path string, v any) error {
	b, _, err := c.do(ctx, http.MethodGet, path, "", nil)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// Templates listet die Vorlagen.
func (c *Client) Templates(ctx context.Context) ([]TemplateInfo, error) {
	var r struct{ Templates []TemplateInfo }
	return r.Templates, c.getJSON(ctx, "/v1/templates", &r)
}

// Versions listet die Versionen einer Vorlage, ErrNotFound ohne Versionen.
func (c *Client) Versions(ctx context.Context, id string) ([]Version, error) {
	var r struct{ Versions []Version }
	err := c.getJSON(ctx, "/v1/templates/"+url.PathEscape(id)+"/versions", &r)
	if pe, ok := err.(*ProblemError); ok && pe.Problem.Status == http.StatusNotFound {
		return nil, ErrNotFound
	}
	return r.Versions, err
}

// Download holt das Paket einer Version.
func (c *Client) Download(ctx context.Context, id, version string) ([]byte, error) {
	b, _, err := c.do(ctx, http.MethodGet, "/v1/templates/"+url.PathEscape(id)+"/versions/"+url.PathEscape(version), "", nil)
	return b, err
}

// Upload legt ein Paket als neue Version ab. created meldet, ob die
// Version neu ist.
func (c *Client) Upload(ctx context.Context, id string, zip []byte, activate bool) (Version, error) {
	path := "/v1/templates/" + url.PathEscape(id)
	if activate {
		path += "?activate=true"
	}
	var v Version
	b, _, err := c.do(ctx, http.MethodPost, path, "application/zip", zip)
	if err != nil {
		return v, err
	}
	return v, json.Unmarshal(b, &v)
}

// Activate macht eine Version aktiv.
func (c *Client) Activate(ctx context.Context, id, version string) error {
	body, _ := json.Marshal(map[string]string{"version": version})
	_, _, err := c.do(ctx, http.MethodPut, "/v1/templates/"+url.PathEscape(id)+"/active", "application/json", body)
	return err
}

// Render setzt einen Datensatz. query sind die Parameter wie profile.
func (c *Client) Render(ctx context.Context, id string, data []byte, query url.Values) ([]byte, error) {
	path := "/v1/render/" + url.PathEscape(id)
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	b, _, err := c.do(ctx, http.MethodPost, path, "application/xml", data)
	return b, err
}

// String verrät den Schlüssel nicht.
func (c *Client) String() string { return "Client(" + c.URL + ")" }
