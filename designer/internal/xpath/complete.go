package xpath

import (
	"sort"
	"strings"

	"github.com/tim-riep/pdx/designer/internal/schema"
)

// Item is a suggestion of the completion.
type Item struct {
	Label string `json:"label"`
	// Insert replaces the range From to To of the completion.
	Insert string `json:"insert"`
	// Kind is element, attribute, function or step.
	Kind string `json:"kind"`
	// Type is the data type of a node or the signature of a function.
	Type string `json:"type"`
	// Repeated marks repeated nodes.
	Repeated bool   `json:"repeated,omitempty"`
	Optional bool   `json:"optional,omitempty"`
	Doc      string `json:"doc,omitempty"`
}

// Completion is the suggestions for the position cursor.
type Completion struct {
	From  int    `json:"from"`
	To    int    `json:"to"`
	Items []Item `json:"items"`
	// Context is the node whose children are suggested.
	Context string `json:"context"`
}

// Complete returns suggestions for the position cursor (byte position) in expr.
func Complete(sc *schema.Schema, ctx, expr string, cursor int) Completion {
	cursor = max(0, min(cursor, len(expr)))
	// The word before the cursor: names, prefixes, @ and path steps.
	start := cursor
	for start > 0 && (nameChar(expr[start-1]) || expr[start-1] == ':' || expr[start-1] == '@' || expr[start-1] == '/') {
		start--
	}
	word := expr[start:cursor]
	if inString(expr[:start]) {
		return Completion{From: cursor, To: cursor, Items: []Item{}}
	}
	// In a predicate the step before it is the context.
	base := ctx
	if p := openPredicate(expr[:start]); p >= 0 {
		if pctx := pathBefore(sc, ctx, expr, p); pctx != "" {
			base = pctx
		}
	}
	// Complete the prefix after the last slash, resolve what comes before.
	from := start
	partial := word
	dir := ""
	if i := strings.LastIndex(word, "/"); i >= 0 {
		dir, partial = word[:i], word[i+1:]
		from = start + i + 1
	}
	// Replace up to the end of the word after the cursor.
	to := cursor
	for to < len(expr) && (nameChar(expr[to]) || expr[to] == ':') {
		to++
	}
	out := Completion{From: from, To: to, Items: []Item{}}
	node := base
	if strings.HasPrefix(word, "/") {
		node = ""
		dir = strings.TrimPrefix(dir, "/")
	}
	if dir != "" {
		node = resolve(sc, node, dir)
		if node == unknown {
			return out
		}
	}
	out.Context = node
	low := strings.ToLower(strings.TrimPrefix(partial, "@"))
	matches := func(name string) bool {
		name = strings.ToLower(strings.TrimPrefix(name, "@"))
		local := name
		if i := strings.Index(name, ":"); i >= 0 {
			local = name[i+1:]
		}
		return strings.HasPrefix(name, low) || strings.HasPrefix(local, low)
	}
	if sc != nil {
		var kids []*schema.Node
		if node == "" {
			kids = sc.Roots
		} else if n := sc.Find(node); n != nil {
			kids = n.Children
		}
		for _, k := range kids {
			if strings.HasPrefix(partial, "@") && !k.Attr {
				continue
			}
			if !matches(k.Name) {
				continue
			}
			kind := "element"
			if k.Attr {
				kind = "attribute"
			}
			out.Items = append(out.Items, Item{Label: k.Name, Insert: k.Name, Kind: kind, Type: k.Type,
				Repeated: k.Repeated(), Optional: k.Min == 0, Doc: k.Doc})
		}
	}
	if dir == "" && !strings.HasPrefix(word, "/") && !strings.HasPrefix(partial, "@") {
		if node != "" && strings.Contains(node, "/") && strings.HasPrefix("..", partial) {
			out.Items = append(out.Items, Item{Label: "..", Insert: "..", Kind: "step", Doc: "Elternknoten"})
		}
		// Functions only at the start of a path, after the data nodes.
		for _, f := range functions {
			if strings.HasPrefix(f.Label, strings.ToLower(partial)) {
				out.Items = append(out.Items, f)
			}
		}
	}
	return out
}

// resolve follows a relative path of simple steps.
func resolve(sc *schema.Schema, cur, rel string) string {
	if sc == nil {
		return unknown
	}
	for _, s := range strings.Split(rel, "/") {
		switch s {
		case "", ".":
		case "..":
			i := strings.LastIndex(cur, "/")
			if i < 0 {
				return unknown
			}
			cur = cur[:i]
		default:
			next := s
			if cur != "" {
				next = cur + "/" + s
			}
			if sc.Find(next) == nil {
				return unknown
			}
			cur = next
		}
	}
	return cur
}

// inString reports whether s ends in an open string.
func inString(s string) bool {
	toks := lex(s)
	return len(toks) > 0 && toks[len(toks)-1].kind == tBroken
}

// openPredicate returns the position of the innermost open square bracket.
func openPredicate(s string) int {
	var stack []int
	for _, t := range lex(s) {
		switch {
		case t.is("["), t.is("("):
			stack = append(stack, t.pos)
		case t.is("]"), t.is(")"):
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	for i := len(stack) - 1; i >= 0; i-- {
		if s[stack[i]] == '[' {
			return stack[i]
		}
		return -1 // innermost bracket is round: function argument
	}
	return -1
}

// pathBefore resolves the path immediately before the bracket at pos.
func pathBefore(sc *schema.Schema, ctx, expr string, pos int) string {
	start := pos
	for start > 0 && (nameChar(expr[start-1]) || expr[start-1] == ':' || expr[start-1] == '@' || expr[start-1] == '/' || expr[start-1] == '.') {
		start--
	}
	rel := expr[start:pos]
	cur := ctx
	if outer := openPredicate(expr[:start]); outer >= 0 {
		if c := pathBefore(sc, ctx, expr, outer); c != "" {
			cur = c
		}
	}
	if strings.HasPrefix(rel, "/") {
		cur, rel = "", strings.TrimPrefix(rel, "/")
	}
	if r := resolve(sc, cur, rel); r != unknown {
		return r
	}
	return ""
}

// functions are common functions with a short description.
var functions = func() []Item {
	list := []struct{ name, sig, doc string }{
		{"abs", "abs(zahl)", "Betrag einer Zahl"},
		{"avg", "avg(folge)", "Mittelwert"},
		{"boolean", "boolean(wert)", "Wahrheitswert"},
		{"ceiling", "ceiling(zahl)", "Aufrunden auf eine ganze Zahl"},
		{"concat", "concat(a, b, …)", "Texte verbinden"},
		{"contains", "contains(text, teil)", "Enthält der Text den Teil"},
		{"count", "count(folge)", "Anzahl der Einträge"},
		{"current-date", "current-date()", "Heutiges Datum"},
		{"empty", "empty(folge)", "Wahr, wenn nichts da ist"},
		{"ends-with", "ends-with(text, ende)", "Endet der Text so"},
		{"exists", "exists(folge)", "Wahr, wenn etwas da ist"},
		{"false", "false()", "Falsch"},
		{"floor", "floor(zahl)", "Abrunden auf eine ganze Zahl"},
		{"format-date", "format-date(datum, bild)", "Datum formatieren, etwa '[D01].[M01].[Y0001]'"},
		{"format-number", "format-number(zahl, bild)", "Zahl formatieren, etwa '#,##0.00'"},
		{"last", "last()", "Zahl der Einträge im Kontext"},
		{"lower-case", "lower-case(text)", "Kleinbuchstaben"},
		{"max", "max(folge)", "Größter Wert"},
		{"min", "min(folge)", "Kleinster Wert"},
		{"normalize-space", "normalize-space(text)", "Leerraum zusammenfassen, leer gilt als falsch"},
		{"not", "not(wert)", "Verneinung"},
		{"number", "number(wert)", "In eine Zahl umwandeln"},
		{"position", "position()", "Stelle im Kontext"},
		{"replace", "replace(text, muster, ersatz)", "Ersetzen mit regulärem Ausdruck"},
		{"round", "round(zahl, stellen)", "Runden"},
		{"starts-with", "starts-with(text, anfang)", "Beginnt der Text so"},
		{"string", "string(wert)", "In Text umwandeln"},
		{"string-join", "string-join(folge, trenner)", "Texte mit Trenner verbinden"},
		{"string-length", "string-length(text)", "Zahl der Zeichen"},
		{"substring", "substring(text, start, länge)", "Teil eines Textes"},
		{"substring-after", "substring-after(text, trenner)", "Text nach dem Trenner"},
		{"substring-before", "substring-before(text, trenner)", "Text vor dem Trenner"},
		{"sum", "sum(folge)", "Summe"},
		{"true", "true()", "Wahr"},
		{"upper-case", "upper-case(text)", "Großbuchstaben"},
	}
	out := make([]Item, 0, len(list))
	for _, f := range list {
		out = append(out, Item{Label: f.name, Insert: f.name + "(", Kind: "function", Type: f.sig, Doc: f.doc})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	return out
}()
