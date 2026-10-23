# Deliberate deviations from Apache FOP

The reference is the W3C XSL 1.1 specification, not Apache FOP. pdx follows the XSL 1.1 specification. Where Apache FOP deviates from it, or where
the specification leaves room that the two engines use differently,
the case is listed here with a reason. The FOP comparison does not count these
cases as errors.

## left and top with absolute positioning

XSL 1.1 sections 7.6.4 to 7.6.7 take `top`, `right`, `bottom` and
`left` from CSS2 9.3.2. There, `left` is described as: "specifies how far a box's
left margin edge is offset to the right of the left edge of the box's
containing block". This means the margin edge. Without a margin, this is
the outer edge of the border.

For `fo:block-container` with `absolute-position`, FOP 2.11 places the
content edge at `left` horizontally, and the border extends beyond it on the left
(negative `space-start`). Vertically, FOP uses the margin edge. pdx uses the
margin edge in both directions. Effect: lines in absolutely positioned
containers with a border or padding are shifted horizontally by border plus padding
(corpus: adressblock).

In both engines, `width` and `height` refer to the
content rectangle (XSL 1.1 7.15.14, `inline-progression-dimension`). Without
`width`, the width follows from `left`, `right`, margins,
borders and padding according to CSS2 10.3.7. In pdx, margins shift the box in both directions.
FOP drops margin-top vertically (case
block-container_absolute-position_border_padding).

## Ligatures

During shaping, pdx applies the standard OpenType features, including `liga`
(fi, fl, ffi). FOP 2.11 uses no ligatures for Latin script. XSL
1.1 does not regulate ligatures. Glyph selection is up to the implementation
(4.7.2 "glyph-areas"). Ligatures shorten words slightly, so in borderline cases a line can
break differently. This can be switched off in pdx
through the shaping options.

## Distribution of white space in justified text

Both engines stretch U+0020 and U+00A0, as XSL 1.1 7.17.7
(`treat-as-word-space`) specifies. How the white space is distributed among the
gaps is left open by the specification: "Word spacing
algorithms are user agent-dependent" (7.17.8 from CSS2). pdx distributes it
in proportion to the stretchability of each gap. In lines with a lot of
white space (for example `text-align-last="justify"` with few words), words
are therefore placed at different positions, while line contents and baselines
match. For columns, a table is the right tool.

## Tables in the collapse model

`border-collapse="collapse"` is the initial value (XSL 1.1 7.28.3). pdx resolves
every cell edge between cell, neighboring cell, row, section, column and
table where they adjoin (6.7.10): `hidden` suppresses the edge, otherwise
the wider border wins, then the stronger style, then the precedence according to
the initial values of `border-*-precedence` (7.28.1). If a cell spans
several rows, the widest side edge determines its indent. The
`collapse-with-precedence` model with its own precedences is treated like `collapse`.

## fo:retrieve-table-marker in the table header

XSL 1.1 6.13.7 and 7.25.6 allow `fo:retrieve-table-marker` in
`fo:table-header` and `fo:table-footer`. For `first-starting`, the
first area with a marker that starts in the primary retrieve scope (the table fragment
on the page) applies. pdx therefore lays out the header only after the
body rows of the page are fixed, and shows in the header of the following page the
carry-over from the first row that starts there.

FOP 2.11 does not fill table markers in the header, and the header stays empty at this
position. In the footer, both engines behave the same. The comparison does not count the
missing line in the FOP header as an error (corpus: uebertrag).

In the sample invoice (template testdata/templates/musterrechnung), the
carry-over row in the header of the following pages stays empty in FOP and is therefore lower. All
rows on the page are placed higher in FOP by its height (12.35 pt). The
page break is the same in both engines.

FOP 2.11 also aborts when `fo:retrieve-table-marker` is placed directly in
`fo:table-cell` (NullPointerException in TableCellLayoutManager) or
when a marker that is retrieved into an `fo:block` contains blocks
(ClassCastException). The compiler therefore always places the marker in a
block and writes only inline content in carry-over rows. Both are valid
XSL and do not change the result of pdx.

In FOP, markers must be placed in `fo:table-cell`. XSL also allows them
directly in `fo:table-row`, and pdx supports both. Markers on `fo:inline`
are not yet implemented in pdx.

## Hyphenation

pdx ships hyphenation patterns for German (de-1996) and American English
(en-us) from hyph-utf8. For license reasons, FOP 2.11 ships no patterns
(the OFFO project is distributed separately), and without them FOP does not hyphenate.
Cases with `hyphenate="true"` are therefore not compared with FOP
(corpus: silbentrennung). Like FOP, pdx first tries a line break without
hyphenation points and gives hyphenation points a penalty of 50.

## Bookmarks with external-destination

XSL 1.1 6.11.2 allows `external-destination` on `fo:bookmark` in addition to
`internal-destination`. For it, pdx writes an outline entry with a
URI action. FOP 2.11 aborts with "DestinationData must contain a ID
reference". The corpus case verweise therefore uses only internal destinations.
External bookmarks are checked by a separate test (pdf/render).

## Barcodes in fo:instream-foreign-object

pdx places `pdx:barcode` in `fo:instream-foreign-object`, the place that XSL
1.1 6.6.6 provides for content from foreign namespaces. FOP 2.11 does not know the
element: the PDF is created without the barcode, FOP reports "The intrinsic
dimensions of an instream-foreign-object could not be determined" (SEVERE),
and the area tree export (`-at`) aborts. The barcodes case is not compared with
FOP. Instead, a test decodes the rasterized barcodes with
zbar (QR code and Code 128) and checks their content.

## Attributes from foreign namespaces (pdx:id)

XSL 1.1 1.1.2 allows attributes from other namespaces on all
formatting objects, and an implementation may ignore them. The
compiler writes `pdx:id` with the ID of the template element on every generated FO element,
so that the designer and error messages can map areas to their
origin.

In its default setting, FOP 2.11 rejects attributes from namespaces without its own element mapping
as invalid properties and aborts ("Invalid
property encountered on fo:root: pdx:id"). With relaxed validation (`fop -r`
or `<strict-validation>false</strict-validation>` in the configuration),
FOP only prints warnings and lays out the document. FOP offers no configuration that
ignores only one namespace, only the Java API does
(`FopFactoryBuilder.ignoreNamespace`). The FOP comparison of the sample invoice
therefore runs with `-r`.

## Invalid property values

XSL 1.1 defines no fallback rule for invalid values. According to 5.9.6, a
length without a unit is a number with power zero. If the property requires
a length (power one), the specification says: "If any other power is specified, the
value is an error." The permitted units are listed in 5.9.13 (cm, mm, in,
pt, pc, px, em). pdx reports such errors with their position and aborts instead of
silently using a fallback value. FOP, in contrast, continues, for example with the
initial value or with millipoints for numbers without a unit. This affects
test cases with values such as `space-after="100"`, `height="8400mpt"`,
`font-style="obscure"`, `color="invalidvalue"` or `padding="auto"`.

Attributes from foreign namespaces, on the other hand, must be ignored by a processor without an error
(XSL 1.1 1.2: "must ignore such attributes without giving an
error if it does not recognize the namespace URI"). pdx does this. FOP 2.11
violates this rule without `-r` (see Attributes from foreign namespaces).

## Missing characters

If no font has a glyph for a character, XSL 1.1 7.9.3 recommends a
visible replacement: "the User Agent should provide a visual indication
that a character is not being displayed (for example, using the 'missing
character' glyph)". PDF/A forbids references to the .notdef glyph
(ISO 19005-2 6.2.11.8). pdx therefore reports missing characters as errors with their
position instead of producing a non-conforming file. This affects
test cases with U+0085, U+007F and U+3000, for which the built-in
Liberation fonts have no glyph. According to 7.17.7, U+3000 is not a
word space but an ordinary character.

## body-start()

XSL 1.1 7.30.12 defines: "body-start() = the value of the start-indent +
start-intrusion-adjustment + the value of the provisional-distance-between-starts
of the closest ancestor fo:list-block". As with label-end(), where parentheses
group the values, pdx reads all values from the closest fo:list-block. FOP
takes the start-indent of the list item. Differences only occur when a
list item has its own margins, borders or padding.

## Padding on tables and cells

According to XSL 1.1 5.3.2, start-indent is the sum of margin, border and padding,
and the content rectangle starts at start-indent (4.2.2). pdx therefore indents the content
of a table or cell by border plus padding. FOP adds the
padding again when a margin is specified, and indents the content by
twice the padding (cases table_padding_percentages and
table-cell_padding_percentages).

## Unit px, thin, medium and thick

For px, XSL 1.1 5.9.13.1 requires a device-dependent size according to the
"arc-span rule" from CSS2, that is, the reference pixel. Like CSS 2.1, pdx uses 1/96
inch (0.75 pt). FOP uses 1/72 inch. According to
7.8.24, the widths thin, medium and thick are up to the formatter (pdx: 0.5 pt, 1 pt, 2 pt).

## Spaces at inline boundaries

XSL 1.1 7.16.12 summarizes the effect of white-space-collapse as follows:
after refinement, every remaining sequence of two or more
spaces is replaced by a single one. A boundary of fo:inline does not interrupt
the sequence. pdx keeps the first space. FOP keeps one before and one after
the boundary when the fo:inline has a border, and therefore breaks lines
differently (cases nist-cbpbp-border-color-inline3 and
nist-cbpbp-border-width-inline1).

## Choice within the range of a space

According to XSL 1.1 4.3, the resolved minimum and maximum values only limit a
space. Which value within the range the formatter uses is left open by the
specification. The optimum is the preferred value. pdx uses the
optimum and only compresses when a fo:block-container with a fixed height would otherwise
overflow. FOP also compresses spaces to fill pages (cases fopc-hyphen,
fopc-leader, fopc-pagelevel2 and fopc-pagelevel3). If only .optimum is
specified, .minimum stays at its initial value 0pt (7.11.5).

## page-position="rest"

XSL 1.1 7.27.14: "This master is eligible for selection if this is not the
first page nor the last page in the page-sequence." A page master with rest
therefore does not apply to the last page. If no alternative matches, pdx uses the
most recently used page master. FOP also applies rest to the last page (case
list-item-with-ipd-change).
