package props

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Env stellt Funktionen bereit, die vom Kontext abhängen. Felder dürfen nil
// sein, dann ist die Funktion an dieser Stelle nicht erlaubt.
type Env struct {
	// BodyStart und LabelEnd liefern die Werte der gleichnamigen Funktionen
	// für Listen (XSL 1.1 5.10.4).
	BodyStart func() (Length, error)
	LabelEnd  func() (Length, error)
}

// value ist ein Zwischenergebnis: eine Zahl, eine Länge oder eine
// proportionale Spaltenbreite.
type value struct {
	kind valueKind
	num  float64 // Zahl, Länge in Millipunkt oder Anteil
}

type valueKind uint8

const (
	vNumber valueKind = iota
	vLength
	vProportional
)

// Proportional ist eine Spaltenbreite in Anteilen aus
// proportional-column-width().
type Proportional float64

// EvalLength wertet einen Längenausdruck nach XSL 1.1 5.9 aus: Längen,
// Prozent, Zahlen, + - * div mod, Klammern und die Funktionen abs, min, max,
// floor, ceiling, round, body-start, label-end. Ein Ergebnis aus
// proportional-column-width() liefert einen Fehler, dafür gibt es
// EvalColumnWidth.
func EvalLength(s string, fontSize, percentBase Length, env *Env) (Length, error) {
	v, err := eval(s, fontSize, percentBase, env)
	if err != nil {
		return 0, err
	}
	switch v.kind {
	case vLength:
		return Length(math.Round(v.num)), nil
	case vNumber:
		if v.num == 0 {
			return 0, nil
		}
		return 0, fmt.Errorf("Ausdruck %q liefert eine Zahl statt einer Länge", strings.TrimSpace(s))
	}
	return 0, fmt.Errorf("proportional-column-width ist nur an fo:table-column erlaubt")
}

// EvalColumnWidth wertet column-width aus. Das Ergebnis ist entweder eine
// feste Länge oder ein Anteil (prop > 0).
func EvalColumnWidth(s string, fontSize, tableWidth Length) (fixed Length, prop float64, err error) {
	v, err := eval(s, fontSize, tableWidth, nil)
	if err != nil {
		return 0, 0, err
	}
	switch v.kind {
	case vProportional:
		return 0, v.num, nil
	case vLength:
		return Length(math.Round(v.num)), 0, nil
	}
	return 0, 0, fmt.Errorf("column-width %q ist keine Länge", s)
}

func eval(s string, fontSize, percentBase Length, env *Env) (value, error) {
	p := &parser{s: s, fs: fontSize, pct: percentBase, env: env}
	p.next()
	v, err := p.additive()
	if err != nil {
		return value{}, err
	}
	if p.tok.kind != tEOF {
		return value{}, fmt.Errorf("unerwartetes %q in Ausdruck %q", p.tok.text, strings.TrimSpace(s))
	}
	return v, nil
}

type tokKind uint8

const (
	tEOF tokKind = iota
	tNum         // Zahl mit optionaler Einheit oder Prozent
	tName
	tOp
	tLParen
	tRParen
	tComma
)

type token struct {
	kind tokKind
	text string
}

type parser struct {
	s   string
	i   int
	tok token
	fs  Length
	pct Length
	env *Env
}

func (p *parser) next() {
	for p.i < len(p.s) && (p.s[p.i] == ' ' || p.s[p.i] == '\t' || p.s[p.i] == '\n' || p.s[p.i] == '\r') {
		p.i++
	}
	if p.i >= len(p.s) {
		p.tok = token{kind: tEOF}
		return
	}
	c := p.s[p.i]
	switch {
	case c == '(':
		p.i++
		p.tok = token{tLParen, "("}
	case c == ')':
		p.i++
		p.tok = token{tRParen, ")"}
	case c == ',':
		p.i++
		p.tok = token{tComma, ","}
	case c == '+' || c == '*' || c == '-':
		p.i++
		p.tok = token{tOp, string(c)}
	case c >= '0' && c <= '9' || c == '.':
		j := p.i
		for j < len(p.s) && (p.s[j] >= '0' && p.s[j] <= '9' || p.s[j] == '.') {
			j++
		}
		for j < len(p.s) && (p.s[j] >= 'a' && p.s[j] <= 'z' || p.s[j] == '%') {
			j++
		}
		p.tok = token{tNum, p.s[p.i:j]}
		p.i = j
	case c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
		j := p.i
		for j < len(p.s) && (p.s[j] >= 'a' && p.s[j] <= 'z' || p.s[j] >= 'A' && p.s[j] <= 'Z' || p.s[j] == '-' || p.s[j] >= '0' && p.s[j] <= '9') {
			j++
		}
		name := p.s[p.i:j]
		p.i = j
		if name == "div" || name == "mod" {
			p.tok = token{tOp, name}
		} else {
			p.tok = token{tName, name}
		}
	default:
		p.tok = token{tOp, string(c)}
		p.i++
	}
}

func (p *parser) additive() (value, error) {
	l, err := p.multiplicative()
	if err != nil {
		return l, err
	}
	for p.tok.kind == tOp && (p.tok.text == "+" || p.tok.text == "-") {
		op := p.tok.text
		p.next()
		r, err := p.multiplicative()
		if err != nil {
			return l, err
		}
		if l.kind != r.kind && !(l.kind == vNumber && l.num == 0) && !(r.kind == vNumber && r.num == 0) {
			return l, fmt.Errorf("Addition von Zahl und Länge in %q", strings.TrimSpace(p.s))
		}
		kind := l.kind
		if l.kind == vNumber && l.num == 0 {
			kind = r.kind
		}
		if op == "+" {
			l = value{kind, l.num + r.num}
		} else {
			l = value{kind, l.num - r.num}
		}
	}
	return l, nil
}

func (p *parser) multiplicative() (value, error) {
	l, err := p.unary()
	if err != nil {
		return l, err
	}
	for p.tok.kind == tOp && (p.tok.text == "*" || p.tok.text == "div" || p.tok.text == "mod") {
		op := p.tok.text
		p.next()
		r, err := p.unary()
		if err != nil {
			return l, err
		}
		switch op {
		case "*":
			switch {
			case l.kind == vNumber:
				l = value{r.kind, l.num * r.num}
			case r.kind == vNumber:
				l = value{l.kind, l.num * r.num}
			default:
				return l, fmt.Errorf("Produkt zweier Längen in %q", strings.TrimSpace(p.s))
			}
		case "div", "mod":
			if r.num == 0 {
				return l, fmt.Errorf("Division durch 0 in %q", strings.TrimSpace(p.s))
			}
			kind := l.kind
			if l.kind == r.kind {
				kind = vNumber
			} else if r.kind != vNumber {
				return l, fmt.Errorf("Zahl geteilt durch Länge in %q", strings.TrimSpace(p.s))
			}
			if op == "div" {
				l = value{kind, l.num / r.num}
			} else {
				l = value{kind, math.Mod(l.num, r.num)}
			}
		}
	}
	return l, nil
}

func (p *parser) unary() (value, error) {
	if p.tok.kind == tOp && p.tok.text == "-" {
		p.next()
		v, err := p.unary()
		v.num = -v.num
		return v, err
	}
	if p.tok.kind == tOp && p.tok.text == "+" {
		p.next()
	}
	return p.primary()
}

func (p *parser) primary() (value, error) {
	switch p.tok.kind {
	case tNum:
		text := p.tok.text
		p.next()
		if strings.HasSuffix(text, "%") || strings.IndexFunc(text, func(r rune) bool { return r >= 'a' && r <= 'z' }) >= 0 {
			l, err := ParseLength(text, p.fs, p.pct)
			return value{vLength, float64(l)}, err
		}
		f, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return value{}, fmt.Errorf("ungültige Zahl %q", text)
		}
		return value{vNumber, f}, nil
	case tLParen:
		p.next()
		v, err := p.additive()
		if err != nil {
			return v, err
		}
		if p.tok.kind != tRParen {
			return v, fmt.Errorf("schließende Klammer fehlt in %q", strings.TrimSpace(p.s))
		}
		p.next()
		return v, nil
	case tName:
		name := p.tok.text
		p.next()
		if p.tok.kind != tLParen {
			return value{}, fmt.Errorf("unbekannter Wert %q", name)
		}
		p.next()
		var args []value
		for p.tok.kind != tRParen {
			a, err := p.additive()
			if err != nil {
				return a, err
			}
			args = append(args, a)
			if p.tok.kind == tComma {
				p.next()
				continue
			}
			if p.tok.kind != tRParen {
				return a, fmt.Errorf("Argumentliste von %s nicht geschlossen", name)
			}
		}
		p.next()
		return p.call(name, args)
	}
	return value{}, fmt.Errorf("Ausdruck %q ist unvollständig", strings.TrimSpace(p.s))
}

func (p *parser) call(name string, args []value) (value, error) {
	need := func(n int) error {
		if len(args) != n {
			return fmt.Errorf("%s erwartet %d Argumente", name, n)
		}
		return nil
	}
	switch name {
	case "body-start", "label-end":
		if err := need(0); err != nil {
			return value{}, err
		}
		var f func() (Length, error)
		if p.env != nil {
			f = p.env.BodyStart
			if name == "label-end" {
				f = p.env.LabelEnd
			}
		}
		if f == nil {
			return value{}, fmt.Errorf("%s() ist nur innerhalb von fo:list-item erlaubt", name)
		}
		l, err := f()
		return value{vLength, float64(l)}, err
	case "proportional-column-width":
		if err := need(1); err != nil {
			return value{}, err
		}
		if args[0].kind != vNumber || args[0].num <= 0 {
			return value{}, fmt.Errorf("proportional-column-width erwartet eine positive Zahl")
		}
		return value{vProportional, args[0].num}, nil
	case "abs", "floor", "ceiling", "round":
		if err := need(1); err != nil {
			return value{}, err
		}
		v := args[0]
		switch name {
		case "abs":
			v.num = math.Abs(v.num)
		case "floor":
			v.num = math.Floor(v.num)
		case "ceiling":
			v.num = math.Ceil(v.num)
		case "round":
			v.num = math.Round(v.num)
		}
		return v, nil
	case "min", "max":
		if err := need(2); err != nil {
			return value{}, err
		}
		if args[0].kind != args[1].kind {
			return value{}, fmt.Errorf("%s mit verschiedenen Arten", name)
		}
		if (name == "min") == (args[0].num < args[1].num) {
			return args[0], nil
		}
		return args[1], nil
	}
	return value{}, fmt.Errorf("Funktion %s() wird nicht unterstützt", name)
}
