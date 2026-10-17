// Package pkg lädt, prüft und schreibt Vorlagenpakete: ZIP-Dateien oder
// Verzeichnisse mit template.xml und den Ordnern schema (Daten-XSD), assets
// (Schriften, Bilder, ICC), samples (Beispieldaten) und texts.
//
// Geschrieben wird deterministisch: Einträge sortiert, fester Zeitstempel,
// feste Kompression und template.xml in kanonischer Form. Gleiche Pakete
// ergeben byte-identische ZIP-Dateien, Änderungen bleiben in Git lesbar.
package pkg

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tim-riep/pdx/template"
)

// Grenzen gegen entartete Pakete (ZIP-Bomben).
const (
	MaxFiles     = 10_000
	MaxFileSize  = 64 << 20
	MaxTotalSize = 512 << 20
)

// Ordner, die ein Paket enthalten darf.
var dirs = []string{"schema", "assets", "samples", "texts"}

// zipTime ist der feste Zeitstempel aller Einträge.
var zipTime = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

// Package ist ein geladenes Vorlagenpaket.
type Package struct {
	Template *template.Template
	// Files enthält alle übrigen Dateien unter ihrem Pfad mit Schrägstrichen.
	Files map[string][]byte
}

// Open lädt ein Paket aus einer ZIP-Datei oder einem Verzeichnis.
func Open(p string) (*Package, error) {
	st, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	if st.IsDir() {
		return ReadDir(p)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	return Read(data)
}

// Read lädt ein Paket aus ZIP-Daten.
func Read(data []byte) (*Package, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("Vorlagenpaket ist keine ZIP-Datei: %w", err)
	}
	if len(zr.File) > MaxFiles {
		return nil, fmt.Errorf("Vorlagenpaket hat mehr als %d Dateien", MaxFiles)
	}
	files := map[string][]byte{}
	var total int64
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, "/") {
			continue
		}
		if f.UncompressedSize64 > MaxFileSize {
			return nil, fmt.Errorf("%s ist größer als %d MiB", f.Name, MaxFileSize>>20)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Name, err)
		}
		b, err := io.ReadAll(io.LimitReader(rc, MaxFileSize+1))
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Name, err)
		}
		if len(b) > MaxFileSize {
			return nil, fmt.Errorf("%s ist größer als %d MiB", f.Name, MaxFileSize>>20)
		}
		total += int64(len(b))
		if total > MaxTotalSize {
			return nil, fmt.Errorf("Vorlagenpaket ist entpackt größer als %d MiB", MaxTotalSize>>20)
		}
		if _, dup := files[f.Name]; dup {
			return nil, fmt.Errorf("%s ist doppelt im Paket", f.Name)
		}
		files[f.Name] = b
	}
	return build(files)
}

// ReadDir lädt ein Paket aus einem Verzeichnis, etwa für die Entwicklung.
func ReadDir(dir string) (*Package, error) {
	files := map[string][]byte{}
	var total int64
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(rel)
		if strings.HasPrefix(path.Base(name), ".") {
			return nil // .DS_Store und ähnliche
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Size() > MaxFileSize {
			return fmt.Errorf("%s ist größer als %d MiB", name, MaxFileSize>>20)
		}
		total += info.Size()
		if total > MaxTotalSize || len(files) >= MaxFiles {
			return fmt.Errorf("Verzeichnis ist zu groß für ein Vorlagenpaket")
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		files[name] = b
		return nil
	})
	if err != nil {
		return nil, err
	}
	return build(files)
}

// checkPath prüft einen Pfad im Paket.
func checkPath(name string) error {
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") || path.Clean(name) != name ||
		name == ".." || strings.HasPrefix(name, "../") {
		return fmt.Errorf("Pfad %q im Paket ist unzulässig", name)
	}
	if name == "template.xml" {
		return nil
	}
	top := strings.SplitN(name, "/", 2)[0]
	for _, d := range dirs {
		if top == d && strings.Contains(name, "/") {
			return nil
		}
	}
	return fmt.Errorf("Datei %q gehört nicht in ein Vorlagenpaket, erlaubt sind template.xml und die Ordner %s", name, strings.Join(dirs, ", "))
}

func build(files map[string][]byte) (*Package, error) {
	for name := range files {
		if err := checkPath(name); err != nil {
			return nil, err
		}
	}
	src, ok := files["template.xml"]
	if !ok {
		return nil, fmt.Errorf("template.xml fehlt im Paket")
	}
	tpl, err := template.Parse(src)
	if err != nil {
		return nil, err
	}
	delete(files, "template.xml")
	p := &Package{Template: tpl, Files: files}
	if err := p.checkRefs(); err != nil {
		return nil, err
	}
	return p, nil
}

// checkRefs prüft, dass Dateien, auf die die Vorlage verweist, im Paket sind.
func (p *Package) checkRefs() error {
	var errs template.Errors
	need := func(n *template.Node, attr, file string) {
		if _, ok := p.Files[file]; !ok {
			errs = append(errs, &template.Error{ID: n.NearestID(), Line: n.Line, Col: n.Col,
				Msg: fmt.Sprintf("%s verweist auf %q, die Datei fehlt im Paket", attr, file)})
		}
	}
	root := p.Template.Root
	if s, ok := root.Attr("schema"); ok {
		need(root, "schema", s)
	}
	var walk func(n *template.Node)
	walk = func(n *template.Node) {
		if n.Kind != template.Element {
			return
		}
		switch n.Name {
		case "image":
			need(n, "src", n.Get("src", ""))
		case "attachment":
			if s := n.Get("source", ""); s != "data" {
				need(n, "source", s)
			}
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)
	if len(errs) > 0 {
		return errs
	}
	return nil
}

// File liefert eine Datei aus dem Paket.
func (p *Package) File(name string) ([]byte, bool) {
	b, ok := p.Files[name]
	return b, ok
}

// Names liefert die Pfade aller Dateien außer template.xml, sortiert.
func (p *Package) Names() []string {
	names := make([]string, 0, len(p.Files))
	for n := range p.Files {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Samples liefert die Pfade der Beispieldaten.
func (p *Package) Samples() []string {
	var out []string
	for _, n := range p.Names() {
		if strings.HasPrefix(n, "samples/") {
			out = append(out, n)
		}
	}
	return out
}

// Schemas liefert das Haupt-XSD (aus template/@schema) und alle XSD-Dateien
// unter schema/ für xs:import und xs:include.
func (p *Package) Schemas() (main string, all map[string][]byte) {
	all = map[string][]byte{}
	for n, b := range p.Files {
		if strings.HasPrefix(n, "schema/") && strings.HasSuffix(n, ".xsd") {
			all[n] = b
		}
	}
	main, _ = p.Template.Root.Attr("schema")
	return main, all
}

// Write schreibt das Paket deterministisch als ZIP.
func (p *Package) Write(w io.Writer) error {
	zw := zip.NewWriter(w)
	zw.RegisterCompressor(zip.Deflate, func(out io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(out, flate.BestCompression)
	})
	put := func(name string, data []byte) error {
		h := &zip.FileHeader{Name: name, Method: zip.Deflate, Modified: zipTime}
		h.SetMode(0o644)
		f, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		_, err = f.Write(data)
		return err
	}
	if err := put("template.xml", template.Write(p.Template)); err != nil {
		return err
	}
	for _, n := range p.Names() {
		if err := put(n, p.Files[n]); err != nil {
			return err
		}
	}
	return zw.Close()
}

// Bytes liefert das Paket als ZIP-Daten.
func (p *Package) Bytes() ([]byte, error) {
	var b bytes.Buffer
	if err := p.Write(&b); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
