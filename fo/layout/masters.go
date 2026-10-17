package layout

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/tim-riep/pdx/fo/tree"
)

// specKind ist die Art einer Teilfolge in fo:page-sequence-master.
type specKind uint8

const (
	specSingle specKind = iota
	specRepeat
	specAlternatives
)

// pageSpec ist eine Teilfolge (sub-sequence-specifier).
type pageSpec struct {
	kind   specKind
	master string
	max    int // -1 unbegrenzt
	conds  []condRef
	node   *tree.Node
}

// condRef ist eine fo:conditional-page-master-reference.
type condRef struct {
	master   string
	position string // first, last, rest, only, any
	oddEven  string // odd, even, any
	blank    string // blank, not-blank, any
	node     *tree.Node
}

// seqMaster ist eine fo:page-sequence-master.
type seqMaster struct {
	node  *tree.Node
	specs []pageSpec
}

func (e *engine) readSequenceMaster(n *tree.Node) error {
	name, ok := n.Attr("master-name")
	if !ok || name == "" {
		return tree.Errorf(n, "master-name fehlt")
	}
	if _, dup := e.masters[name]; dup {
		return tree.Errorf(n, "master-name %q ist doppelt", name)
	}
	if _, dup := e.seqMasters[name]; dup {
		return tree.Errorf(n, "master-name %q ist doppelt", name)
	}
	sm := &seqMaster{node: n}
	for _, c := range n.Children {
		if c.Kind != tree.Element {
			continue
		}
		max, err := maxRepeats(c)
		if err != nil {
			return err
		}
		switch {
		case c.IsFO("single-page-master-reference"):
			sm.specs = append(sm.specs, pageSpec{kind: specSingle, master: attrOr(c, "master-reference", ""), max: 1, node: c})
		case c.IsFO("repeatable-page-master-reference"):
			sm.specs = append(sm.specs, pageSpec{kind: specRepeat, master: attrOr(c, "master-reference", ""), max: max, node: c})
		case c.IsFO("repeatable-page-master-alternatives"):
			sp := pageSpec{kind: specAlternatives, max: max, node: c}
			for _, cc := range c.Children {
				if !cc.IsFO("conditional-page-master-reference") {
					continue
				}
				sp.conds = append(sp.conds, condRef{
					master:   attrOr(cc, "master-reference", ""),
					position: attrOr(cc, "page-position", "any"),
					oddEven:  attrOr(cc, "odd-or-even", "any"),
					blank:    attrOr(cc, "blank-or-not-blank", "any"),
					node:     cc,
				})
			}
			if len(sp.conds) == 0 {
				return tree.Errorf(c, "fo:repeatable-page-master-alternatives ohne fo:conditional-page-master-reference")
			}
			sm.specs = append(sm.specs, sp)
		}
	}
	if len(sm.specs) == 0 {
		return tree.Errorf(n, "fo:page-sequence-master ohne Teilfolge")
	}
	e.seqMasters[name] = sm
	return nil
}

func maxRepeats(n *tree.Node) (int, error) {
	v, ok := n.Attr("maximum-repeats")
	if !ok || v == "no-limit" {
		return -1, nil
	}
	m, err := strconv.Atoi(v)
	if err != nil || m < 0 {
		return 0, tree.Errorf(n, "maximum-repeats %q ist ungültig", v)
	}
	return m, nil
}

// checkMasters prüft, dass alle Verweise auf einfache Seitenvorlagen zeigen.
func (e *engine) checkMasters() error {
	for _, sm := range e.seqMasters {
		for _, sp := range sm.specs {
			refs := []struct {
				name string
				node *tree.Node
			}{{sp.master, sp.node}}
			if sp.kind == specAlternatives {
				refs = refs[:0]
				for _, c := range sp.conds {
					refs = append(refs, struct {
						name string
						node *tree.Node
					}{c.master, c.node})
				}
			}
			for _, r := range refs {
				if _, ok := e.masters[r.name]; !ok {
					return tree.Errorf(r.node, "master-reference %q verweist auf keine fo:simple-page-master", r.name)
				}
			}
		}
	}
	return nil
}

// pageCond beschreibt die zu wählende Seite.
type pageCond struct {
	number int  // Seitenzahl (für gerade und ungerade)
	first  bool // erste Seite der Folge
	last   bool // letzte Seite der Folge
	blank  bool // Leerseite ohne Inhalt aus dem Fluss
}

// selector wählt die Seitenvorlage je Seite.
type selector struct {
	e      *engine
	simple *master
	sm     *seqMaster
	i      int    // aktuelle Teilfolge
	used   int    // in der aktuellen Teilfolge verbrauchte Seiten
	last   string // zuletzt verwendete Vorlage
}

func (e *engine) newSelector(ps *tree.Node, ref string) (*selector, error) {
	if m, ok := e.masters[ref]; ok {
		return &selector{e: e, simple: m}, nil
	}
	if sm, ok := e.seqMasters[ref]; ok {
		return &selector{e: e, sm: sm}, nil
	}
	return nil, tree.Errorf(ps, "Seitenvorlage %q nicht gefunden", ref)
}

// pick wählt die Vorlage für eine Seite. Mit commit wird die Seite
// verbraucht, sonst nur vorausgeschaut.
func (s *selector) pick(c pageCond, commit bool) (*master, error) {
	if s.simple != nil {
		return s.simple, nil
	}
	i, used := s.i, s.used
	for i < len(s.sm.specs) {
		sp := s.sm.specs[i]
		if sp.max >= 0 && used >= sp.max {
			i, used = i+1, 0
			continue
		}
		name := sp.master
		if sp.kind == specAlternatives {
			name = ""
			for _, cr := range sp.conds {
				if cr.matches(c) {
					name = cr.master
					break
				}
			}
			if name == "" {
				// Keine Alternative passt. XSL 1.1 6.4.5 erlaubt, die zuletzt
				// verwendete Vorlage weiter zu nutzen.
				if s.last == "" {
					return nil, tree.Errorf(sp.node, "keine fo:conditional-page-master-reference passt zu Seite %d", c.number)
				}
				name = s.last
			}
		}
		if commit {
			s.i, s.used, s.last = i, used+1, name
		}
		return s.e.masters[name], nil
	}
	// Erschöpfte Folge: ebenso die zuletzt verwendete Vorlage (6.4.5).
	if s.last != "" {
		return s.e.masters[s.last], nil
	}
	return nil, tree.Errorf(s.sm.node, "Seitenfolge erschöpft bei Seite %d, die Teilfolgen reichen nicht aus", c.number)
}

func (cr condRef) matches(c pageCond) bool {
	switch cr.position {
	case "first":
		if !c.first {
			return false
		}
	case "last":
		if !c.last {
			return false
		}
	case "rest":
		if c.first || c.last {
			return false
		}
	case "only":
		if !c.first || !c.last {
			return false
		}
	}
	switch cr.oddEven {
	case "odd":
		if c.number%2 == 0 {
			return false
		}
	case "even":
		if c.number%2 != 0 {
			return false
		}
	}
	switch cr.blank {
	case "blank":
		return c.blank
	case "not-blank":
		return !c.blank
	}
	return true
}

// formatNumber formatiert eine Seitenzahl nach dem Attribut format der
// fo:page-sequence (XSL 1.1 7.26.1): 1, 01, i, I, a, A.
func formatNumber(n int, format string) string {
	switch {
	case format == "" || format == "1":
		return strconv.Itoa(n)
	case format == "i":
		return strings.ToLower(roman(n))
	case format == "I":
		return roman(n)
	case format == "a":
		return alpha(n, 'a')
	case format == "A":
		return alpha(n, 'A')
	case strings.Trim(format, "0") == "1":
		return fmt.Sprintf("%0*d", len(format), n)
	}
	return strconv.Itoa(n)
}

func roman(n int) string {
	if n <= 0 || n >= 4000 {
		return strconv.Itoa(n)
	}
	vals := []int{1000, 900, 500, 400, 100, 90, 50, 40, 10, 9, 5, 4, 1}
	syms := []string{"M", "CM", "D", "CD", "C", "XC", "L", "XL", "X", "IX", "V", "IV", "I"}
	var b strings.Builder
	for i, v := range vals {
		for n >= v {
			b.WriteString(syms[i])
			n -= v
		}
	}
	return b.String()
}

func alpha(n int, base rune) string {
	if n <= 0 {
		return strconv.Itoa(n)
	}
	var r []rune
	for n > 0 {
		n--
		r = append([]rune{base + rune(n%26)}, r...)
		n /= 26
	}
	return string(r)
}
