// Package xpath supports the XPath editor of the designer: checking the
// paths against the data XSD, evaluation with sample data and
// completion. go-xslt checks the syntax when compiling, this package
// splits expressions only as far as needed for paths.
package xpath

import "strings"

// kind is the kind of a token.
type kind int

const (
	tName   kind = iota // name with or without prefix
	tString             // string in quotation marks
	tNumber
	tVar    // $name
	tSym    // / // @ . .. * [ ] ( ) , :: and operators
	tBroken // unterminated string
)

type token struct {
	kind kind
	text string
	pos  int // byte position in the expression
}

func (t token) is(s string) bool { return t.kind == tSym && t.text == s }

// lex splits an expression into tokens. Whitespace and comments are dropped.
func lex(s string) []token {
	var out []token
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '(' && i+1 < len(s) && s[i+1] == ':':
			// comment (: … :), nested
			depth, j := 1, i+2
			for j < len(s) && depth > 0 {
				switch {
				case strings.HasPrefix(s[j:], "(:"):
					depth++
					j += 2
				case strings.HasPrefix(s[j:], ":)"):
					depth--
					j += 2
				default:
					j++
				}
			}
			i = j
		case c == '"' || c == '\'':
			j := i + 1
			for {
				if j >= len(s) {
					out = append(out, token{tBroken, s[i:], i})
					return out
				}
				if s[j] == c {
					// a doubled quotation mark stands for itself
					if j+1 < len(s) && s[j+1] == c {
						j += 2
						continue
					}
					break
				}
				j++
			}
			out = append(out, token{tString, s[i : j+1], i})
			i = j + 1
		case c >= '0' && c <= '9' || c == '.' && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9':
			j := i
			for j < len(s) && (s[j] >= '0' && s[j] <= '9' || s[j] == '.' || s[j] == 'e' || s[j] == 'E') {
				j++
			}
			out = append(out, token{tNumber, s[i:j], i})
			i = j
		case c == '$':
			j := i + 1
			for j < len(s) && nameChar(s[j]) {
				j++
			}
			out = append(out, token{tVar, s[i:j], i})
			i = j
		case nameStart(c):
			j := i
			for j < len(s) && nameChar(s[j]) {
				j++
			}
			// prefix: name, colon, name without whitespace in between
			if j+1 < len(s) && s[j] == ':' && nameStart(s[j+1]) {
				j++
				for j < len(s) && nameChar(s[j]) {
					j++
				}
			}
			out = append(out, token{tName, s[i:j], i})
			i = j
		default:
			for _, op := range []string{"//", "..", "::", "!=", "<=", ">=", "||", "=>", "<<", ">>"} {
				if strings.HasPrefix(s[i:], op) {
					out = append(out, token{tSym, op, i})
					i += len(op)
					goto next
				}
			}
			out = append(out, token{tSym, s[i : i+1], i})
			i++
		next:
		}
	}
	return out
}

func nameStart(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' || c >= 0x80
}

func nameChar(c byte) bool {
	return nameStart(c) || c >= '0' && c <= '9' || c == '-' || c == '.'
}

// operators are names that count as operators after an operand.
var operators = map[string]bool{
	"and": true, "or": true, "div": true, "idiv": true, "mod": true, "eq": true, "ne": true,
	"lt": true, "le": true, "gt": true, "ge": true, "is": true, "to": true, "union": true,
	"intersect": true, "except": true, "instance": true, "treat": true, "castable": true,
	"cast": true, "then": true, "else": true, "return": true, "satisfies": true, "in": true,
	"as": true, "of": true,
}

// keywords start expressions when an operand is expected and no
// path follows.
var keywords = map[string]bool{"if": true, "for": true, "let": true, "some": true, "every": true}
