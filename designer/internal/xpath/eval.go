package xpath

import (
	"bytes"
	"encoding/xml"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/tim-riep/go-xslt/engine"
)

// Result ist das Ergebnis eines Ausdrucks mit Beispieldaten im ersten
// Vorkommen des Kontexts.
type Result struct {
	// Error ist der Fehler beim Übersetzen oder Auswerten, sonst leer.
	Error string `json:"error"`
	// Count ist die Zahl der Einträge der Ergebnisfolge.
	Count int `json:"count"`
	// Kind ist nodes, boolean, number, string oder empty.
	Kind string `json:"kind"`
	// Values sind die ersten Werte als Text.
	Values []string `json:"values"`
	// Truth ist der Wahrheitswert, wenn er sich bilden lässt (true, false
	// oder leer).
	Truth string `json:"truth"`
	// NoContext meldet, dass der Kontext in den Daten fehlt.
	NoContext bool `json:"noContext"`
}

// maxValues begrenzt die gezeigten Werte.
const maxValues = 5

// Eval wertet expr mit den Daten im ersten Knoten des Kontexts aus. ns
// ordnet Präfixe Namespaces zu.
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
	// Der Ausdruck steht allein in einer Zeile, damit Meldungen passen.
	b.WriteString("\n<xsl:variable name=\"r\" select=\"" + attr(expr) + "\"/>\n")
	b.WriteString(`<xsl:value-of select="count($r)"/><xsl:text>&#10;</xsl:text>` +
		`<xsl:value-of select="if (empty($r)) then 'empty' else if ($r instance of node()*) then 'nodes' else if ($r instance of xs:boolean*) then 'boolean' else if ($r instance of xs:numeric*) then 'number' else 'string'"/><xsl:text>&#10;</xsl:text>` +
		`<xsl:value-of select="if (empty($r) or ($r instance of node()*) or count($r) eq 1) then string(boolean($r)) else ''"/><xsl:text>&#10;</xsl:text>` +
		`<xsl:for-each select="subsequence($r, 1, ` + strconv.Itoa(maxValues) + `)"><xsl:value-of select="translate(string(.), '&#10;&#13;', '  ')"/><xsl:text>&#10;</xsl:text></xsl:for-each>`)
	b.WriteString(`</xsl:for-each></xsl:template></xsl:stylesheet>`)

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

// message kürzt die Meldung von go-xslt auf den Teil zum Ausdruck und
// übersetzt die häufigen Fälle.
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
	// EscapeText ersetzt Anführungszeichen, Zeilenumbrüche bleiben als
	// Zeichenreferenz erhalten.
	return b.String()
}
