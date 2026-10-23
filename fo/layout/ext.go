package layout

import (
	"path"
	"strings"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/fo/tree"
)

// declarations reads the pdx extensions in fo:declarations.
//
//	<pdx:attachment src="factur-x.xml" name="factur-x.xml" mime-type="text/xml"
//	                relationship="Alternative" description="Invoice data"/>
//	<pdx:metadata title="..." author="..." subject="..." keywords="..." creator="..."/>
//
// Apache FOP ignores elements of foreign namespaces in fo:declarations.
func (e *engine) declarations(d *tree.Node) error {
	for _, n := range d.Children {
		if n.Kind != tree.Element || n.Space != tree.NSPDX {
			continue
		}
		switch n.Name {
		case "attachment":
			if err := e.attachment(n); err != nil {
				return err
			}
		case "metadata":
			m := &e.doc.Meta
			for _, f := range []struct {
				attr string
				dst  *string
			}{{"title", &m.Title}, {"author", &m.Author}, {"subject", &m.Subject}, {"keywords", &m.Keywords}, {"creator", &m.Creator}} {
				if v, ok := n.Attr(f.attr); ok {
					*f.dst = v
				}
			}
		default:
			return tree.Errorf(n, "unbekannte Erweiterung pdx:%s", n.Name)
		}
	}
	return nil
}

func (e *engine) attachment(n *tree.Node) error {
	src, ok := n.Attr("src")
	if !ok || srcURI(src) == "" {
		return tree.Errorf(n, "pdx:attachment: src fehlt")
	}
	src = srcURI(src)
	mime, ok := n.Attr("mime-type")
	if !ok || mime == "" {
		return tree.Errorf(n, "pdx:attachment: mime-type fehlt")
	}
	data, err := e.opts.Resolver.Read(src)
	if err != nil {
		return tree.Errorf(n, "pdx:attachment %q: %v", src, err)
	}
	name := attrOr(n, "name", path.Base(strings.ReplaceAll(src, "\\", "/")))
	e.doc.Attachments = append(e.doc.Attachments, areatree.Attachment{
		Name:         name,
		MIMEType:     mime,
		Relationship: areatree.Relationship(attrOr(n, "relationship", string(areatree.Unspecified))),
		Description:  attrOr(n, "description", ""),
		Data:         data,
	})
	return nil
}
