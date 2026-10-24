package xpath

import (
	"bytes"
	"encoding/xml"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/tim-riep/go-xslt/engine"
	"github.com/tim-riep/pdx/internal/xmlsafe"
)

// Result is the result of an expression with sample data in the first
// occurrence of the context.
type Result struct {
	// Error is the error during compiling or evaluating, otherwise empty.
	Error string `json:"error"`
	// Count is the number of items of the result sequence.
	Count int `json:"count"`
	// Kind is nodes, boolean, number, string or empty.
	Kind string `json:"kind"`
	// Values are the first values as text.
	Values []string `json:"values"`
	// Truth is the truth value, if it can be formed (true, false
	// or empty).
	Truth string `json:"truth"`
	// NoContext reports that the context is missing in the data.
	NoContext bool `json:"noContext"`
}

// maxValues limits the values shown.
const maxValues = 5

// Eval evaluates expr with the data in the first node of the context. ns
// maps prefixes to namespaces.
func Eval(expr, ctx string, ns map[string]string, data []byte) Result {
	var b bytes.Buffer
	b.WriteString(`<xsl:stylesheet version="3.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform" xmlns:xs="http://www.w3.org/2001/XMLSchema"`)
	prefixes := make([]string, 0, len(ns))
	for p := range ns {
		prefixes = append(prefixes, p)
	}
	sort.Strings(prefixes)
	for _, p := range prefixes {
		if p == "xs" || p == "xsl" || p == "" {
			continue
		}
		b.WriteString(" xmlns:" + p + `="` + attr(ns[p]) + `"`)
	}
	b.WriteString(` exclude-result-prefixes="#all"><xsl:output method="text"/>`)
	b.WriteString(`<xsl:template match="/"><xsl:variable name="c" select="(/` + attr(ctx) + `)[1]"/>`)
	b.WriteString(`<xsl:if test="empty($c)">-&#10;</xsl:if><xsl:for-each select="$c">`)
	// The expression stands alone on a line, so that messages match.
	b.WriteString("\n<xsl:variable name=\"r\" select=\"" + attr(expr) + "\"/>\n")
	b.WriteString(`<xsl:value-of select="count($r)"/><xsl:text>&#10;</xsl:text>` +
		`<xsl:value-of select="if (empty($r)) then 'empty' else if ($r instance of node()*) then 'nodes' else if ($r instance of xs:boolean*) then 'boolean' else if ($r instance of xs:numeric*) then 'number' else 'string'"/><xsl:text>&#10;</xsl:text>` +
		`<xsl:value-of select="if (empty($r) or ($r instance of node()*) or count($r) eq 1) then string(boolean($r)) else ''"/><xsl:text>&#10;</xsl:text>` +
		`<xsl:for-each select="subsequence($r, 1, ` + strconv.Itoa(maxValues) + `)"><xsl:value-of select="translate(string(.), '&#10;&#13;', '  ')"/><xsl:text>&#10;</xsl:text></xsl:for-each>`)
	b.WriteString(`</xsl:for-each></xsl:template></xsl:stylesheet>`)

	if err := xmlsafe.Check(data); err != nil {
		return Result{Error: err.Error()}
	}
	res := engine.Transform(engine.Request{Stylesheet: b.String(), Source: string(data)})
	for _, d := range res.Diagnostics {
		if d.Severity == engine.SeverityError {
			return Result{Error: message(d)}
		}
	}
	lines := strings.Split(res.Output, "\n")
	if len(lines) > 0 && lines[0] == "-" {
		return Result{NoContext: true}
	}
	if len(lines) < 3 {
		return Result{Error: "keine Auswertung möglich"}
	}
	n, _ := strconv.Atoi(lines[0])
	r := Result{Count: n, Kind: lines[1], Truth: lines[2], Values: []string{}}
	for _, v := range lines[3:] {
		if len(r.Values) < min(n, maxValues) {
			r.Values = append(r.Values, v)
		}
	}
	return r
}

// message shortens the message of go-xslt to the part about the expression and
// translates the common cases.
func message(d engine.Diagnostic) string {
	m := strings.TrimSpace(d.Message)
	m = strings.TrimPrefix(m, "err:")
	if i := strings.LastIndex(m, `": `); strings.HasPrefix(m, "bad select") && i >= 0 {
		m = m[i+3:]
	}
	for _, t := range translations {
		if g := t.re.FindStringSubmatch(m); g != nil {
			out := t.de
			for i, v := range g[1:] {
				out = strings.ReplaceAll(out, "$"+strconv.Itoa(i+1), v)
			}
			return out
		}
	}
	if d.Code != "" && !strings.Contains(m, d.Code) {
		m = d.Code + ": " + m
	}
	return "XPath-Fehler " + m
}

var translations = []struct {
	re *regexp.Regexp
	de string
}{
	{regexp.MustCompile(`expected [a-z ]+(?: \d+)?, got ""$`), "Der Ausdruck ist unvollständig"},
	{regexp.MustCompile(`unexpected trailing token "(.*)"$`), "Unerwartet: „$1“"},
	{regexp.MustCompile(`expected [a-z ]+(?: \d+)?, got "(.*)"$`), "Unerwartet: „$1“"},
	{regexp.MustCompile(`unterminated string literal`), "Die Zeichenkette ist nicht geschlossen"},
	{regexp.MustCompile(`prefix "(.*)" has no namespace binding`), "Der Präfix $1 ist in der Vorlage nicht deklariert"},
	{regexp.MustCompile(`(\S+)\(\) does not accept (\d+) arguments?`), "$1() nimmt keine $2 Argumente"},
	{regexp.MustCompile(`unknown function (\S+\(\))`), "Die Funktion $1 gibt es nicht"},
	{regexp.MustCompile(`arithmetic operand is a sequence of (\d+) items`), "Gerechnet wird mit einzelnen Werten, hier sind es $1"},
}

func attr(s string) string {
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(s))
	// EscapeText replaces quotation marks, line breaks are kept as
	// character references.
	return b.String()
}
