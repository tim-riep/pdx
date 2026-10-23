package compile

import "strconv"

// Form fields become pdx:form-field in an fo:instream-foreign-object.
// The layout turns them into widgets, the renderer writes AcroForm fields.
// Every field carries the path of its context node in the data (path(.)),
// so that pdx extract-data can write the input back relative to it via the
// template's binding, even if the data node is still missing.

var fieldTypes = map[string]string{
	"textField": "text", "checkBox": "checkbox", "radioGroup": "radio",
	"listBox": "list", "comboBox": "combo", "signatureField": "signature",
}

// formField translates text field, check box, lists and signature field.
func (c *compiler) formField(n *node) {
	// The field inherits font and color from the block, border, background
	// and padding belong to the field itself.
	inh, non := c.styleSplit(n)
	c.start("fo:block", n, append([]string{"pdx:id", n.ID()}, inh...)...)
	c.w.start("fo:instream-foreign-object", append([]string{"pdx:id", n.ID(), "content-width", n.Get("w", ""), "content-height", n.Get("h", "")}, non...)...)
	c.fieldElem(n, fieldTypes[n.Name], "")
	c.options(n)
	c.w.end()
	c.w.end()
	c.w.end()
}

// fieldElem opens pdx:form-field with all properties and the computed
// attributes value and context. export is the value of an option for
// radio buttons. The caller closes the element.
func (c *compiler) fieldElem(n *node, typ, export string) {
	maxLen := n.Get("maxLength", "")
	if l := c.maxLens[n.ID()]; maxLen == "" && l > 0 && typ == "text" {
		maxLen = strconv.Itoa(l) // maximum length from the data XSD
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

// options writes the choice values: option elements of the template,
// otherwise the enumeration from the data XSD.
func (c *compiler) options(n *node) {
	for _, o := range c.choices(n) {
		c.w.empty("pdx:option", "value", avtLiteral(o[0]), "label", avtLiteral(o[1]))
	}
}

// choices returns value and text per choice.
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

// radioGroup writes one row with radio button and text per option. All
// radio buttons belong to one field with the name of the group.
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
