package fopcompare

// Tolerance defines when pdx and FOP count as equal. The initial values were
// agreed with the client and may only be changed deliberately.
type Tolerance struct {
	// Position is the allowed deviation of lines and blocks in points.
	Position float64
	// DPI is the resolution for the pixel comparison.
	DPI int
	// GrayDelta is the gray value distance (0 to 255) up to which two pixels
	// count as equal.
	GrayDelta int
	// Shift is the allowed displacement of a pixel in pixels.
	Shift int
	// MaxPixelRatio is the highest share of differing pixels per page.
	MaxPixelRatio float64
}

// Default holds the agreed initial values: ±1 pt, 72 dpi, gray value
// distance 64 with 1 px shift, at most 0.5 % differing pixels.
var Default = Tolerance{Position: 1, DPI: 72, GrayDelta: 64, Shift: 1, MaxPixelRatio: 0.005}
