// Package areatree defines the area tree of pdx: pages with absolutely
// positioned areas, glyph runs, images, lines and rectangles.
//
// The area tree is the stable intermediate format between layout and output.
// The PDF writer, the later canvas preview in the designer and raster tests
// read it. It therefore depends on no other pdx package.
//
// All lengths are whole millipoints (1/1000 pt). Coordinates are absolute on
// the page, origin top left, y grows downward. The nesting of
// areas only reflects the structure (for selection in the designer and later
// for tagging), not the position.
package areatree

import "strconv"

// Length is a length in millipoints.
type Length int64

// Pt is one point (1/72 inch).
const Pt Length = 1000

// Points converts points to Length and rounds half away from zero.
func Points(f float64) Length {
	if f < 0 {
		return -Length(-f*1000 + 0.5)
	}
	return Length(f*1000 + 0.5)
}

// Pt returns the length in points.
func (l Length) Pt() float64 { return float64(l) / 1000 }

// String formats the length in points without trailing zeros.
func (l Length) String() string {
	neg := l < 0
	if neg {
		l = -l
	}
	s := strconv.FormatInt(int64(l/1000), 10)
	if frac := l % 1000; frac != 0 {
		f := strconv.FormatInt(int64(frac)+1000, 10)[1:]
		for f[len(f)-1] == '0' {
			f = f[:len(f)-1]
		}
		s += "." + f
	}
	if neg {
		s = "-" + s
	}
	return s
}

// Point is a point on the page.
type Point struct{ X, Y Length }

// Rect is a rectangle with top left corner X, Y.
type Rect struct{ X, Y, W, H Length }

// Bounds returns the rectangle itself and so satisfies part of Area.
func (r Rect) Bounds() Rect { return r }

// ColorSpace is the color space of a color.
type ColorSpace uint8

const (
	// Gray is DeviceGray, one component.
	Gray ColorSpace = iota
	// RGB is DeviceRGB, three components. Under PDF/A the output intent applies.
	RGB
	// CMYK is DeviceCMYK, four components. Not output in phase 0.
	CMYK
)

// Color is a color with components in the range 0 to 1.
type Color struct {
	Space ColorSpace
	C     [4]float64
}

// Black is black in DeviceGray.
var Black = Color{Space: Gray}

// RGBColor builds an RGB color.
func RGBColor(r, g, b float64) Color { return Color{Space: RGB, C: [4]float64{r, g, b}} }

// GrayColor builds a gray level.
func GrayColor(v float64) Color { return Color{Space: Gray, C: [4]float64{v}} }

// Document is a fully laid out document.
type Document struct {
	Pages       []*Page
	Fonts       []FontRef
	Images      []ImageRef
	Attachments []Attachment
	Meta        Metadata
	// Dests maps IDs to their position, for internal links and bookmarks.
	Dests map[string]Dest
	// Bookmarks are the bookmarks (fo:bookmark-tree).
	Bookmarks []Bookmark
	// Inline holds role and details for Tagged PDF of inline elements
	// (spans, fields, links) by template ID. Their glyphs carry the ID,
	// they get a container of their own only with background or border.
	Inline map[string]InlineTag
}

// InlineTag holds role and details of an inline element.
type InlineTag struct {
	Role string
	Tag  Tag
}

// Dest is a jump target: page index (from 0) and point on the page.
type Dest struct {
	Page int
	At   Point
	// Source is the source (Container.Source) of the element at the target, for
	// structure destinations in Tagged PDF. Empty if the target is a page.
	Source string
}

// Bookmark is a bookmark. The target is either Dest (an ID) or URI.
type Bookmark struct {
	Title    string
	Dest     string
	URI      string
	Open     bool
	Children []Bookmark
}

// Metadata is the document metadata.
type Metadata struct {
	Title, Author, Subject, Keywords, Creator string
	// Lang is the document language per BCP 47, such as "de-DE".
	Lang string
	// XMP holds additional XMP schemas. Under PDF/A a pdfaExtension
	// schema is written for each one.
	XMP []XMPSchema
}

// XMPSchema is a custom XMP schema with values.
type XMPSchema struct {
	NamespaceURI string
	Prefix       string
	Description  string
	Properties   []XMPProperty
}

// XMPProperty is a simple text property of an XMP schema.
type XMPProperty struct {
	Name        string
	Value       string
	Description string
	// ValueType is the XMP value type, such as "Text" or "Date". Empty means Text.
	ValueType string
	// Category is "internal" or "external" (default) for the
	// pdfaExtension description.
	Category string
}

// FontID indexes Document.Fonts.
type FontID int

// FontSource is the font behind a FontRef, typically *font.Face.
type FontSource interface {
	PostScriptName() string
}

// FontRef connects a FontID with its font.
type FontRef struct {
	Source FontSource
}

// ImageID indexes Document.Images.
type ImageID int

// ImageSource is the image behind an ImageRef, typically *image.Image.
type ImageSource interface {
	// Key identifies the image uniquely, for example by a content hash.
	Key() string
	// PixelSize returns width and height in pixels.
	PixelSize() (w, h int)
}

// ImageRef connects an ImageID with its image.
type ImageRef struct {
	Source ImageSource
}

// Relationship is the value of /AFRelationship according to ISO 32000-2.
type Relationship string

// Values for Relationship.
const (
	Source           Relationship = "Source"
	Data             Relationship = "Data"
	Alternative      Relationship = "Alternative"
	Supplement       Relationship = "Supplement"
	EncryptedPayload Relationship = "EncryptedPayload"
	FormData         Relationship = "FormData"
	Schema           Relationship = "Schema"
	Unspecified      Relationship = "Unspecified"
)

// Attachment is an embedded file.
type Attachment struct {
	Name         string
	MIMEType     string
	Relationship Relationship
	Description  string
	Data         []byte
}

// Page is a page.
type Page struct {
	// Number is the page number from 1.
	Number int
	Width  Length
	Height Length
	// TrimBox and BleedBox are optional.
	TrimBox  *Rect
	BleedBox *Rect
	Areas    []Area
}

// Area is an area on a page.
type Area interface {
	Bounds() Rect
	isArea()
}

// Kind distinguishes containers.
type Kind uint8

const (
	// KindBlock is a block area.
	KindBlock Kind = iota
	// KindLine is a line area.
	KindLine
	// KindInline is an inline area.
	KindInline
	// KindRegion is a region of the page master.
	KindRegion
	// KindViewport is an area that clips its content.
	KindViewport
)

var kindNames = [...]string{"block", "line", "inline", "region", "viewport"}

func (k Kind) String() string { return kindNames[k] }

// BorderStyle is the line style of a border or a line.
type BorderStyle uint8

const (
	// None draws nothing.
	None BorderStyle = iota
	// Solid draws a solid line.
	Solid
	// Dashed draws a dashed line.
	Dashed
	// Dotted draws a dotted line.
	Dotted
	// Double draws a double line.
	Double
	// Hidden draws nothing like None, but wins against all other styles for
	// collapsing table borders (CSS2 17.6.2.1).
	Hidden
)

var styleNames = [...]string{"none", "solid", "dashed", "dotted", "double", "hidden"}

func (s BorderStyle) String() string { return styleNames[s] }

// Border is a border edge.
type Border struct {
	Width Length
	Style BorderStyle
	Color Color
}

// Visible reports whether the edge is drawn.
func (b Border) Visible() bool { return b.Style != None && b.Style != Hidden && b.Width > 0 }

// Sides for Container.Border.
const (
	Top = iota
	Right
	Bottom
	Left
)

// Container is a block, line, inline, region or viewport area.
// Background and border refer to the rectangle including
// padding, the border lies outside around it.
type Container struct {
	Rect
	Kind       Kind
	Background *Color
	Border     [4]Border
	Children   []Area
	// Role is the semantic role from the template (FO attribute role),
	// the basis of the structure type in the structure tree.
	Role string
	// Tag carries further details for Tagged PDF, nil without such details.
	Tag *Tag
	// Source refers to the origin in the input document, such as "fo:block@12:5".
	Source string
	// ID is the ID of the template element (pdx:id in the FO), inherited from the
	// nearest ancestor. Empty for FO without a template.
	ID string
}

// Tag describes a container for Tagged PDF beyond the role.
type Tag struct {
	// Alt is the alternative text, Lang the language if it differs,
	// Scope the reference of a header cell (Column, Row or Both).
	Alt, Lang, Scope string
	// Order is the position in the reading order among positioned
	// siblings, 0 if not given.
	Order int
	// Artifact removes the area with all children from the structure tree.
	Artifact bool
	// Repeat marks a repetition: table header or footer on a
	// following page or the content of a retrieved marker. Repetitions
	// are artifacts so that screen readers do not read them more than once.
	Repeat bool
	// Row numbers the rows of a table or list per document, cells
	// of the same row carry the same number. Section is header, body,
	// footer or item (list item).
	Row     int
	Section string
	// ColSpan and RowSpan are the spans of a cell, 0 for one column
	// or row.
	ColSpan, RowSpan int
	// Static marks content from fo:static-content (header, footer, page numbers)
	// that counts as an artifact. Content cancels that (artifact="false" in the
	// template), for example for address and invoice data in the header of the first page.
	Static, Content bool
	// Continued marks a fragment that continues a block or a cell from
	// a previous page.
	Continued bool
	// Parent is, for absolutely positioned containers, the source (Source)
	// of the nearest absolutely positioned ancestor. The layout places all
	// positioned containers at page level, Parent preserves the
	// nesting for structure and reading order.
	Parent string
}

// Glyph is a positioned glyph.
type Glyph struct {
	ID uint16
	// Advance is the advance width including kerning.
	Advance Length
	// XOffset and YOffset shift the glyph without changing the advance.
	XOffset, YOffset Length
	// Text is the Unicode text that the glyph represents. For ligatures
	// the first glyph carries the whole cluster, subsequent glyphs stay empty.
	Text string
	// Hyphen marks the hyphen of automatic hyphenation. It
	// does not belong to the text of the word.
	Hyphen bool
}

// GlyphRun is a sequence of glyphs of one font on one baseline.
type GlyphRun struct {
	// Origin is the start of the baseline.
	Origin Point
	Font   FontID
	Size   Length
	Color  Color
	Glyphs []Glyph
	// Ascent and Descent (positive) bound the run vertically.
	Ascent, Descent Length
	// ID is the ID of the template element if it differs from that of the
	// enclosing container, for example for a field in a text. Else empty.
	ID string
	// Link is the key of the fo:basic-link whose text the run
	// belongs to (Link.Key), else empty.
	Link string
	// Decor marks pure decoration such as the dots of an fo:leader. It
	// becomes an artifact in Tagged PDF and is skipped by screen readers.
	Decor bool
}

// Width is the sum of the advances.
func (g *GlyphRun) Width() Length {
	var w Length
	for _, gl := range g.Glyphs {
		w += gl.Advance
	}
	return w
}

// Bounds returns the rectangle from baseline, ascent and descent.
func (g *GlyphRun) Bounds() Rect {
	return Rect{X: g.Origin.X, Y: g.Origin.Y - g.Ascent, W: g.Width(), H: g.Ascent + g.Descent}
}

// Image is a placed image, scaled to the rectangle.
type Image struct {
	Rect
	Image ImageID
	// ID is the ID of the template element as for GlyphRun.
	ID string
}

// Line is a straight line.
type Line struct {
	From, To Point
	Width    Length
	Style    BorderStyle
	Color    Color
}

// Bounds returns the bounding rectangle without line width.
func (l *Line) Bounds() Rect {
	x0, x1 := min(l.From.X, l.To.X), max(l.From.X, l.To.X)
	y0, y1 := min(l.From.Y, l.To.Y), max(l.From.Y, l.To.Y)
	return Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
}

// Box is a filled and optionally stroked rectangle.
type Box struct {
	Rect
	Fill   *Color
	Stroke *Border
}

// Link is a clickable area. The target is either Dest (an ID in the
// document) or URI.
type Link struct {
	Rect
	Dest string
	URI  string
	// Key identifies the fo:basic-link, the glyphs of its text carry
	// the same key (GlyphRun.Link).
	Key string
}

func (*Link) isArea() {}

// Widget is a form field (AcroForm) at a position on the page. Runs
// are the glyphs of the appearance laid out by the layout, absolute on
// the page like all areas.
type Widget struct {
	Rect
	Field Field
	Runs  []*GlyphRun
	// Selected is, for list boxes, the rectangle of the selected row, nil
	// without a selection.
	Selected *Rect
	// ID is the ID of the template element.
	ID string
}

func (*Widget) isArea() {}

// Field describes a form field. Several widgets with the same name
// form one field, for example the radio buttons of a group.
type Field struct {
	// Type is text, checkbox, radio, list, combo or signature.
	Type string
	// Name is the field name from the template, Label the accessible name.
	Name, Label string
	// Value is the preset value, Export for radio buttons the value of
	// this option, OnValue and OffValue the values of a checkbox.
	Value, Export     string
	OnValue, OffValue string
	Options           []FieldOption
	Multiline         bool
	ReadOnly          bool
	Required          bool
	Editable          bool
	MaxLength         int
	// Context is the path of the context node in the data (fn:path) to
	// which the binding of the field belongs.
	Context string
	// Font, size and color for appearance and input.
	Font  FontID
	Size  Length
	Color Color
	// Border and background of the field.
	Border     Border
	Background *Color
}

// FieldOption is a choice value with displayed text.
type FieldOption struct {
	Value, Label string
}

// Path is a set of filled rectangles in one color, such as the modules
// of a barcode. The rectangles are absolute on the page.
type Path struct {
	Rect
	Fill  Color
	Rects []Rect
	// Source describes the content for Dump and the designer, such as "qr:…".
	Source string
	// ID is the ID of the template element as for GlyphRun.
	ID string
}

func (*Path) isArea() {}

func (*Container) isArea() {}
func (*GlyphRun) isArea()  {}
func (*Image) isArea()     {}
func (*Line) isArea()      {}
func (*Box) isArea()       {}

// Walk visits all areas of a page in drawing order. If fn returns
// false, the children of the area are skipped.
func Walk(areas []Area, fn func(Area) bool) {
	for _, a := range areas {
		if !fn(a) {
			continue
		}
		if c, ok := a.(*Container); ok {
			Walk(c.Children, fn)
		}
	}
}
