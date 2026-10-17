// Package xpath unterstützt den XPath-Editor des Designers: Prüfung der
// Pfade gegen das Daten-XSD, Auswertung mit Beispieldaten und
// Vervollständigung. Die Syntax prüft go-xslt beim Übersetzen, dieses Paket
// zerlegt Ausdrücke nur so weit, wie es für Pfade nötig ist.
package xpath

import "strings"

// kind ist die Art eines Tokens.
type kind int

const (
	tName   kind = iota // Name mit oder ohne Präfix
	tString             // Zeichenkette in Anführungszeichen
	tNumber
	tVar    // $name
	tSym    // / // @ . .. * [ ] ( ) , :: und Operatoren
	tBroken // nicht geschlossene Zeichenkette
)

type token struct {
	kind kind
	text string
	pos  int // Byte-Position im Ausdruck
}

func (t token) is(s string) bool { return t.kind == tSym && t.text == s }

// lex zerlegt einen Ausdruck in Tokens. Leerraum und Kommentare entfallen.
func lex(s string) []token {
	var out []token
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '(' && i+1 < len(s) && s[i+1] == ':':
			// Kommentar (: … :), verschachtelt
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
					// verdoppeltes Anführungszeichen steht für sich selbst
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
			// Präfix: Name, Doppelpunkt, Name ohne Leerraum dazwischen
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

// operators sind Namen, die nach einem Operanden als Operator gelten.
var operators = map[string]bool{
	"and": true, "or": true, "div": true, "idiv": true, "mod": true, "eq": true, "ne": true,
	"lt": true, "le": true, "gt": true, "ge": true, "is": true, "to": true, "union": true,
	"intersect": true, "except": true, "instance": true, "treat": true, "castable": true,
	"cast": true, "then": true, "else": true, "return": true, "satisfies": true, "in": true,
	"as": true, "of": true,
}

// keywords leiten Ausdrücke ein, wenn ein Operand erwartet wird und kein
// Pfad folgt.
var keywords = map[string]bool{"if": true, "for": true, "let": true, "some": true, "every": true}
