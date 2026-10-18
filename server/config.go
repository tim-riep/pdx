// Package server ist der Render-Dienst von pdx über HTTP. Er ist
// zustandslos bis auf die Vorlagenablage, hält keine Nutzdaten über eine
// Anfrage hinaus und schreibt keine Nutzdaten in Logs oder Metriken.
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

// Rechte eines API-Schlüssels.
const (
	RightRender    = "render"
	RightTemplates = "templates"
	RightVerify    = "verify"
	RightSign      = "sign"
)

var allRights = []string{RightRender, RightTemplates, RightVerify, RightSign}

// APIKey ist ein Schlüssel in der Konfiguration. Gespeichert ist nur der
// SHA-256-Hash, nie der Schlüssel selbst.
type APIKey struct {
	// Name erscheint in Logs statt des Schlüssels.
	Name   string   `json:"name"`
	SHA256 string   `json:"sha256"`
	Rights []string `json:"rights"`
	hash   []byte
}

// Duration liest Zeitangaben wie "30s" aus JSON.
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

// Config ist die Konfiguration des Dienstes.
type Config struct {
	// Addr ist die Adresse, Vorgabe :8080.
	Addr string `json:"addr"`
	// Keys sind die API-Schlüssel. Ohne Schlüssel lehnt der Dienst jede
	// Anfrage außer Health und Readiness ab.
	Keys []APIKey `json:"keys"`
	// MaxBody begrenzt den Rumpf einer Anfrage in Bytes, Vorgabe 10 MiB.
	MaxBody int64 `json:"maxBody"`
	// MaxConcurrent begrenzt gleichzeitige Rendervorgänge, Vorgabe 8.
	MaxConcurrent int `json:"maxConcurrent"`
	// Timeout begrenzt die Laufzeit einer Anfrage, Vorgabe 30s.
	Timeout Duration `json:"timeout"`
	// QueueWait ist die längste Wartezeit auf einen freien Platz, danach
	// antwortet der Dienst mit 503, Vorgabe 5s.
	QueueWait Duration `json:"queueWait"`
	// TemplateDir ist das Verzeichnis der Vorlagenablage.
	TemplateDir string `json:"templateDir"`
}

// LoadConfig liest die Konfiguration aus einer JSON-Datei.
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

// Check setzt Vorgaben und prüft die Konfiguration, alle Befunde auf einmal.
func (c *Config) Check() error {
	if c.Addr == "" {
		c.Addr = ":8080"
	}
	if c.MaxBody == 0 {
		c.MaxBody = 10 << 20
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
	for i := range c.Keys {
		k := &c.Keys[i]
		h, err := hex.DecodeString(k.SHA256)
		if err != nil || len(h) != sha256.Size {
			errs = append(errs, fmt.Errorf("Schlüssel %q: sha256 ist kein SHA-256-Hash in Hex", k.Name))
		}
		k.hash = h
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

// HashKey liefert den Hash eines Schlüssels für die Konfiguration.
func HashKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}

// lookup sucht den Schlüssel zum Klartext. Verglichen wird der Hash in
// konstanter Zeit, über alle Schlüssel.
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
