// Package areatree definiert den Area Tree von pdx: Seiten mit absolut
// positionierten Bereichen, Glyph-Runs, Bildern, Linien und Rechtecken.
//
// Der Area Tree ist das stabile Zwischenformat zwischen Layout und Ausgabe.
// Der PDF-Writer, die spätere Canvas-Vorschau im Designer und Rastertests
// lesen ihn. Er hängt deshalb von keinem anderen pdx-Paket ab.
//
// Alle Längen sind ganze Millipunkt (1/1000 pt). Koordinaten sind absolut auf
// der Seite, Ursprung oben links, y wächst nach unten. Die Verschachtelung von
// Bereichen bildet nur die Struktur ab (für Auswahl im Designer und später für
// Tagging), nicht die Lage.
package areatree

import "strconv"

// Length ist eine Länge in Millipunkt.
type Length int64

// Pt ist ein Punkt (1/72 Zoll).
const Pt Length = 1000

// Points rechnet Punkt in Length um und rundet kaufmännisch.
func Points(f float64) Length {
	if f < 0 {
		return -Length(-f*1000 + 0.5)
	}
	return Length(f*1000 + 0.5)
}

// Pt liefert die Länge in Punkt.
func (l Length) Pt() float64 { return float64(l) / 1000 }

// String formatiert die Länge in Punkt ohne abschließende Nullen.
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

// Point ist ein Punkt auf der Seite.
type Point struct{ X, Y Length }

// Rect ist ein Rechteck mit linker oberer Ecke X, Y.
type Rect struct{ X, Y, W, H Length }

// Bounds liefert das Rechteck selbst und erfüllt so einen Teil von Area.
func (r Rect) Bounds() Rect { return r }

// ColorSpace ist der Farbraum einer Farbe.
type ColorSpace uint8

const (
	// Gray ist DeviceGray, eine Komponente.
	Gray ColorSpace = iota
	// RGB ist DeviceRGB, drei Komponenten. Unter PDF/A gilt das Output Intent.
	RGB
	// CMYK ist DeviceCMYK, vier Komponenten. In Phase 0 nicht ausgegeben.
	CMYK
)

// Color ist eine Farbe mit Komponenten im Bereich 0 bis 1.
type Color struct {
	Space ColorSpace
	C     [4]float64
}

// Black ist Schwarz in DeviceGray.
var Black = Color{Space: Gray}

// RGBColor baut eine RGB-Farbe.
func RGBColor(r, g, b float64) Color { return Color{Space: RGB, C: [4]float64{r, g, b}} }

// GrayColor baut eine Graustufe.
func GrayColor(v float64) Color { return Color{Space: Gray, C: [4]float64{v}} }

// Document ist ein vollständig gesetztes Dokument.
type Document struct {
	Pages       []*Page
	Fonts       []FontRef
	Images      []ImageRef
	Attachments []Attachment
	Meta        Metadata
	// Dests ordnet IDs ihre Lage zu, für interne Verweise und Lesezeichen.
	Dests map[string]Dest
	// Bookmarks sind die Lesezeichen (fo:bookmark-tree).
	Bookmarks []Bookmark
}

// Dest ist ein Sprungziel: Seitenindex (ab 0) und Punkt auf der Seite.
type Dest struct {
	Page int
	At   Point
}

// Bookmark ist ein Lesezeichen. Ziel ist entweder Dest (eine ID) oder URI.
type Bookmark struct {
	Title    string
	Dest     string
	URI      string
	Open     bool
	Children []Bookmark
}

// Metadata sind die Dokumentmetadaten.
type Metadata struct {
	Title, Author, Subject, Keywords, Creator string
	// Lang ist die Dokumentsprache nach BCP 47, etwa "de-DE".
	Lang string
	// XMP sind zusätzliche XMP-Schemata. Unter PDF/A wird für jedes ein
	// pdfaExtension-Schema geschrieben.
	XMP []XMPSchema
}

// XMPSchema ist ein eigenes XMP-Schema mit Werten.
type XMPSchema struct {
	NamespaceURI string
	Prefix       string
	Description  string
	Properties   []XMPProperty
}

// XMPProperty ist eine einfache Texteigenschaft eines XMP-Schemas.
type XMPProperty struct {
	Name        string
	Value       string
	Description string
	// ValueType ist der XMP-Werttyp, etwa "Text" oder "Date". Leer heißt Text.
	ValueType string
}

// FontID indiziert Document.Fonts.
type FontID int

// FontSource ist die Schrift hinter einer FontRef, typischerweise *font.Face.
type FontSource interface {
	PostScriptName() string
}

// FontRef verbindet eine FontID mit ihrer Schrift.
type FontRef struct {
	Source FontSource
}

// ImageID indiziert Document.Images.
type ImageID int

// ImageSource ist das Bild hinter einer ImageRef, typischerweise *image.Image.
type ImageSource interface {
	// Key identifiziert das Bild eindeutig, etwa über einen Inhaltshash.
	Key() string
	// PixelSize liefert Breite und Höhe in Pixel.
	PixelSize() (w, h int)
}

// ImageRef verbindet eine ImageID mit ihrem Bild.
type ImageRef struct {
	Source ImageSource
}

// Relationship ist der Wert von /AFRelationship nach ISO 32000-2.
type Relationship string

// Werte für Relationship.
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

// Attachment ist eine eingebettete Datei.
type Attachment struct {
	Name         string
	MIMEType     string
	Relationship Relationship
	Description  string
	Data         []byte
}

// Page ist eine Seite.
type Page struct {
	// Number ist die Seitenzahl ab 1.
	Number int
	Width  Length
	Height Length
	// TrimBox und BleedBox sind optional.
	TrimBox  *Rect
	BleedBox *Rect
	Areas    []Area
}

// Area ist ein Bereich auf einer Seite.
type Area interface {
	Bounds() Rect
	isArea()
}

// Kind unterscheidet Container.
type Kind uint8

const (
	// KindBlock ist ein Blockbereich.
	KindBlock Kind = iota
	// KindLine ist ein Zeilenbereich.
	KindLine
	// KindInline ist ein Inlinebereich.
	KindInline
	// KindRegion ist eine Region der Seitenvorlage.
	KindRegion
	// KindViewport ist ein Bereich, der seinen Inhalt beschneidet.
	KindViewport
)

var kindNames = [...]string{"block", "line", "inline", "region", "viewport"}

func (k Kind) String() string { return kindNames[k] }

// BorderStyle ist der Linienstil eines Rahmens oder einer Linie.
type BorderStyle uint8

const (
	// None zeichnet nichts.
	None BorderStyle = iota
	// Solid zeichnet eine durchgezogene Linie.
	Solid
	// Dashed zeichnet eine gestrichelte Linie.
	Dashed
	// Dotted zeichnet eine gepunktete Linie.
	Dotted
	// Double zeichnet eine Doppellinie.
	Double
)

var styleNames = [...]string{"none", "solid", "dashed", "dotted", "double"}

func (s BorderStyle) String() string { return styleNames[s] }

// Border ist eine Rahmenkante.
type Border struct {
	Width Length
	Style BorderStyle
	Color Color
}

// Visible meldet, ob die Kante gezeichnet wird.
func (b Border) Visible() bool { return b.Style != None && b.Width > 0 }

// Seiten für Container.Border.
const (
	Top = iota
	Right
	Bottom
	Left
)

// Container ist ein Block-, Zeilen-, Inline-, Regions- oder Viewport-Bereich.
// Hintergrund und Rahmen beziehen sich auf das Rechteck einschließlich
// Padding, der Rahmen liegt außen darum.
type Container struct {
	Rect
	Kind       Kind
	Background *Color
	Border     [4]Border
	Children   []Area
	// Role ist für Tagging in Phase 3 vorgesehen und bleibt vorerst leer.
	Role string
	// Source verweist auf den Ursprung im Eingabedokument, etwa "fo:block@12:5".
	Source string
}

// Glyph ist eine positionierte Glyphe.
type Glyph struct {
	ID uint16
	// Advance ist die Vorschubbreite einschließlich Kerning.
	Advance Length
	// XOffset und YOffset verschieben die Glyphe ohne den Vorschub zu ändern.
	XOffset, YOffset Length
	// Text ist der Unicode-Text, den die Glyphe darstellt. Bei Ligaturen
	// trägt die erste Glyphe den ganzen Cluster, Folgeglyphen bleiben leer.
	Text string
}

// GlyphRun ist eine Folge von Glyphen einer Schrift auf einer Grundlinie.
type GlyphRun struct {
	// Origin ist der Anfang der Grundlinie.
	Origin Point
	Font   FontID
	Size   Length
	Color  Color
	Glyphs []Glyph
	// Ascent und Descent (positiv) begrenzen den Run vertikal.
	Ascent, Descent Length
}

// Width ist die Summe der Vorschübe.
func (g *GlyphRun) Width() Length {
	var w Length
	for _, gl := range g.Glyphs {
		w += gl.Advance
	}
	return w
}

// Bounds liefert das Rechteck aus Grundlinie, Ascent und Descent.
func (g *GlyphRun) Bounds() Rect {
	return Rect{X: g.Origin.X, Y: g.Origin.Y - g.Ascent, W: g.Width(), H: g.Ascent + g.Descent}
}

// Image ist ein platziertes Bild, skaliert auf das Rechteck.
type Image struct {
	Rect
	Image ImageID
}

// Line ist eine gerade Linie.
type Line struct {
	From, To Point
	Width    Length
	Style    BorderStyle
	Color    Color
}

// Bounds liefert das umschließende Rechteck ohne Linienbreite.
func (l *Line) Bounds() Rect {
	x0, x1 := min(l.From.X, l.To.X), max(l.From.X, l.To.X)
	y0, y1 := min(l.From.Y, l.To.Y), max(l.From.Y, l.To.Y)
	return Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
}

// Box ist ein gefülltes und optional umrandetes Rechteck.
type Box struct {
	Rect
	Fill   *Color
	Stroke *Border
}

// Link ist ein anklickbarer Bereich. Ziel ist entweder Dest (eine ID im
// Dokument) oder URI.
type Link struct {
	Rect
	Dest string
	URI  string
}

func (*Link) isArea() {}

func (*Container) isArea() {}
func (*GlyphRun) isArea()  {}
func (*Image) isArea()     {}
func (*Line) isArea()      {}
func (*Box) isArea()       {}

// Walk besucht alle Bereiche einer Seite in Zeichenreihenfolge. Liefert fn
// false, werden die Kinder des Bereichs übersprungen.
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
