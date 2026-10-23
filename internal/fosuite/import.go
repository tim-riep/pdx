package fosuite

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/tim-riep/go-xslt/engine"
	"github.com/tim-riep/pdx/fo/tree"
)

// extractFO extracts fo:root from a FOP test case and substitutes variables:
// attribute values "##name" are replaced by /testcase/variables/name, as
// FOP's testcase2fo.xsl does.
const extractFO = `<?xml version="1.0" encoding="UTF-8"?>
<xsl:stylesheet version="3.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:output method="xml" encoding="UTF-8" indent="no"/>
  <xsl:template match="/testcase">
    <xsl:apply-templates select="fo/*" mode="copy"/>
  </xsl:template>
  <xsl:template match="node()" mode="copy">
    <xsl:copy>
      <xsl:apply-templates select="@*|node()" mode="copy"/>
    </xsl:copy>
  </xsl:template>
  <xsl:template match="@*[starts-with(., '##')]" mode="copy">
    <xsl:variable name="v" select="/testcase/variables/*[local-name() = substring(current(), 3)]"/>
    <xsl:attribute name="{name()}" namespace="{namespace-uri()}" select="if ($v) then string($v[1]) else string(.)"/>
  </xsl:template>
  <xsl:template match="@*" mode="copy">
    <xsl:copy/>
  </xsl:template>
</xsl:stylesheet>
`

// Reasons assigned by the import itself. On a repeated import, lines with
// these prefixes are rewritten, all others stay in place.
const (
	ReasonUnsupported = "nicht im Teilumfang von pdx: "
	ReasonFOPDisabled = "in FOP 2.11 deaktiviert: "
	ReasonResources   = "braucht Dateien aus dem Testverzeichnis: "
	ReasonFOPFails    = "FOP 2.11 setzt den Fall nicht: "
	ReasonNoText      = "keine vergleichbaren Zeilen, FOP prüft hier nur Flächen"
)

// importReason reports whether a reason comes from the import. Reasons from
// the reference (FOP does not typeset the case) are only rechecked by the
// reference.
func importReason(why string) bool {
	return autoReason(why) && !strings.HasPrefix(why, ReasonFOPFails)
}

// autoReason reports whether a reason comes from the import or the reference.
func autoReason(why string) bool {
	for _, p := range []string{ReasonUnsupported, ReasonFOPDisabled, ReasonResources, ReasonFOPFails, ReasonNoText} {
		if strings.HasPrefix(why, p) {
			return true
		}
	}
	return false
}

var (
	headerComment = regexp.MustCompile(`(?s)<!--.*?-->`)
	externalRef   = regexp.MustCompile(`(?:src|external-destination|background-image)="\s*(?:url\()?\s*['"]?([^'")]+)`)
)

// unsupportedProps are properties that the pdx layout does not implement
// yet, with their initial values and equivalent spellings. A case that sets a
// different value lies outside the supported subset.
var unsupportedProps = map[string][]string{
	"column-count":                 {"1"},
	"span":                         {"none"},
	"writing-mode":                 {"lr-tb", "lr"},
	"reference-orientation":        {"0"},
	"baseline-shift":               {"baseline", "0pt", "0"},
	"alignment-baseline":           {"auto"},
	"dominant-baseline":            {"auto"},
	"alignment-adjust":             {"auto"},
	"vertical-align":               {"baseline"},
	"letter-spacing":               {"normal", "0pt"},
	"word-spacing":                 {"normal", "0pt"},
	"white-space-collapse":         {"true"},
	"white-space-treatment":        {"ignore-if-surrounding-linefeed"},
	"white-space":                  {"normal"},
	"text-decoration":              {"none"},
	"text-transform":               {"none"},
	"font-variant":                 {"normal"},
	"relative-position":            {"static"},
	"visibility":                   {"visible"},
	"text-altitude":                {"use-font-metrics"},
	"text-depth":                   {"use-font-metrics"},
	"line-stacking-strategy":       {"max-height"},
	"line-height-shift-adjustment": {"consider-shifts"},
	"wrap-option":                  {"wrap"},
	"leader-pattern":               {"space", "rule", "dots"},
	"empty-cells":                  {"show"},
}

var attrRe = regexp.MustCompile(`\s([a-z][a-z-]*)((?:\.[a-z-]+)*)="([^"]*)"`)

var (
	foreignContent = regexp.MustCompile(`<fo:instream-foreign-object[^>]*>\s*<([A-Za-z_][\w.-]*(?::[\w.-]+)?)`)
	remoteSrc      = regexp.MustCompile(`src="\s*(?:url\()?\s*['"]?https?:`)
)

// nsFOX is the namespace of the FOP extensions.
const nsFOX = "http://xmlgraphics.apache.org/fop/extensions"

// unsupported returns the reason why a case lies outside the supported
// subset, or "".
func unsupported(fo string) string {
	if strings.Contains(fo, nsFOX) {
		return "nutzt FOP-Erweiterungen (fox)"
	}
	if m := foreignContent.FindStringSubmatch(fo); m != nil && m[1] != "pdx:barcode" {
		return "fo:instream-foreign-object mit " + m[1] + ", unterstützt ist nur pdx:barcode"
	}
	if remoteSrc.MatchString(fo) {
		return "Bild über das Netz, pdx lädt keine Daten nach"
	}
	for _, m := range attrRe.FindAllStringSubmatch(fo, -1) {
		name, sub, v := m[1], m[2], strings.TrimSpace(m[3])
		if strings.HasSuffix(sub, ".conditionality") && v == "retain" {
			return "Eigenschaft " + name + sub + "=\"retain\""
		}
		defs, ok := unsupportedProps[name]
		if !ok || sub != "" {
			continue
		}
		known := false
		for _, d := range defs {
			if v == d {
				known = true
			}
		}
		if !known {
			return "Eigenschaft " + name + "=\"" + v + "\""
		}
	}
	return ""
}

// ImportFOP reads the test cases from test/layoutengine of a FOP source tree.
// Cases in the supported subset go to testdata/suites/fop/cases, the others
// go to disabled with a reason.
func ImportFOP(layoutDir, root string, disabled map[string]string) (int, error) {
	fopDisabled, err := readFOPDisabled(filepath.Join(layoutDir, "disabled-testcases.xml"))
	if err != nil {
		return 0, err
	}
	files, err := filepath.Glob(filepath.Join(layoutDir, "standard-testcases", "*.xml"))
	if err != nil {
		return 0, err
	}
	for id, why := range disabled {
		if strings.HasPrefix(id, SuiteFOP+"/") && importReason(why) {
			delete(disabled, id)
		}
	}
	dst := caseDir(root, SuiteFOP)
	os.RemoveAll(dst)
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return 0, err
	}
	sheet, diags := engine.Compile(engine.Request{Stylesheet: extractFO})
	if sheet == nil {
		return 0, fmt.Errorf("Auszug-XSLT: %v", diags)
	}
	n := 0
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".xml")
		id := SuiteFOP + "/" + name
		if why, ok := fopDisabled[filepath.Base(f)]; ok {
			disabled[id] = ReasonFOPDisabled + oneLine(why)
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			return n, err
		}
		res := sheet.Transform(engine.TransformRequest{Source: string(src)})
		if res.HasErrors() || !strings.Contains(res.Output, "<fo:root") && !strings.Contains(res.Output, ":root") {
			disabled[id] = ReasonUnsupported + "Testfall ohne fo:root"
			continue
		}
		fo := res.Output
		if m := externalRef.FindStringSubmatch(fo); m != nil && !strings.HasPrefix(m[1], "data:") &&
			!strings.HasPrefix(m[1], "http") && !strings.HasPrefix(m[1], "#") {
			disabled[id] = ReasonResources + m[1]
			continue
		}
		if _, err := tree.Parse(strings.NewReader(fo)); err != nil {
			disabled[id] = ReasonUnsupported + oneLine(err.Error())
			continue
		}
		if why := unsupported(fo); why != "" {
			disabled[id] = ReasonUnsupported + why
			continue
		}
		// The license header of the test case is preserved (Apache License 2.0).
		var b bytes.Buffer
		b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
		if h := headerComment.Find(src); h != nil {
			b.Write(h)
			b.WriteString("\n")
		}
		b.WriteString("<!-- Aus Apache FOP 2.11, test/layoutengine/standard-testcases/" + filepath.Base(f) + " -->\n")
		b.WriteString(strings.TrimSpace(strings.TrimPrefix(fo, `<?xml version="1.0" encoding="UTF-8"?>`)))
		b.WriteString("\n")
		if err := os.WriteFile(filepath.Join(dst, name+".fo"), b.Bytes(), 0o644); err != nil {
			return n, err
		}
		n++
	}
	removeStaleExpect(root, SuiteFOP)
	return n, nil
}

// removeStaleExpect deletes expectations for cases that no longer exist.
func removeStaleExpect(root, suite string) {
	files, _ := filepath.Glob(filepath.Join(DataDir(root), suite, "expect", "*.txt"))
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".txt")
		if _, err := os.Stat(filepath.Join(caseDir(root, suite), name+".fo")); err != nil {
			os.Remove(f)
		}
	}
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 160 {
		s = s[:160] + "…"
	}
	return s
}

// readFOPDisabled reads FOP's list of disabled cases (file to description).
func readFOPDisabled(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Cases []struct {
			Name string `xml:"name"`
			File string `xml:"file"`
			Desc string `xml:"description"`
		} `xml:"testcase"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, c := range doc.Cases {
		why := c.Desc
		if why == "" {
			why = c.Name
		}
		out[c.File] = why
	}
	return out, nil
}

// sortedKeys returns the keys of a list in sorted order.
func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
