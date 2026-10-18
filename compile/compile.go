// Package compile übersetzt Vorlagen (template.xml) in XSLT 3.0. Das
// Stylesheet erzeugt aus den Daten XSL-FO für die Layout-Engine.
//
// Die Ausgabe ist deterministisch und lesbar: feste Einrückung, Attribute in
// fester Reihenfolge, jedes erzeugte FO-Element trägt pdx:id mit der ID des
// Vorlagenelements. Eine Quellzuordnung (SourceMap) übersetzt Zeilen im
// Stylesheet zurück in Vorlagen-IDs, damit Fehler beim Transformieren dem
// Element im Designer zugeordnet werden können.
//
// Überträge und Zwischensummen laufen über Marker: Jede Datenzeile einer
// Tabelle trägt die Summe vor der Zeile (für carryOver im Tabellenkopf, über
// retrieve-position-within-table="first-starting") und die Summe einschließlich
// der Zeile (für subtotal im Tabellenfuß, über "last-ending"). Die laufende
// Summe entsteht mit xsl:iterate.
package compile

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/tim-riep/go-xslt/engine"
	"github.com/tim-riep/pdx/fo/tree"
	"github.com/tim-riep/pdx/template"
)

// Namespaces im erzeugten Stylesheet.
const (
	nsXSL = "http://www.w3.org/1999/XSL/Transform"
	nsFO  = "http://www.w3.org/1999/XSL/Format"
	nsXS  = "http://www.w3.org/2001/XMLSchema"
)

// SequenceID ist die FO-ID der Seitenfolge. {total} in pageNumber verweist
// mit fo:page-number-citation-last auf sie.
const SequenceID = "pdx-sequence"

// reserved sind Präfixe, die das Stylesheet selbst belegt.
var reserved = map[string]bool{"xsl": true, "fo": true, "pdx": true, "xs": true, "xml": true, "xmlns": true}

// Options steuert Compile.
type Options struct {
	// Lang wählt Textressourcen und Formatmuster, leer bedeutet template/@lang.
	Lang string
}

// Mapping ordnet einen Zeilenbereich im Stylesheet einer Vorlagen-ID zu.
type Mapping struct {
	Start, End int
	ID         string
}

// Attachment ist ein Anhang aus den Eingabedaten (source="data"). Die
// Pipeline bettet die Daten selbst ein, im FO steht er nicht.
type Attachment struct {
	ID, Name, MIMEType, Relationship, Description string
}

// Result ist eine kompilierte Vorlage.
type Result struct {
	XSLT        []byte
	SourceMap   []Mapping
	Attachments []Attachment

	sheet *engine.Stylesheet
}

// IDAt liefert die Vorlagen-ID zur Zeile im Stylesheet, bei verschachtelten
// Bereichen die innerste.
func (r *Result) IDAt(line int) string {
	best := -1
	for i, m := range r.SourceMap {
		if m.Start <= line && line <= m.End && (best < 0 || m.Start >= r.SourceMap[best].Start) {
			best = i
		}
	}
	if best < 0 {
		return ""
	}
	return r.SourceMap[best].ID
}

type node = template.Node

type compiler struct {
	t       *template.Template
	w       *writer
	errs    template.Errors
	styles  map[string]*node
	locale  *node
	texts   map[string]string
	lang    string
	table   *tableCtx
	attachs []Attachment
}

// Compile übersetzt eine geprüfte Vorlage in XSLT 3.0.
func Compile(t *template.Template, opts Options) (*Result, error) {
	t, errs := resolveTokens(t)
	if len(errs) > 0 {
		return nil, errs
	}
	c := &compiler{t: t, w: newWriter(), styles: map[string]*node{}, texts: map[string]string{}}
	c.lang = opts.Lang
	if c.lang == "" {
		c.lang = t.Root.Get("lang", "")
	}
	c.resources()
	c.stylesheet()
	if len(c.errs) > 0 {
		return nil, c.errs
	}
	res := &Result{XSLT: c.w.bytes(), SourceMap: c.w.srcmap, Attachments: c.attachs}
	if err := res.check(); err != nil {
		return nil, err
	}
	return res, nil
}

func (c *compiler) errorf(n *node, format string, args ...any) {
	c.errs = append(c.errs, &template.Error{ID: n.NearestID(), Line: n.Line, Col: n.Col, Msg: fmt.Sprintf(format, args...)})
}

// matchLang wählt aus Elementen mit lang das passende: gleiche Sprache, dann
// gleiche Hauptsprache, sonst das erste.
func matchLang(ns []*node, lang string) *node {
	if len(ns) == 0 {
		return nil
	}
	primary := func(s string) string { return strings.ToLower(strings.SplitN(s, "-", 2)[0]) }
	for _, n := range ns {
		if strings.EqualFold(n.Get("lang", ""), lang) {
			return n
		}
	}
	for _, n := range ns {
		if primary(n.Get("lang", "")) == primary(lang) {
			return n
		}
	}
	return ns[0]
}

func (c *compiler) resources() {
	root := c.t.Root
	if s := root.First("styles"); s != nil {
		for _, st := range s.Elements("style") {
			c.styles[st.ID()] = st
		}
	}
	if f := root.First("formats"); f != nil {
		c.locale = matchLang(f.Elements("locale"), c.lang)
	}
	if tx := root.First("texts"); tx != nil {
		if l := matchLang(tx.Elements("lang"), c.lang); l != nil {
			for _, s := range l.Elements("string") {
				var b strings.Builder
				for _, ch := range s.Children {
					if ch.Kind == template.Text {
						b.WriteString(ch.Data)
					}
				}
				c.texts[s.Get("key", "")] = b.String()
			}
		}
	}
}

// open öffnet ein Element und den Quellbereich der Vorlagen-ID. Das
// zugehörige close schließt beides.
func (c *compiler) open(id, name string, attrs ...string) int {
	i := c.w.enter(id)
	c.w.start(name, attrs...)
	return i
}

func (c *compiler) close(i int) {
	c.w.end()
	c.w.leave(i)
}

func (c *compiler) stylesheet() {
	root := c.t.Root
	attrs := []string{"version", "3.0", "xmlns:xsl", nsXSL, "xmlns:fo", nsFO, "xmlns:pdx", tree.NSPDX, "xmlns:xs", nsXS}
	exclude := []string{"xs"}
	if nss := root.First("namespaces"); nss != nil {
		for _, ns := range nss.Elements("ns") {
			p := ns.Get("prefix", "")
			if reserved[p] {
				c.errorf(ns, "Präfix %s ist für das Stylesheet reserviert", p)
				continue
			}
			attrs = append(attrs, "xmlns:"+p, ns.Get("uri", ""))
			exclude = append(exclude, p)
		}
	}
	attrs = append(attrs, "exclude-result-prefixes", strings.Join(exclude, " "))
	top := c.w.enter(root.ID())
	c.w.start("xsl:stylesheet", attrs...)
	c.w.comment("Erzeugt von pdx aus der Vorlage " + root.ID() + ". Nicht von Hand bearbeiten.")
	c.w.empty("xsl:output", "method", "xml", "encoding", "UTF-8", "indent", "no")
	if c.locale != nil {
		i := c.w.enter(c.locale.ID())
		c.w.empty("xsl:decimal-format", "name", c.locale.ID(),
			"decimal-separator", c.locale.Get("decimalSeparator", "."),
			"grouping-separator", c.locale.Get("groupingSeparator", ","))
		c.w.leave(i)
	}

	c.w.start("xsl:template", "match", "/")
	c.w.empty("xsl:apply-templates", "select", "*")
	c.w.end()

	dataRoot := root.Get("dataRoot", "")
	if dataRoot != "" {
		c.w.start("xsl:template", "match", "/*")
		c.w.start("xsl:message", "terminate", "yes")
		c.w.text("Die Daten haben das Wurzelelement ")
		c.w.empty("xsl:value-of", "select", "name()")
		c.w.text(", erwartet ist " + dataRoot + ".")
		c.w.end()
		c.w.end()
	}
	match := "/*"
	if dataRoot != "" {
		match = "/" + dataRoot
	}
	c.w.start("xsl:template", "match", match)
	c.foRoot()
	c.w.end()
	c.w.end()
	c.w.leave(top)
}

func (c *compiler) foRoot() {
	root := c.t.Root
	// Die Dokumentsprache folgt der gewählten Sprache (Option Lang), sonst der
	// Vorlage.
	r := c.open(root.ID(), "fo:root", "pdx:id", root.ID(), "xml:lang", c.lang)
	ps := root.First("pageSet")
	c.masters(ps)
	c.declarations()
	body := root.First("body")
	inh, non := c.styleSplit(body)
	attrs := append([]string{"pdx:id", ps.ID(), "id", SequenceID, "master-reference", ps.ID()}, inh...)
	seq := c.open(ps.ID(), "fo:page-sequence", attrs...)
	for _, pm := range ps.Elements("pageMaster") {
		for _, name := range []string{"header", "footer"} {
			if reg := pm.First(name); reg != nil {
				i := c.open(reg.ID(), "fo:static-content", "pdx:id", reg.ID(), "flow-name", reg.ID())
				kids := reg.Elements("")
				if len(kids) == 0 {
					c.w.empty("fo:block", "pdx:id", reg.ID())
				}
				for _, k := range kids {
					c.content(k, true)
				}
				c.close(i)
			}
		}
	}
	fl := c.open(body.ID(), "fo:flow", "pdx:id", body.ID(), "flow-name", "xsl-region-body")
	wrap := -1
	if len(non) > 0 {
		wrap = c.open(body.ID(), "fo:block", append([]string{"pdx:id", body.ID()}, non...)...)
	}
	kids := body.Elements("")
	if len(kids) == 0 {
		c.w.empty("fo:block", "pdx:id", body.ID())
	}
	for _, k := range kids {
		c.content(k, false)
	}
	if wrap >= 0 {
		c.close(wrap)
	}
	c.close(fl)
	c.close(seq)
	c.close(r)
}

// Seitenformate in Millimetern (Breite, Höhe im Hochformat).
var sizes = map[string][2]string{
	"A3": {"297mm", "420mm"}, "A4": {"210mm", "297mm"}, "A5": {"148mm", "210mm"},
	"Letter": {"8.5in", "11in"}, "Legal": {"8.5in", "14in"},
}

// Reihenfolge, in der die Bedingungen der Seitenvorlagen geprüft werden.
var useOrder = []string{"only", "first", "last", "odd", "even", "rest", "any"}

func (c *compiler) masters(ps *node) {
	lms := c.open(ps.ID(), "fo:layout-master-set", "pdx:id", ps.ID())
	pms := ps.Elements("pageMaster")
	for _, pm := range pms {
		w, h := pm.Get("width", ""), pm.Get("height", "")
		if sz := pm.Get("size", "A4"); sz != "custom" {
			w, h = sizes[sz][0], sizes[sz][1]
		} else if w == "" || h == "" {
			c.errorf(pm, "size=\"custom\" verlangt width und height")
		}
		if pm.Get("orientation", "portrait") == "landscape" {
			w, h = h, w
		}
		i := c.open(pm.ID(), "fo:simple-page-master", "pdx:id", pm.ID(), "master-name", pm.ID(),
			"page-width", w, "page-height", h, "margin", lengths(pm.Get("margin", "20mm")))
		var top, bottom string
		hd, ft := pm.First("header"), pm.First("footer")
		if hd != nil {
			top = c.addLengths(hd, hd.Get("height", ""), hd.Get("gap", "5mm"))
		}
		if ft != nil {
			bottom = c.addLengths(ft, ft.Get("height", ""), ft.Get("gap", "5mm"))
		}
		c.w.empty("fo:region-body", "pdx:id", pm.ID(), "margin-top", top, "margin-bottom", bottom)
		if hd != nil {
			c.w.empty("fo:region-before", "pdx:id", hd.ID(), "region-name", hd.ID(), "extent", hd.Get("height", ""))
		}
		if ft != nil {
			c.w.empty("fo:region-after", "pdx:id", ft.ID(), "region-name", ft.ID(), "extent", ft.Get("height", ""))
		}
		c.close(i)
	}
	i := c.open(ps.ID(), "fo:page-sequence-master", "pdx:id", ps.ID(), "master-name", ps.ID())
	c.w.start("fo:repeatable-page-master-alternatives", "pdx:id", ps.ID())
	for _, use := range useOrder {
		for _, pm := range pms {
			if pm.Get("use", "any") != use {
				continue
			}
			// rest heißt in der Vorlage jede Seite außer der ersten. In XSL
			// schließt page-position="rest" die letzte Seite aus (6.4.12),
			// daher folgt eine zweite Referenz für die letzte Seite.
			conds := [][]string{nil}
			switch use {
			case "only", "first", "last":
				conds = [][]string{{"page-position", use}}
			case "rest":
				conds = [][]string{{"page-position", "rest"}, {"page-position", "last"}}
			case "odd", "even":
				conds = [][]string{{"odd-or-even", use}}
			}
			j := c.w.enter(pm.ID())
			for _, cond := range conds {
				c.w.empty("fo:conditional-page-master-reference", append([]string{"pdx:id", pm.ID(), "master-reference", pm.ID()}, cond...)...)
			}
			c.w.leave(j)
		}
	}
	c.w.end()
	c.close(i)
	c.close(lms)
}

func (c *compiler) declarations() {
	root := c.t.Root
	var files []*node
	if as := root.First("attachments"); as != nil {
		for _, a := range as.Elements("attachment") {
			if a.Get("source", "") == "data" {
				c.attachs = append(c.attachs, Attachment{ID: a.ID(), Name: a.Get("name", ""),
					MIMEType: a.Get("mimeType", ""), Relationship: a.Get("relationship", "Unspecified"),
					Description: a.Get("description", "")})
				continue
			}
			files = append(files, a)
		}
	}
	title := root.Get("title", "")
	if title == "" && len(files) == 0 {
		return
	}
	c.w.start("fo:declarations")
	if title != "" {
		c.w.empty("pdx:metadata", "title", title)
	}
	for _, a := range files {
		i := c.w.enter(a.ID())
		c.w.empty("pdx:attachment", "src", a.Get("source", ""), "name", a.Get("name", ""),
			"mime-type", a.Get("mimeType", ""), "relationship", a.Get("relationship", "Unspecified"),
			"description", a.Get("description", ""))
		c.w.leave(i)
	}
	c.w.end()
}

// presence umschließt den Inhalt mit xsl:if, wenn das Element presence hat.
func (c *compiler) presence(n *node, body func()) {
	if p, ok := n.Attr("presence"); ok {
		c.w.start("xsl:if", "test", p)
		body()
		c.w.end()
		return
	}
	body()
}

// content übersetzt ein Inhaltselement. positioned gilt für Kinder von
// Kopf, Fuß und Teilformularen mit layout="positioned".
func (c *compiler) content(n *node, positioned bool) {
	region := c.w.enter(n.ID())
	defer c.w.leave(region)
	c.presence(n, func() {
		page := n.Name == "subform" && n.Get("anchor", "flow") == "page"
		if !positioned && !page {
			c.inner(n, false)
			return
		}
		attrs := []string{"pdx:id", n.ID(), "absolute-position", "absolute"}
		if page {
			attrs[3] = "fixed"
		}
		attrs = append(attrs, "left", orZero(n.Get("x", "")), "top", orZero(n.Get("y", "")),
			"width", n.Get("w", ""), "height", n.Get("h", ""))
		if n.Name == "subform" {
			// Das Teilformular ist selbst der Container, seine Bindung
			// umschließt ihn.
			c.bound(n, func() {
				c.start("fo:block-container", n, append(attrs, c.style(n)...)...)
				c.children(n)
				c.w.end()
			})
			return
		}
		c.start("fo:block-container", n, attrs...)
		c.inner(n, true)
		c.w.end()
	})
}

func orZero(s string) string {
	if s == "" {
		return "0pt"
	}
	return s
}

// bound setzt den Kontext eines Teilformulars mit bind: bei repeat="true"
// je Datenknoten, sonst auf den ersten.
func (c *compiler) bound(n *node, body func()) {
	b, ok := n.Attr("bind")
	if !ok {
		body()
		return
	}
	sel := paren(b) + "[1]"
	if n.Get("repeat", "false") == "true" {
		sel = b
	}
	c.w.start("xsl:for-each", "select", sel)
	body()
	c.w.end()
}

// children übersetzt die Kinder eines Teilformulars nach seinem Layout.
func (c *compiler) children(n *node) {
	kids := n.Elements("")
	if len(kids) == 0 {
		c.w.empty("fo:block", "pdx:id", n.ID())
	}
	for _, k := range kids {
		c.content(k, n.Get("layout", "") == "positioned")
	}
}

// inner übersetzt ein Element im Fluss. wrapped ist gesetzt, wenn ein
// absolut positionierter Container es bereits umschließt.
func (c *compiler) inner(n *node, wrapped bool) {
	switch n.Name {
	case "subform":
		c.bound(n, func() {
			attrs := []string{"pdx:id", n.ID()}
			name := "fo:block"
			if n.Get("layout", "") == "positioned" || n.Get("w", "") != "" || n.Get("h", "") != "" {
				name = "fo:block-container"
				if n.Get("layout", "") == "positioned" && n.Get("h", "") == "" {
					c.errorf(n, "Teilformular mit layout=\"positioned\" im Fluss braucht h")
				}
				attrs = append(attrs, "width", n.Get("w", ""), "height", n.Get("h", ""))
			}
			attrs = append(attrs, c.blockAttrs(n)...)
			c.w.start(name, attrs...)
			c.children(n)
			c.w.end()
		})
	case "text":
		c.w.start("fo:block", append([]string{"pdx:id", n.ID()}, c.blockAttrs(n)...)...)
		c.altBind(n)
		c.textContent(n)
		c.w.end()
	case "field":
		c.start("fo:block", n, append([]string{"pdx:id", n.ID()}, c.style(n)...)...)
		c.w.empty("xsl:value-of", "select", c.fieldExpr(n))
		c.w.end()
	case "pageNumber":
		c.w.start("fo:block", append([]string{"pdx:id", n.ID()}, c.blockAttrs(n)...)...)
		c.altBind(n)
		c.pageNumber(n)
		c.w.end()
	case "image":
		c.start("fo:block", n, "pdx:id", n.ID())
		c.w.empty("fo:external-graphic", "pdx:id", n.ID(), "src", "url('"+n.Get("src", "")+"')",
			"content-width", n.Get("w", ""), "content-height", n.Get("h", ""))
		c.w.end()
	case "line":
		c.line(n, wrapped)
	case "rect":
		c.rect(n, wrapped)
	case "barcode":
		c.barcode(n)
	case "table":
		c.tableElem(n)
	default:
		c.errorf(n, "Element %s wird vom Compiler nicht unterstützt", n.Name)
	}
}

func (c *compiler) line(n *node, wrapped bool) {
	border := strings.TrimSpace(n.Get("thickness", "0.5pt") + " " + n.Get("lineStyle", "solid") + " " + n.Get("color", "black"))
	side := "border-top"
	if n.Get("w", "") == "" && n.Get("h", "") != "" {
		side = "border-left"
	}
	attrs := []string{"pdx:id", n.ID()}
	if !wrapped {
		attrs = append(attrs, "width", n.Get("w", ""), "height", n.Get("h", ""))
	}
	attrs = append(attrs, side, border)
	c.start("fo:block-container", n, attrs...)
	c.w.empty("fo:block", "pdx:id", n.ID())
	c.w.end()
}

func (c *compiler) rect(n *node, wrapped bool) {
	attrs := []string{"pdx:id", n.ID()}
	if !wrapped {
		attrs = append(attrs, "width", n.Get("w", ""), "height", n.Get("h", ""))
	}
	attrs = append(attrs, "background-color", n.Get("fill", ""), "border", n.Get("border", ""))
	c.start("fo:block-container", n, attrs...)
	c.w.empty("fo:block", "pdx:id", n.ID())
	c.w.end()
}

func (c *compiler) barcode(n *node) {
	c.start("fo:block", n, "pdx:id", n.ID())
	c.w.start("fo:instream-foreign-object", "pdx:id", n.ID(), "content-width", n.Get("w", ""), "content-height", n.Get("h", ""))
	c.w.start("pdx:barcode", "type", n.Get("type", ""), "value", n.Get("value", ""),
		"module-width", n.Get("moduleWidth", ""), "bar-height", n.Get("barHeight", ""))
	if b, ok := n.Attr("bind"); ok {
		c.w.empty("xsl:attribute", "name", "value", "select", b)
	}
	for _, d := range n.Elements("data") {
		i := c.w.enter(d.ID())
		if b, ok := d.Attr("bind"); ok {
			c.w.empty("xsl:attribute", "name", d.Get("name", ""), "select", b)
		} else {
			c.w.empty("xsl:attribute", "name", d.Get("name", ""), "select", xpathString(d.Get("value", "")))
		}
		c.w.leave(i)
	}
	c.w.end()
	c.w.end()
	c.w.end()
}

// textContent schreibt den Inhalt eines text-Elements: Textressource oder
// gemischten Inhalt.
func (c *compiler) textContent(n *node) {
	if ref, ok := n.Attr("ref"); ok {
		for _, ch := range n.Children {
			if ch.Kind == template.Element || strings.TrimSpace(ch.Data) != "" {
				c.errorf(n, "text mit ref darf keinen Inhalt haben")
				return
			}
		}
		s, ok := c.texts[ref]
		if !ok {
			c.errorf(n, "Textressource %q fehlt für die Sprache %s", ref, c.lang)
			return
		}
		c.w.text(s)
		return
	}
	c.inlines(n)
}

func (c *compiler) inlines(n *node) {
	for _, ch := range n.Children {
		if ch.Kind == template.Text {
			c.w.text(ch.Data)
			continue
		}
		i := c.w.enter(ch.ID())
		c.presence(ch, func() {
			switch ch.Name {
			case "span":
				attrs := append([]string{"pdx:id", ch.ID()}, c.style(ch)...)
				if href, ok := ch.Attr("href"); ok {
					c.start("fo:basic-link", ch, append(attrs, "external-destination", "url('"+href+"')")...)
				} else {
					c.start("fo:inline", ch, attrs...)
				}
				c.inlines(ch)
				c.w.end()
			case "field":
				c.start("fo:inline", ch, append([]string{"pdx:id", ch.ID()}, c.style(ch)...)...)
				c.w.empty("xsl:value-of", "select", c.fieldExpr(ch))
				c.w.end()
			case "pageNumber":
				c.start("fo:inline", ch, append([]string{"pdx:id", ch.ID()}, c.style(ch)...)...)
				c.pageNumber(ch)
				c.w.end()
			case "br":
				c.w.empty("fo:block", "pdx:id", ch.ID())
			}
		})
		c.w.leave(i)
	}
}

var placeholder = regexp.MustCompile(`\{[^{}]*\}`)

// pageNumber schreibt das Muster mit {current} und {total}.
func (c *compiler) pageNumber(n *node) {
	pat := n.Get("pattern", "{current}")
	last := 0
	for _, m := range placeholder.FindAllStringIndex(pat, -1) {
		c.w.text(pat[last:m[0]])
		switch pat[m[0]:m[1]] {
		case "{current}":
			c.w.empty("fo:page-number", "pdx:id", n.ID())
		case "{total}":
			c.w.empty("fo:page-number-citation-last", "pdx:id", n.ID(), "ref-id", SequenceID)
		default:
			c.errorf(n, "unbekannter Platzhalter %s im Muster, erlaubt sind {current} und {total}", pat[m[0]:m[1]])
		}
		last = m[1]
	}
	c.w.text(pat[last:])
}

var simplePath = regexp.MustCompile(`^[\p{L}\p{N}_:./@*\-]+$`)

// paren klammert einen Ausdruck, wenn er kein einfacher Pfad ist.
func paren(expr string) string {
	if simplePath.MatchString(expr) {
		return expr
	}
	return "(" + expr + ")"
}

// pattern sucht ein Formatmuster im gewählten Gebietsschema.
func (c *compiler) pattern(n *node, name string) *node {
	if c.locale == nil {
		c.errorf(n, "Formatmuster %q verlangt formats in der Vorlage", name)
		return nil
	}
	for _, p := range c.locale.Elements("pattern") {
		if p.Get("name", "") == name {
			return p
		}
	}
	c.errorf(n, "Formatmuster %q fehlt im Gebietsschema %s", name, c.locale.Get("lang", ""))
	return nil
}

// formatCall formatiert arg mit dem Muster.
func (c *compiler) formatCall(p *node, arg string) string {
	pic := xpathString(p.Get("picture", ""))
	lang := xpathString(strings.SplitN(c.locale.Get("lang", ""), "-", 2)[0])
	switch p.Get("kind", "") {
	case "date":
		return "format-date(xs:date(" + arg + "), " + pic + ", " + lang + ", (), ())"
	case "dateTime":
		return "format-dateTime(xs:dateTime(" + arg + "), " + pic + ", " + lang + ", (), ())"
	}
	return "format-number(xs:decimal(" + arg + "), " + pic + ", " + xpathString(c.locale.ID()) + ")"
}

// fieldExpr liefert den Ausdruck für den Wert eines Feldes.
func (c *compiler) fieldExpr(n *node) string {
	base := paren(n.Get("bind", ""))
	def, hasDef := n.Attr("default")
	if hasDef {
		base += "[normalize-space()]"
	}
	expr := base
	if f, ok := n.Attr("format"); ok {
		if p := c.pattern(n, f); p != nil {
			expr = base + " ! " + c.formatCall(p, ".")
		}
	}
	if hasDef {
		expr = "(" + expr + ", " + xpathString(def) + ")[1]"
	}
	return expr
}

// styleProps ordnet Stilattribute FO-Eigenschaften zu. Die Reihenfolge ist
// die Reihenfolge im erzeugten FO.
var styleProps = []struct {
	attr, fo  string
	inherited bool
}{
	{"fontFamily", "font-family", true},
	{"fontSize", "font-size", true},
	{"fontWeight", "font-weight", true},
	{"fontStyle", "font-style", true},
	{"lineHeight", "line-height", true},
	{"color", "color", true},
	{"textAlign", "text-align", true},
	{"hyphenate", "hyphenate", true},
	{"backgroundColor", "background-color", false},
	{"spaceBefore", "space-before", false},
	{"spaceAfter", "space-after", false},
	{"padding", "padding", false},
	{"border", "border", false},
	{"borderTop", "border-top", false},
	{"borderBottom", "border-bottom", false},
	{"borderLeft", "border-left", false},
	{"borderRight", "border-right", false},
}

// resolved liefert die wirksamen Stilattribute: Kette der Stile über parent,
// dann die eigenen Attribute des Elements.
func (c *compiler) resolved(n *node) map[string]string {
	out := map[string]string{}
	var chain []*node
	seen := map[string]bool{}
	for id := n.Get("style", ""); id != ""; {
		st, ok := c.styles[id]
		if !ok {
			c.errorf(n, "%q ist kein Stil", id)
			break
		}
		if seen[id] {
			c.errorf(st, "Stil %q erbt von sich selbst", id)
			break
		}
		seen[id] = true
		chain = append(chain, st)
		id = st.Get("parent", "")
	}
	for i := len(chain) - 1; i >= 0; i-- {
		for _, a := range chain[i].Attrs {
			out[a.Name] = a.Value
		}
	}
	for _, a := range n.Attrs {
		out[a.Name] = a.Value
	}
	return out
}

// style liefert die FO-Eigenschaften der Stilattribute als Name-Wert-Paare.
func (c *compiler) style(n *node) []string {
	inh, non := c.styleSplit(n)
	return append(inh, non...)
}

// styleSplit trennt vererbte und nicht vererbte Eigenschaften.
func (c *compiler) styleSplit(n *node) (inh, non []string) {
	r := c.resolved(n)
	for _, p := range styleProps {
		v, ok := r[p.attr]
		if !ok {
			continue
		}
		if p.attr == "padding" {
			v = lengths(v)
		}
		if p.inherited {
			inh = append(inh, p.fo, v)
		} else {
			non = append(non, p.fo, v)
		}
	}
	return inh, non
}

// semantic liefert die Angaben für Tagged PDF als FO-Attribute: Rolle,
// statischer Alternativtext, Sprache, Lesereihenfolge und Artefakt. Ein
// Alternativtext aus den Daten (altBind) folgt mit altBind als
// xsl:attribute direkt nach dem Start des Elements.
func (c *compiler) semantic(n *node) []string {
	out := []string{"role", n.Get("role", ""), "xml:lang", n.Get("lang", ""),
		"pdx:reading-order", n.Get("readingOrder", "")}
	if _, dyn := n.Attr("altBind"); !dyn {
		out = append(out, "pdx:alt", n.Get("alt", ""))
	}
	// artifact="false" macht Inhalt von Kopf und Fuß zu echtem Inhalt.
	if v, ok := n.Attr("artifact"); ok {
		out = append(out, "pdx:artifact", v)
	}
	return out
}

// altBind schreibt den Alternativtext aus den Daten als Attribut des eben
// begonnenen Elements.
func (c *compiler) altBind(n *node) {
	if b, ok := n.Attr("altBind"); ok {
		c.w.empty("xsl:attribute", "name", "pdx:alt", "select", "string("+paren(b)+")")
	}
}

// start beginnt ein FO-Element mit den Angaben für Tagged PDF.
func (c *compiler) start(name string, n *node, attrs ...string) {
	c.w.start(name, append(attrs, c.semantic(n)...)...)
	c.altBind(n)
}

// blockAttrs liefert Rolle, Umbrüche, Keeps und Stil eines Blockelements.
func (c *compiler) blockAttrs(n *node) []string {
	out := c.semantic(n)
	breaks := map[string]string{"page": "page", "oddPage": "odd-page", "evenPage": "even-page"}
	if v := n.Get("breakBefore", "auto"); v != "auto" {
		out = append(out, "break-before", breaks[v])
	}
	if v := n.Get("breakAfter", "auto"); v != "auto" {
		out = append(out, "break-after", breaks[v])
	}
	for _, k := range [][2]string{{"keepTogether", "keep-together.within-page"}, {"keepWithNext", "keep-with-next.within-page"}, {"keepWithPrevious", "keep-with-previous.within-page"}} {
		if v := n.Get(k[0], "auto"); v != "auto" {
			out = append(out, k[1], v)
		}
	}
	return append(out, c.style(n)...)
}

var lengthRe = regexp.MustCompile(`^(-?[0-9]+(?:\.[0-9]+)?)(pt|mm|cm|in|pc|px|%)?$`)

// lengths ergänzt Zahlen ohne Einheit um pt.
func lengths(s string) string {
	parts := strings.Fields(s)
	for i, p := range parts {
		if m := lengthRe.FindStringSubmatch(p); m != nil && m[2] == "" && m[1] != "0" {
			parts[i] = p + "pt"
		}
	}
	return strings.Join(parts, " ")
}

// Punkte je Einheit.
var unitPt = map[string]float64{"pt": 1, "mm": 72 / 25.4, "cm": 72 / 2.54, "in": 72, "pc": 12, "px": 0.75, "": 1}

// addLengths addiert zwei Längen. Gleiche Einheiten bleiben erhalten, sonst
// wird in Punkten gerechnet.
func (c *compiler) addLengths(n *node, a, b string) string {
	ma, mb := lengthRe.FindStringSubmatch(a), lengthRe.FindStringSubmatch(b)
	if ma == nil || mb == nil || ma[2] == "%" || mb[2] == "%" {
		c.errorf(n, "Längen %q und %q lassen sich nicht addieren", a, b)
		return a
	}
	x, _ := strconv.ParseFloat(ma[1], 64)
	y, _ := strconv.ParseFloat(mb[1], 64)
	unit := ma[2]
	if ma[2] != mb[2] {
		x, y, unit = x*unitPt[ma[2]], y*unitPt[mb[2]], "pt"
	}
	if unit == "" {
		unit = "pt"
	}
	return strconv.FormatFloat(x+y, 'f', -1, 64) + unit
}
