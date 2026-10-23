package dataschema

import (
	"regexp"
	"strings"
)

type stepKind uint8

const (
	stepChild stepKind = iota
	stepAttr
	stepSelf
	stepParent
)

type predKind uint8

const (
	predNone predKind = iota
	// predIndex selects a node, such as [1] or [last()].
	predIndex
	// predFilter filters, the binding can then be empty.
	predFilter
)

type step struct {
	kind stepKind
	name string
	pred predKind
}

func (s step) attr() bool { return s.kind == stepAttr }

var (
	qnameRe = regexp.MustCompile(`^[\p{L}_][\p{L}\p{N}_.\-]*(:[\p{L}_][\p{L}\p{N}_.\-]*)?$`)
	indexRe = regexp.MustCompile(`^\s*([0-9]+|last\(\))\s*$`)
)

// splitPath splits simple paths such as inv:items/inv:item[1]/@unit.
// Everything else (functions, operators, //, wildcards) yields ok=false.
func splitPath(expr string) (steps []step, abs bool, ok bool) {
	if expr == "" || strings.Contains(expr, "//") {
		return nil, false, false
	}
	if strings.HasPrefix(expr, "/") {
		abs = true
		expr = expr[1:]
		if expr == "" {
			return nil, false, false
		}
	}
	segs, ok := splitTop(expr)
	if !ok {
		return nil, false, false
	}
	for _, seg := range segs {
		st, ok := parseStep(seg)
		if !ok {
			return nil, false, false
		}
		steps = append(steps, st)
	}
	return steps, abs, true
}

// splitTop splits at slashes outside of predicates and literals.
func splitTop(s string) ([]string, bool) {
	var out []string
	depth, start := 0, 0
	var quote rune
	for i, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '\'' || r == '"':
			quote = r
		case r == '[':
			depth++
		case r == ']':
			depth--
			if depth < 0 {
				return nil, false
			}
		case r == '/' && depth == 0:
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if depth != 0 || quote != 0 {
		return nil, false
	}
	return append(out, s[start:]), true
}

func parseStep(seg string) (step, bool) {
	seg = strings.TrimSpace(seg)
	var st step
	name := seg
	if i := strings.IndexByte(seg, '['); i >= 0 {
		name = strings.TrimSpace(seg[:i])
		preds := seg[i:]
		for preds != "" {
			if preds[0] != '[' {
				return st, false
			}
			end := closing(preds)
			if end < 0 {
				return st, false
			}
			if indexRe.MatchString(preds[1:end]) {
				if st.pred == predNone {
					st.pred = predIndex
				}
			} else {
				st.pred = predFilter
			}
			preds = strings.TrimSpace(preds[end+1:])
		}
	}
	switch {
	case name == "." || name == "self::node()":
		st.kind = stepSelf
	case name == ".." || name == "parent::node()":
		st.kind = stepParent
	case strings.HasPrefix(name, "@"):
		st.kind, st.name = stepAttr, name[1:]
	case strings.HasPrefix(name, "attribute::"):
		st.kind, st.name = stepAttr, strings.TrimPrefix(name, "attribute::")
	case strings.HasPrefix(name, "child::"):
		st.kind, st.name = stepChild, strings.TrimPrefix(name, "child::")
	default:
		st.kind, st.name = stepChild, name
	}
	if st.kind == stepChild || st.kind == stepAttr {
		if !qnameRe.MatchString(st.name) {
			return st, false
		}
	}
	return st, true
}

// closing returns the index of the closing bracket for the first character.
func closing(s string) int {
	depth := 0
	var quote rune
	for i, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '\'' || r == '"':
			quote = r
		case r == '[':
			depth++
		case r == ']':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}
