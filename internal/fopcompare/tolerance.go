package fopcompare

// Tolerance legt fest, wann pdx und FOP als gleich gelten. Die Startwerte
// sind mit dem Auftraggeber abgestimmt und dürfen nur bewusst geändert werden.
type Tolerance struct {
	// Position ist die erlaubte Abweichung von Zeilen und Blöcken in Punkt.
	Position float64
	// DPI ist die Auflösung für den Pixelvergleich.
	DPI int
	// GrayDelta ist der Grauwertabstand (0 bis 255), bis zu dem zwei Pixel
	// als gleich gelten.
	GrayDelta int
	// Shift ist die erlaubte Verschiebung eines Pixels in Pixeln.
	Shift int
	// MaxPixelRatio ist der höchste Anteil abweichender Pixel je Seite.
	MaxPixelRatio float64
}

// Default sind die vereinbarten Startwerte: ±1 pt, 72 dpi, Grauwertabstand
// 64 mit 1 px Verschiebung, höchstens 0,5 % abweichende Pixel.
var Default = Tolerance{Position: 1, DPI: 72, GrayDelta: 64, Shift: 1, MaxPixelRatio: 0.005}
