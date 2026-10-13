package layout

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Resolver lädt Ressourcen wie Bilder und Anhänge. Der Designer wird später
// einen Resolver auf das Vorlagenpaket setzen.
type Resolver interface {
	Read(uri string) ([]byte, error)
}

// DirResolver liest Dateien relativ zu einem Verzeichnis. Netzwerkadressen
// werden abgelehnt, weil pdx nur in sich geschlossene Dateien erzeugt und
// keine Daten nachlädt.
type DirResolver struct {
	Dir string
}

// Read liest eine Datei. Erlaubt sind relative und absolute Pfade sowie
// file:-URIs.
func (r DirResolver) Read(uri string) ([]byte, error) {
	if u, err := url.Parse(uri); err == nil && u.Scheme != "" && len(u.Scheme) > 1 {
		if u.Scheme != "file" {
			return nil, fmt.Errorf("Schema %q wird nicht unterstützt, nur lokale Dateien", u.Scheme)
		}
		uri = u.Path
	}
	path := filepath.FromSlash(uri)
	if !filepath.IsAbs(path) {
		path = filepath.Join(r.Dir, path)
	}
	return os.ReadFile(path)
}

type denyResolver struct{}

func (denyResolver) Read(uri string) ([]byte, error) {
	return nil, errors.New("kein Resolver konfiguriert, " + strings.TrimSpace(uri) + " ist nicht lesbar")
}
