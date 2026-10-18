package compile

import "strconv"

// Formularfelder werden zu pdx:form-field in einem fo:instream-foreign-object.
// Das Layout setzt daraus Widgets, der Renderer schreibt AcroForm-Felder.
// Jedes Feld trägt den Pfad seines Kontextknotens in den Daten (path(.)),
// damit pdx extract-data die Eingabe relativ dazu über die Bindung der
// Vorlage zurückschreiben kann, auch wenn der Datenknoten noch fehlt.

var fieldTypes = map[string]string{
	"textField": "text", "checkBox": "checkbox", "radioGroup": "radio",
	"listBox": "list", "comboBox": "combo", "signatureField": "signature",
}

// formField übersetzt Textfeld, Kontrollkästchen, Listen und Signaturfeld.
func (c *compiler) formField(n *node) {
	// Schrift und Farbe erbt das Feld vom Block, Rahmen, Hintergrund und
	// Innenabstand gehören zum Feld selbst.
	inh, non := c.styleSplit(n)
	c.start("fo:block", n, append([]string{"pdx:id", n.ID()}, inh...)...)
	c.w.start("fo:instream-foreign-object", append([]string{"pdx:id", n.ID(), "content-width", n.Get("w", ""), "content-height", n.Get("h", "")}, non...)...)
	c.fieldElem(n, fieldTypes[n.Name], "")
	c.options(n)
	c.w.end()
	c.w.end()
	c.w.end()
}

// fieldElem öffnet pdx:form-field mit allen Angaben und den berechneten
// Attributen value und context. export ist der Wert einer Option bei
// Optionsfeldern. Der Aufrufer schließt das Element.
func (c *compiler) fieldElem(n *node, typ, export string) {
	maxLen := n.Get("maxLength", "")
	if l := c.maxLens[n.ID()]; maxLen == "" && l > 0 && typ == "text" {
		maxLen = strconv.Itoa(l) // Höchstlänge aus dem Daten-XSD
	}
	attrs := []string{"type", typ, "name", n.ID(), "label", avtLiteral(n.Get("label", "")),
		"multiline", n.Get("multiline", ""), "max-length", maxLen,
		"read-only", n.Get("readOnly", ""), "required", n.Get("required", ""),
		"editable", n.Get("editable", ""), "export", avtLiteral(export)}
	if typ == "checkbox" {
		attrs = append(attrs, "on-value", avtLiteral(n.Get("onValue", "true")), "off-value", avtLiteral(n.Get("offValue", "false")))
	}
	c.w.start("pdx:form-field", attrs...)
	if b, ok := n.Attr("bind"); ok {
		c.w.empty("xsl:attribute", "name", "value", "select", "string("+paren(b)+"[1])")
		c.w.empty("xsl:attribute", "name", "context", "select", "path(.)")
	}
}

// options schreibt die Auswahlwerte: option-Elemente der Vorlage, sonst die
// Enumeration aus dem Daten-XSD.
func (c *compiler) options(n *node) {
	for _, o := range c.choices(n) {
		c.w.empty("pdx:option", "value", avtLiteral(o[0]), "label", avtLiteral(o[1]))
	}
}

// choices liefert Wert und Text je Auswahl.
func (c *compiler) choices(n *node) [][2]string {
	var out [][2]string
	for _, o := range n.Elements("option") {
		v := o.Get("value", "")
		out = append(out, [2]string{v, o.Get("label", v)})
	}
	if len(out) == 0 {
		for _, v := range c.enums[n.ID()] {
			out = append(out, [2]string{v, v})
		}
	}
	return out
}

// radioGroup schreibt je Option eine Zeile mit Optionsfeld und Text. Alle
// Optionsfelder gehören zu einem Feld mit dem Namen der Gruppe.
func (c *compiler) radioGroup(n *node) {
	c.start("fo:block", n, append([]string{"pdx:id", n.ID()}, c.blockAttrs(n)...)...)
	opts := c.choices(n)
	if len(opts) == 0 {
		c.errorf(n, "Optionsfeldgruppe ohne Optionen: option-Elemente oder Enumeration im Daten-XSD fehlen")
	}
	for _, o := range opts {
		c.w.start("fo:block")
		c.w.start("fo:instream-foreign-object", "pdx:id", n.ID())
		c.fieldElem(n, "radio", o[0])
		c.w.end()
		c.w.end()
		c.w.start("fo:inline", "padding-left", "0.4em")
		c.w.text(o[1])
		c.w.end()
		c.w.end()
	}
	c.w.end()
}
