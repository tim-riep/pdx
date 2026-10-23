package app

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tim-riep/pdx/dataschema"
	"github.com/tim-riep/pdx/designer/internal/model"
	"github.com/tim-riep/pdx/designer/internal/schema"
	"github.com/tim-riep/pdx/template"
)

// SchemaService returns the data tree of the data XSD and loads new schemas.
type SchemaService struct{ c *Core }

// NewSchemaService returns the view on the shared state.
func NewSchemaService(c *Core) *SchemaService { return &SchemaService{c} }

// Tree returns the data tree. Without a data XSD it is empty.
func (s *SchemaService) Tree() (*schema.Schema, error) {
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	if s.c.doc == nil {
		return nil, errors.New("keine Vorlage geöffnet")
	}
	return s.c.schemaLocked()
}

// schemaCache holds the prepared schema as long as the XSD, the data root
// and the namespaces of the template do not change.
type schemaCache struct {
	key string
	s   *schema.Schema
	err error
}

func (c *Core) schemaLocked() (*schema.Schema, error) {
	p := c.doc.Pkg
	main, files := p.Schemas()
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00", main, p.Template.Root.Get("dataRoot", ""))
	if ns := p.Template.Root.First("namespaces"); ns != nil {
		for _, n := range ns.Elements("ns") {
			fmt.Fprintf(h, "%s=%s\x00", n.Get("prefix", ""), n.Get("uri", ""))
		}
	}
	for _, name := range p.Names() {
		if b, ok := files[name]; ok {
			fmt.Fprintf(h, "%s\x00", name)
			h.Write(b)
		}
	}
	key := hex.EncodeToString(h.Sum(nil))
	if c.schema.key != key {
		s, err := schema.Load(p)
		c.schema = schemaCache{key: key, s: s, err: err}
	}
	return c.schema.s, c.schema.err
}

// Import loads a data XSD from a file into the package (schema/) and
// sets it as the schema of the template. XSD files in the same folder come
// along for xs:import and xs:include. root chooses the data root, empty takes
// the only global element. The step can be undone.
func (s *SchemaService) Import(path, root string) (Result, error) {
	x, err := readXSD(path, root)
	if err != nil {
		return Result{}, err
	}
	ts := NewTemplateService(s.c)
	return ts.change(func(d *model.Document) (string, error) {
		return "", d.Apply("Daten-XSD laden", model.Command{Op: "batch", Cmds: x.commands(d.Pkg.Template)})
	})
}

// xsdImport is a read data XSD with a chosen data root.
type xsdImport struct {
	main  string            // schema/<file>.xsd
	files map[string][]byte // all XSD files under schema/
	root  *dataschema.Decl
}

// readXSD reads a data XSD and the XSD files in the same folder (for
// xs:import and xs:include) and chooses the data root.
func readXSD(path, root string) (*xsdImport, error) {
	main, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("XSD %s nicht lesbar: %v", path, err)
	}
	files := map[string][]byte{"schema/" + filepath.Base(path): main}
	others, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "*.xsd"))
	for _, o := range others {
		if o == path {
			continue
		}
		if b, err := os.ReadFile(o); err == nil {
			files["schema/"+filepath.Base(o)] = b
		}
	}
	mainName := "schema/" + filepath.Base(path)
	m, err := dataschema.FromXSD(mainName, files)
	if err != nil {
		return nil, err
	}
	roots := m.Roots()
	var rootDecl *dataschema.Decl
	for _, r := range roots {
		if root == "" && len(roots) == 1 || r.Name.Local == root || strings.HasSuffix(root, ":"+r.Name.Local) {
			rootDecl = r
		}
	}
	if rootDecl == nil {
		var names []string
		for _, r := range roots {
			names = append(names, r.Name.Local)
		}
		return nil, fmt.Errorf("Datenwurzel wählen: %s", strings.Join(names, ", "))
	}
	return &xsdImport{main: mainName, files: files, root: rootDecl}, nil
}

// commands returns the commands that put the XSD into the package and set
// it as the schema of the template.
func (x *xsdImport) commands(t *template.Template) []model.Command {
	var cmds []model.Command
	names := make([]string, 0, len(x.files))
	for name := range x.files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		cmds = append(cmds, model.Command{Op: "setFile", Name: name, Data: x.files[name]})
	}
	prefix, nsCmd := ensurePrefix(t, x.root.Name.Space)
	if nsCmd != nil {
		cmds = append(cmds, *nsCmd)
	}
	rootRef := model.Ref(t.Root)
	return append(cmds,
		model.Command{Op: "setAttr", Ref: rootRef, Name: "schema", Value: x.main},
		model.Command{Op: "setAttr", Ref: rootRef, Name: "dataRoot", Value: x.rootName(prefix)})
}

func (x *xsdImport) rootName(prefix string) string {
	if prefix != "" {
		return prefix + ":" + x.root.Name.Local
	}
	return x.root.Name.Local
}

// ensurePrefix returns the prefix of a namespace from the template or
// a command that declares a new one.
func ensurePrefix(t *template.Template, uri string) (string, *model.Command) {
	if uri == "" {
		return "", nil
	}
	ns := t.Root.First("namespaces")
	used := map[string]bool{}
	if ns != nil {
		for _, n := range ns.Elements("ns") {
			if n.Get("uri", "") == uri {
				return n.Get("prefix", ""), nil
			}
			used[n.Get("prefix", "")] = true
		}
	}
	prefix := "d"
	for i := 2; used[prefix]; i++ {
		prefix = fmt.Sprintf("d%d", i)
	}
	el := fragment(template.NewElement("ns", template.Attr{Name: "prefix", Value: prefix}, template.Attr{Name: "uri", Value: uri}))
	if ns != nil {
		return prefix, &model.Command{Op: "insert", Parent: model.Ref(ns), Index: -1, XML: el}
	}
	return prefix, &model.Command{Op: "insert", Parent: model.Ref(t.Root), Index: 0, XML: "<namespaces>" + el + "</namespaces>"}
}

// ImportDialog asks for a data XSD and loads it.
func (s *SchemaService) ImportDialog(root string) (Result, error) {
	if s.c.dialogs == nil {
		return Result{}, ErrNoDialogs
	}
	path, err := s.c.dialogs.OpenFile("Daten-XSD laden", []Filter{{Name: "XML Schema (*.xsd)", Pattern: "*.xsd"}}, false)
	if err != nil || path == "" {
		return Result{}, err
	}
	return s.Import(path, root)
}
