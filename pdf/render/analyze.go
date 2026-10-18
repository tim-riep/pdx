package render

import (
	"strings"
	"unicode/utf8"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/pdf/profile"
)

// StructNode ist ein Strukturelement für Werkzeuge wie den Designer: Typ,
// Vorlagen-ID, eigener Text (gekürzt) und Kinder in Lesereihenfolge.
type StructNode struct {
	Type string        `json:"type"`
	ID   string        `json:"id"`
	Text string        `json:"text"`
	Alt  string        `json:"alt"`
	Lang string        `json:"lang"`
	Page int           `json:"page"` // ab 1, 0 ohne Lage
	Kids []*StructNode `json:"kids"`
}

// maxText begrenzt den Text je Knoten.
const maxText = 80

// Analyze plant den Strukturbaum eines Dokuments, ohne ein PDF zu schreiben,
// und prüft ihn gegen den Anspruch wie beim Schreiben. Der Baum steht in
// Lesereihenfolge, die Befunde sind leer, wenn der Anspruch keine
// Barrierefreiheit verlangt.
func Analyze(doc *areatree.Document, c profile.Claim) (*StructNode, CheckErrors) {
	t := newTagger(doc)
	t.pdf2 = c.Version == "2.0"
	t.plan()
	var errs CheckErrors
	if c.Level == "A" || c.UA > 0 {
		if err := t.check(c); err != nil {
			errs, _ = err.(CheckErrors)
		}
	}
	return t.node(t.root), errs
}

func (t *tagger) node(se *structElem) *StructNode {
	n := &StructNode{Type: se.typ, ID: se.id, Alt: se.alt, Lang: se.lang, Kids: []*StructNode{}}
	if t.pdf2 && n.Type == "Note" {
		n.Type = "FENote"
	}
	if se.located {
		n.Page = se.page + 1
	}
	if b := t.texts[se]; b != nil {
		n.Text = shorten(strings.Join(strings.Fields(b.String()), " "))
	}
	for _, k := range se.kids {
		if k.elem != nil {
			n.Kids = append(n.Kids, t.node(k.elem))
		}
	}
	return n
}

func shorten(s string) string {
	if utf8.RuneCountInString(s) <= maxText {
		return s
	}
	r := []rune(s)
	return string(r[:maxText-1]) + "…"
}
