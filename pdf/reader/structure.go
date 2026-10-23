package reader

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// Node is a structure element with the text of its marked content.
type Node struct {
	Type string
	// Std is the standard type according to RoleMap, if it differs.
	Std   string
	Lang  string
	Alt   string
	Scope string
	// ColSpan and RowSpan are the spans of a cell, 0 if not specified.
	ColSpan, RowSpan int
	// Text is the text of its own marked content in tree order.
	Text   string
	Annots []string
	Kids   []*Node
	// Items is the content in the order of /K: text, kid or
	// annotation.
	Items []Item
}

// Item is an entry in the content of an element.
type Item struct {
	Text  string
	Kid   *Node
	Annot string
}

// Structure is the logical structure tree of a file with the findings of the
// consistency check.
type Structure struct {
	Root      *Node
	Marked    bool
	Lang      string
	Artifacts int
	// Problems are contradictions between tree, ParentTree and content.
	Problems []string
}

type mcKey struct{ page, mcid int }

// ReadStructure reads the structure tree and checks it against the content of
// the pages. Without StructTreeRoot, Root is nil.
func (f *File) ReadStructure() (*Structure, error) {
	root := f.Root()
	st := &Structure{}
	if mi := f.Dict(root["MarkInfo"]); mi != nil {
		st.Marked, _ = f.Resolve(mi["Marked"]).(bool)
	}
	if s, ok := f.Resolve(root["Lang"]).(String); ok {
		st.Lang = textString(s)
	}
	pages, refs := f.Pages()
	pageOf := map[Ref]int{}
	for i, r := range refs {
		pageOf[r] = i
	}
	content := map[mcKey]*Marked{}
	used := map[mcKey]bool{}
	for pi, p := range pages {
		pc, err := f.Content(p)
		if err != nil {
			return nil, fmt.Errorf("Seite %d: %w", pi+1, err)
		}
		st.Artifacts += len(pc.ArtifactText)
		for i := range pc.Marked {
			m := &pc.Marked[i]
			k := mcKey{pi, m.MCID}
			if _, dup := content[k]; dup {
				st.problem("Seite %d: MCID %d ist doppelt", pi+1, m.MCID)
			}
			content[k] = m
		}
		// Untagged files may have unmarked content.
		if pc.Unmarked > 0 && (st.Marked || root["StructTreeRoot"] != nil) {
			st.problem("Seite %d: %d zeichnende Operatoren ohne Markierung", pi+1, pc.Unmarked)
		}
		if pc.Nested > 0 {
			st.problem("Seite %d: %d Artefakte in Inhalt oder Inhalte in Artefakten", pi+1, pc.Nested)
		}
		if len(pc.Marked) > 0 && p["StructParents"] == nil {
			st.problem("Seite %d: /StructParents fehlt", pi+1)
		}
	}
	str := f.Dict(root["StructTreeRoot"])
	if str == nil {
		if len(content) > 0 {
			st.problem("markierter Inhalt ohne StructTreeRoot")
		}
		return st, nil
	}
	roleMap := f.Dict(str["RoleMap"])
	parentTree := f.numberTree(str["ParentTree"])
	// Expected entries in the ParentTree: per page the elements per MCID.
	owner := map[mcKey]Ref{}
	seen := map[Ref]bool{}
	var walk func(o Object, pg int, depth int) *Node
	walk = func(o Object, pg int, depth int) *Node {
		r, _ := o.(Ref)
		if r.Num != 0 {
			if seen[r] {
				st.problem("Strukturelement %d kommt mehrfach vor", r.Num)
				return nil
			}
			seen[r] = true
		}
		d := f.Dict(o)
		if d == nil || depth > 256 {
			return nil
		}
		n := &Node{}
		if t, ok := f.Resolve(d["S"]).(Name); ok {
			n.Type = string(t)
			if std, ok := f.Resolve(roleMap[t]).(Name); ok && string(std) != n.Type {
				n.Std = string(std)
			}
		}
		if s, ok := f.Resolve(d["Lang"]).(String); ok {
			n.Lang = textString(s)
		}
		if s, ok := f.Resolve(d["Alt"]).(String); ok {
			n.Alt = textString(s)
		}
		for _, a := range attrs(f, d["A"]) {
			if s, ok := f.Resolve(a["Scope"]).(Name); ok {
				n.Scope = string(s)
			}
			if v, ok := f.Int(a["ColSpan"]); ok {
				n.ColSpan = v
			}
			if v, ok := f.Int(a["RowSpan"]); ok {
				n.RowSpan = v
			}
		}
		if p, ok := d["Pg"].(Ref); ok {
			pg = pageOf[p]
		}
		var kids Array
		switch k := f.Resolve(d["K"]).(type) {
		case Array:
			kids = k
		case nil:
		default:
			kids = Array{d["K"]}
		}
		var text []string
		for _, k := range kids {
			mc := func(page, mcid int) {
				key := mcKey{page, mcid}
				m, ok := content[key]
				if !ok {
					st.problem("%s verweist auf Seite %d MCID %d, die es nicht gibt", n.Type, page+1, mcid)
					return
				}
				if used[key] {
					st.problem("Seite %d MCID %d gehört zu mehreren Elementen", page+1, mcid)
				}
				used[key] = true
				owner[key] = r
				text = append(text, m.Text)
				if k := len(n.Items) - 1; k >= 0 && n.Items[k].Kid == nil && n.Items[k].Annot == "" {
					n.Items[k].Text = joinText([]string{n.Items[k].Text, m.Text})
				} else {
					n.Items = append(n.Items, Item{Text: m.Text})
				}
			}
			switch v := f.Resolve(k).(type) {
			case int64:
				mc(pg, int(v))
			case Dict:
				switch v["Type"] {
				case Name("MCR"):
					p := pg
					if pr, ok := v["Pg"].(Ref); ok {
						p = pageOf[pr]
					}
					if id, ok := f.Int(v["MCID"]); ok {
						mc(p, id)
					}
				case Name("OBJR"):
					a := f.Dict(v["Obj"])
					desc := "Annotation"
					if s, ok := f.Resolve(a["Subtype"]).(Name); ok {
						desc += " " + string(s)
					}
					if c, ok := f.Resolve(a["Contents"]).(String); ok {
						desc += fmt.Sprintf(" %q", textString(c))
					}
					if sp, ok := f.Int(a["StructParent"]); !ok {
						st.problem("%s ohne /StructParent", desc)
					} else if pr, _ := parentTree[sp].(Ref); pr != r {
						st.problem("ParentTree %d zeigt nicht auf das Element der %s", sp, desc)
					}
					n.Annots = append(n.Annots, desc)
					n.Items = append(n.Items, Item{Annot: desc})
				default:
					if c := walk(k, pg, depth+1); c != nil {
						n.Kids = append(n.Kids, c)
						n.Items = append(n.Items, Item{Kid: c})
					}
				}
			}
		}
		n.Text = joinText(text)
		return n
	}
	for _, k := range f.kidsOf(str["K"]) {
		if c := walk(k, -1, 0); c != nil {
			if st.Root == nil {
				st.Root = c
			} else {
				st.Root.Kids = append(st.Root.Kids, c)
				st.problem("StructTreeRoot hat mehr als ein Wurzelelement")
			}
		}
	}
	keys := make([]mcKey, 0, len(content))
	for k := range content {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].page != keys[j].page {
			return keys[i].page < keys[j].page
		}
		return keys[i].mcid < keys[j].mcid
	})
	for _, k := range keys {
		if !used[k] {
			st.problem("Seite %d MCID %d gehört zu keinem Element", k.page+1, k.mcid)
			continue
		}
		sp, ok := f.Int(pages[k.page]["StructParents"])
		if !ok {
			continue
		}
		arr := f.Array(parentTree[sp])
		if k.mcid >= len(arr) {
			st.problem("ParentTree der Seite %d hat keinen Eintrag für MCID %d", k.page+1, k.mcid)
		} else if r, _ := arr[k.mcid].(Ref); r != owner[k] {
			st.problem("ParentTree der Seite %d zeigt für MCID %d auf ein anderes Element", k.page+1, k.mcid)
		}
	}
	return st, nil
}

func (s *Structure) problem(format string, args ...any) {
	s.Problems = append(s.Problems, fmt.Sprintf(format, args...))
}

func (f *File) kidsOf(o Object) Array {
	switch v := f.Resolve(o).(type) {
	case Array:
		return v
	case nil:
		return nil
	}
	return Array{o}
}

func attrs(f *File, o Object) []Dict {
	var out []Dict
	switch v := f.Resolve(o).(type) {
	case Dict:
		out = append(out, v)
	case Array:
		for _, e := range v {
			if d := f.Dict(e); d != nil {
				out = append(out, d)
			}
		}
	}
	return out
}

// numberTree reads a number tree flat (ISO 32000-1 7.9.7).
func (f *File) numberTree(o Object) map[int]Object {
	out := map[int]Object{}
	var walk func(o Object, depth int)
	walk = func(o Object, depth int) {
		d := f.Dict(o)
		if d == nil || depth > 64 {
			return
		}
		nums := f.Array(d["Nums"])
		for i := 0; i+1 < len(nums); i += 2 {
			if k, ok := f.Int(nums[i]); ok {
				out[k] = nums[i+1]
			}
		}
		for _, k := range f.Array(d["Kids"]) {
			walk(k, depth+1)
		}
	}
	walk(o, 0)
	return out
}

// textString decodes a text string, see TextString.
func textString(s String) string { return TextString(s) }

// joinText joins text pieces, with spaces where there are none.
func joinText(parts []string) string {
	var b strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		// A soft hyphen at the end joins with the next piece.
		cur := b.String()
		if strings.HasSuffix(cur, "\u00ad") {
			b.Reset()
			b.WriteString(strings.TrimSuffix(cur, "\u00ad"))
		} else if b.Len() > 0 && !strings.HasSuffix(cur, " ") && !strings.HasPrefix(p, " ") {
			b.WriteByte(' ')
		}
		b.WriteString(p)
	}
	return strings.TrimSpace(strings.ReplaceAll(b.String(), "\u00ad", ""))
}

// Format writes the tree indented, one element per line.
func (s *Structure) Format(w io.Writer) error {
	var b strings.Builder
	if s.Root == nil {
		b.WriteString("kein Strukturbaum\n")
	} else {
		fmt.Fprintf(&b, "MarkInfo Marked %v, Sprache %q, %d Artefakttexte\n", s.Marked, s.Lang, s.Artifacts)
		var node func(n *Node, depth int)
		node = func(n *Node, depth int) {
			b.WriteString(strings.Repeat("  ", depth))
			b.WriteString(n.Type)
			if n.Std != "" {
				b.WriteString(" (" + n.Std + ")")
			}
			if n.Lang != "" {
				fmt.Fprintf(&b, " lang=%q", n.Lang)
			}
			if n.Scope != "" {
				b.WriteString(" scope=" + n.Scope)
			}
			if n.ColSpan > 1 {
				fmt.Fprintf(&b, " colspan=%d", n.ColSpan)
			}
			if n.RowSpan > 1 {
				fmt.Fprintf(&b, " rowspan=%d", n.RowSpan)
			}
			if n.Alt != "" {
				fmt.Fprintf(&b, " alt=%q", n.Alt)
			}
			// Only text goes on the line of the element, mixed content
			// below it in order.
			if len(n.Kids) == 0 && len(n.Annots) == 0 {
				if n.Text != "" {
					fmt.Fprintf(&b, " %q", n.Text)
				}
				b.WriteByte('\n')
				return
			}
			b.WriteByte('\n')
			ind := strings.Repeat("  ", depth+1)
			for _, it := range n.Items {
				switch {
				case it.Kid != nil:
					node(it.Kid, depth+1)
				case it.Annot != "":
					b.WriteString(ind + it.Annot + "\n")
				case strings.TrimSpace(it.Text) != "":
					fmt.Fprintf(&b, "%s%q\n", ind, strings.TrimSpace(it.Text))
				}
			}
		}
		node(s.Root, 0)
	}
	for _, p := range s.Problems {
		b.WriteString("Befund: " + p + "\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}
