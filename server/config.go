// Package server is the render service of pdx over HTTP. It is stateless
// except for the template store, keeps no payload data beyond a request
// and writes no payload data to logs or metrics.
package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"
)

// Permissions of an API key.
const (
	RightRender    = "render"
	RightTemplates = "templates"
	RightVerify    = "verify"
	RightSign      = "sign"
)

var allRights = []string{RightRender, RightTemplates, RightVerify, RightSign}

// APIKey is a key in the configuration. Only the SHA-256 hash is stored,
// never the key itself.
type APIKey struct {
	// Name appears in logs instead of the key.
	Name   string   `json:"name"`
	SHA256 string   `json:"sha256"`
	Rights []string `json:"rights"`
	hash   []byte
}

// Duration reads durations such as "30s" from JSON.
type Duration struct{ time.Duration }

func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	d.Duration = v
	return nil
}

func (d Duration) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

// Config is the configuration of the service.
type Config struct {
	// Addr is the address, default :8080.
	Addr string `json:"addr"`
	// Keys are the API keys. Without keys the service rejects every request
	// except Health and Readiness.
	Keys []APIKey `json:"keys"`
	// MaxBody limits the body of a request in bytes, default 10 MiB.
	MaxBody int64 `json:"maxBody"`
	// MaxTemplate limits an uploaded template package, default 50 MiB.
	MaxTemplate int64 `json:"maxTemplate"`
	// MaxPDF limits a PDF for verify and sign, default 50 MiB.
	MaxPDF int64 `json:"maxPDF"`
	// MaxBatch limits the ZIP of a batch, default 100 MiB, and
	// MaxBatchEntries the number of records in it, default 1000.
	MaxBatch        int64 `json:"maxBatch"`
	MaxBatchEntries int   `json:"maxBatchEntries"`
	// BatchTimeout limits the run time of a batch, default 10m.
	BatchTimeout Duration `json:"batchTimeout"`
	// VeraPDF enables /v1/verify: "docker", "local" or a path to the
	// veraPDF CLI. Empty means off.
	VeraPDF string `json:"verapdf"`
	// Signer enables /v1/sign: pkcs12:datei.p12 (password from
	// PDX_SIGN_PASSWORD) or remote:https://dienst (token from
	// PDX_SIGN_TOKEN). Empty means off.
	Signer string `json:"signer"`
	// TSA is the time stamp service for /v1/sign from B-T upwards.
	TSA string `json:"tsa"`
	// MaxConcurrent limits concurrent render operations, default 8.
	MaxConcurrent int `json:"maxConcurrent"`
	// Timeout limits the run time of a request, default 30s.
	Timeout Duration `json:"timeout"`
	// QueueWait is the longest wait for a free slot, after which the
	// service responds with 503, default 5s.
	QueueWait Duration `json:"queueWait"`
	// TemplateDir is the directory of the template store.
	TemplateDir string `json:"templateDir"`
}

// LoadConfig reads the configuration from a JSON file.
func LoadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("Konfiguration %s: %v", path, err)
	}
	return &c, c.Check()
}

// Check sets defaults and validates the configuration, all findings at once.
func (c *Config) Check() error {
	if c.Addr == "" {
		c.Addr = ":8080"
	}
	if c.MaxBody == 0 {
		c.MaxBody = 10 << 20
	}
	if c.MaxTemplate == 0 {
		c.MaxTemplate = 50 << 20
	}
	if c.MaxPDF == 0 {
		c.MaxPDF = 50 << 20
	}
	if c.MaxBatch == 0 {
		c.MaxBatch = 100 << 20
	}
	if c.MaxBatchEntries == 0 {
		c.MaxBatchEntries = 1000
	}
	if c.BatchTimeout.Duration == 0 {
		c.BatchTimeout.Duration = 10 * time.Minute
	}
	if c.MaxConcurrent == 0 {
		c.MaxConcurrent = 8
	}
	if c.Timeout.Duration == 0 {
		c.Timeout.Duration = 30 * time.Second
	}
	if c.QueueWait.Duration == 0 {
		c.QueueWait.Duration = 5 * time.Second
	}
	var errs []error
	names := map[string]bool{}
	hashes := map[string]string{}
	for i := range c.Keys {
		k := &c.Keys[i]
		h, err := hex.DecodeString(k.SHA256)
		if err != nil || len(h) != sha256.Size {
			errs = append(errs, fmt.Errorf("Schlüssel %q: sha256 ist kein SHA-256-Hash in Hex", k.Name))
		}
		k.hash = h
		// The same hash would mean the same key with unclear permissions.
		if other, dup := hashes[strings.ToLower(k.SHA256)]; dup && len(h) == sha256.Size {
			errs = append(errs, fmt.Errorf("Schlüssel %q und %q haben denselben Hash", other, k.Name))
		}
		hashes[strings.ToLower(k.SHA256)] = k.Name
		if k.Name == "" || names[k.Name] {
			errs = append(errs, fmt.Errorf("Schlüssel %d: Name fehlt oder ist doppelt", i+1))
		}
		names[k.Name] = true
		if len(k.Rights) == 0 {
			errs = append(errs, fmt.Errorf("Schlüssel %q: keine Rechte", k.Name))
		}
		for _, r := range k.Rights {
			if !slices.Contains(allRights, r) {
				errs = append(errs, fmt.Errorf("Schlüssel %q: Recht %q ist unbekannt, erlaubt sind %s", k.Name, r, strings.Join(allRights, ", ")))
			}
		}
	}
	if c.MaxBody < 0 || c.MaxConcurrent < 0 {
		errs = append(errs, errors.New("maxBody und maxConcurrent dürfen nicht negativ sein"))
	}
	return errors.Join(errs...)
}

// HashKey returns the hash of a key for the configuration.
func HashKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}

// lookup finds the key for the plain text. The hash is compared in
// constant time, across all keys.
func (c *Config) lookup(key string) *APIKey {
	h := sha256.Sum256([]byte(key))
	var found *APIKey
	for i := range c.Keys {
		if subtle.ConstantTimeCompare(h[:], c.Keys[i].hash) == 1 {
			found = &c.Keys[i]
		}
	}
	return found
}
