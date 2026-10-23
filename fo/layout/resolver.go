package layout

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Resolver loads resources such as images and attachments. The designer will
// later set a resolver on the template package.
type Resolver interface {
	Read(uri string) ([]byte, error)
}

// DirResolver reads files relative to a directory. Network addresses
// are rejected because pdx only produces self-contained files and
// does not load data afterwards.
type DirResolver struct {
	Dir string
}

// Read reads a file. Relative and absolute paths as well as
// file: URIs are allowed.
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
