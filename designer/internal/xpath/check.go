package xpath

import (
	"fmt"
	"strings"

	"github.com/tim-riep/pdx/designer/internal/schema"
)

// Problem ist ein Befund zu einem Teil des Ausdrucks.
type Problem struct {
	Pos     int    `json:"pos"` // Byte-Bereich im Ausdruck
	End     int    `json:"end"`
	Message string `json:"message"`
}

// unknown steht für einen Kontext, den die Prüfung nicht kennt, etwa nach
// einer Variablen. Pfade darin werden nicht geprüft.
const unknown = "\x00"

// Paths prüft die Pfade eines Ausdrucks gegen den Datenbaum. ctx ist der
// Datenkontext als Pfad im Baum. target ist der Knoten, auf den der ganze
// Ausdruck zeigt, wenn er ein einfacher Pfad ist.
func Paths(sc *schema.Schema, ctx, expr string) (problems []Problem, target string) {
	if sc == nil {
		return nil, ""
	}
	c := &checker{sc: sc, toks: lex(expr)}
	c.expr(0, len(c.toks), ctx)
	if c.single != "" && len(c.toks) > 0 && c.singleEnd == len(c.toks) {
		target = c.single
	}
	return c.out, target
}

type checker struct {
	sc   *schema.Schema
	toks []token
	out  []Problem
	// single ist das Ziel eines Pfads, der am ersten Token beginnt,
	// singleEnd das Token danach.
	single    string
	singleEnd int
}

func (c *checker) at(i int) token {
	if i < len(c.toks) {
		return c.toks[i]
	}
	return token{kind: tSym, text: "", pos: -1}
}

// match liefert das Token mit der schließenden Klammer zu i, sonst end.
func (c *checker) match(i, end int) int {
	open, close := c.toks[i].text, map[string]string{"(": ")", "[": "]", "{": "}"}[c.toks[i].text]
	depth := 0
	for j := i; j < end; j++ {
		switch {
		case c.toks[j].is(open):
			depth++
		case c.toks[j].is(close):
			depth--
			if depth == 0 {
				return j
			}
		}
	}
	return end
}

// expr durchläuft die Tokens [i, end) im Kontext ctx.
func (c *checker) expr(i, end int, ctx string) {
	operand := true
	skip := false // der nächste Pfad bezieht sich auf einen unbekannten Kontext
	for i < end {
		t := c.toks[i]
		next := c.at(i + 1)
		switch {
		case t.kind == tName && next.is("(") && !next.is("::"):
			// Funktionsaufruf, Argumente im selben Kontext
			j := c.match(i+1, end)
			c.expr(i+2, j, ctx)
			i = c.steps(j+1, end)
			operand = false
		case t.kind == tName && operand && keywords[t.text] && (next.kind == tVar || next.is("(")):
			i++
		case t.kind == tName && !operand && operators[t.text]:
			i++
			operand = true
			// cast as, castable as, treat as, instance of
			if (t.text == "cast" || t.text == "castable" || t.text == "treat" || t.text == "instance") && c.at(i).kind == tName && (c.at(i).text == "as" || c.at(i).text == "of") {
				i = c.typeName(i+1, end)
				operand = false
			} else if t.text == "as" || t.text == "of" {
				i = c.typeName(i, end)
				operand = false
			}
		case operand && c.startsPath(t):
			cx := ctx
			if skip {
				cx = unknown
			}
			first := i == 0
			j, target := c.path(i, end, cx)
			if first {
				c.single, c.singleEnd = target, j
			}
			i, operand, skip = j, false, false
		case t.is("(") || t.is("[") || t.is("{"):
			j := c.match(i, end)
			cx := ctx
			if t.is("[") {
				cx = unknown // Prädikat hinter einem Ausdruck ohne Pfad
			}
			c.expr(i+1, j, cx)
			i = c.steps(j+1, end)
			operand = false
		case t.kind == tVar:
			i = c.steps(i+1, end)
			operand = false
		case t.kind == tString || t.kind == tNumber || t.kind == tBroken:
			i++
			operand = false
		case t.is("!"):
			// einfache Abbildung: rechts gilt jedes Element links als Kontext
			i++
			operand, skip = true, true
		default:
			i++
			operand = true
		}
	}
}

// typeName überspringt einen Typnamen nach as oder instance of.
func (c *checker) typeName(i, end int) int {
	if i < end && c.toks[i].kind == tName {
		i++
		if i < end && c.toks[i].is("(") {
			i = c.match(i, end) + 1
		}
		if i < end && (c.toks[i].is("?") || c.toks[i].is("*") || c.toks[i].is("+")) {
			i++
		}
	}
	return i
}

func (c *checker) startsPath(t token) bool {
	return t.kind == tName || t.is("@") || t.is(".") || t.is("..") || t.is("/") || t.is("//") || t.is("*")
}

// steps überspringt Pfadschritte hinter einem Ausdruck ohne bekannten Knoten,
// etwa $x/inv:name. Prädikate darin werden ebenfalls nicht geprüft.
func (c *checker) steps(i, end int) int {
	for i < end && (c.toks[i].is("/") || c.toks[i].is("//") || c.toks[i].is("[")) {
		if c.toks[i].is("[") {
			j := c.match(i, end)
			c.expr(i+1, j, unknown)
			i = j + 1
			continue
		}
		j, _ := c.path(i+1, end, unknown)
		i = j
	}
	return i
}

// path prüft einen Pfad ab Token i und liefert das Token danach sowie das
// Ziel, wenn es bekannt ist.
func (c *checker) path(i, end int, ctx string) (int, string) {
	cur, known := ctx, ctx != unknown
	if i < end && c.toks[i].is("/") {
		cur = ""
		i++
		if i >= end || !c.startsPath(c.toks[i]) || c.toks[i].is("/") || c.toks[i].is("//") {
			return i, ""
		}
	} else if i < end && c.toks[i].is("//") {
		known = false
		i++
	}
	for i < end {
		t := c.toks[i]
		step, pos, stepEnd := "", t.pos, t.pos+len(t.text)
		switch {
		case t.kind == tName && c.at(i+1).is("::"):
			// Achsen prüft der Designer nicht
			known = false
			i += 2
			continue
		case t.is("@") && c.at(i+1).kind == tName:
			step = "@" + c.toks[i+1].text
			stepEnd = c.toks[i+1].pos + len(c.toks[i+1].text)
			i += 2
		case t.is("@"):
			known = false
			i++
			if i < end && c.toks[i].is("*") {
				i++
			}
		case t.kind == tName && c.at(i+1).is("("):
			// Knotentest oder Funktion als Schritt, etwa text() oder string()
			j := c.match(i+1, end)
			c.expr(i+2, j, unknown)
			known = false
			i = j + 1
		case t.kind == tName:
			step = t.text
			i++
		case t.is("."):
			i++
		case t.is(".."):
			if known {
				if cur == "" || !strings.Contains(cur, "/") {
					c.out = append(c.out, Problem{pos, stepEnd, "Der Pfad verlässt die Datenwurzel"})
					known = false
				} else {
					cur = cur[:strings.LastIndex(cur, "/")]
				}
			}
			i++
		case t.is("*"):
			known = false
			i++
		default:
			return i, ""
		}
		if step != "" && known {
			next := step
			if cur != "" {
				next = cur + "/" + step
			}
			if c.sc.Find(next) != nil {
				cur = next
			} else {
				if !c.open(cur) {
					where := cur
					if where == "" {
						where = "der Datenwurzel"
					}
					c.out = append(c.out, Problem{pos, stepEnd, fmt.Sprintf("%s gibt es unter %s nicht", step, where)})
				}
				known = false
			}
		}
		for i < end && c.toks[i].is("[") {
			j := c.match(i, end)
			cx := cur
			if !known {
				cx = unknown
			}
			c.expr(i+1, j, cx)
			i = j + 1
		}
		if i < end && c.toks[i].is("/") {
			i++
			continue
		}
		if i < end && c.toks[i].is("//") {
			known = false
			i++
			continue
		}
		break
	}
	if !known {
		return i, ""
	}
	return i, cur
}

// open meldet einen Knoten, dessen Inhalt das Schema offen lässt.
func (c *checker) open(path string) bool {
	n := c.sc.Find(path)
	return n != nil && (n.Open || n.Recursive)
}
