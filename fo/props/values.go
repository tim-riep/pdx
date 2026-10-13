// Package props löst XSL-FO-Eigenschaften auf: Längen, Farben, Shorthands,
// Vererbung und die Umrechnung von Rändern in Einzüge.
//
// Unterstützt wird der Teilumfang von Phase 0. Ausdrücke wie "2pt + 3mm"
// sind noch nicht möglich und führen zu einem Fehler mit Position.
package props

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/tim-riep/pdx/areatree"
)

// Length ist eine Länge in Millipunkt.
type Length = areatree.Length

var units = map[string]float64{
	"pt": 1,
	"mm": 72 / 25.4,
	"cm": 72 / 2.54,
	"in": 72,
	"pc": 12,
	"px": 0.75, // CSS-Pixel bei 96 dpi
}

// ParseLength liest eine Länge. em bezieht sich auf fontSize, Prozent auf
// percentBase. Ist percentBase negativ, sind Prozentangaben nicht erlaubt.
func ParseLength(s string, fontSize, percentBase Length) (Length, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("leere Längenangabe")
	}
	if strings.HasSuffix(s, "%") {
		if percentBase < 0 {
			return 0, fmt.Errorf("Prozentangabe %q ist hier nicht erlaubt", s)
		}
		v, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
		if err != nil {
			return 0, fmt.Errorf("ungültige Prozentangabe %q", s)
		}
		return Length(float64(percentBase) * v / 100), nil
	}
	i := len(s)
	for i > 0 && (s[i-1] >= 'a' && s[i-1] <= 'z') {
		i--
	}
	num, unit := s[:i], s[i:]
	v, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0, fmt.Errorf("ungültige Längenangabe %q", s)
	}
	switch unit {
	case "em":
		return Length(v * float64(fontSize)), nil
	case "":
		if v == 0 {
			return 0, nil
		}
		return 0, fmt.Errorf("Längenangabe %q ohne Einheit", s)
	}
	f, ok := units[unit]
	if !ok {
		return 0, fmt.Errorf("unbekannte Einheit %q in %q", unit, s)
	}
	return areatree.Points(v * f), nil
}

var namedColors = map[string][3]float64{
	"black": {0, 0, 0}, "white": {1, 1, 1}, "red": {1, 0, 0}, "green": {0, 128.0 / 255, 0},
	"blue": {0, 0, 1}, "gray": {128.0 / 255, 128.0 / 255, 128.0 / 255}, "grey": {128.0 / 255, 128.0 / 255, 128.0 / 255},
	"silver": {192.0 / 255, 192.0 / 255, 192.0 / 255}, "maroon": {128.0 / 255, 0, 0}, "purple": {128.0 / 255, 0, 128.0 / 255},
	"fuchsia": {1, 0, 1}, "lime": {0, 1, 0}, "olive": {128.0 / 255, 128.0 / 255, 0}, "yellow": {1, 1, 0},
	"navy": {0, 0, 128.0 / 255}, "teal": {0, 128.0 / 255, 128.0 / 255}, "aqua": {0, 1, 1}, "orange": {1, 165.0 / 255, 0},
}

// ParseColor liest eine Farbe. transparent liefert ok=false ohne Fehler.
// Unterstützt werden #rgb, #rrggbb, rgb(), rgb-icc() mit RGB-Rückfall und
// die Farbnamen aus XSL 1.1 sowie orange.
func ParseColor(s string) (c areatree.Color, ok bool, err error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "transparent" {
		return c, false, nil
	}
	if v, found := namedColors[s]; found {
		return rgbOrGray(v[0], v[1], v[2]), true, nil
	}
	if strings.HasPrefix(s, "#") {
		h := s[1:]
		if len(h) == 3 {
			h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
		}
		if len(h) != 6 {
			return c, false, fmt.Errorf("ungültige Farbe %q", s)
		}
		n, err := strconv.ParseUint(h, 16, 32)
		if err != nil {
			return c, false, fmt.Errorf("ungültige Farbe %q", s)
		}
		return rgbOrGray(float64(n>>16)/255, float64(n>>8&255)/255, float64(n&255)/255), true, nil
	}
	for _, fn := range []string{"rgb(", "rgb-icc("} {
		if strings.HasPrefix(s, fn) && strings.HasSuffix(s, ")") {
			parts := strings.Split(s[len(fn):len(s)-1], ",")
			if len(parts) < 3 {
				return c, false, fmt.Errorf("ungültige Farbe %q", s)
			}
			var v [3]float64
			for i := 0; i < 3; i++ {
				p := strings.TrimSpace(parts[i])
				if strings.HasSuffix(p, "%") {
					f, err := strconv.ParseFloat(strings.TrimSuffix(p, "%"), 64)
					if err != nil {
						return c, false, fmt.Errorf("ungültige Farbe %q", s)
					}
					v[i] = f / 100
				} else {
					f, err := strconv.ParseFloat(p, 64)
					if err != nil {
						return c, false, fmt.Errorf("ungültige Farbe %q", s)
					}
					v[i] = f / 255
				}
				v[i] = min(max(v[i], 0), 1)
			}
			return rgbOrGray(v[0], v[1], v[2]), true, nil
		}
	}
	return c, false, fmt.Errorf("unbekannte Farbe %q", s)
}

// rgbOrGray liefert reine Grautöne als DeviceGray, das hält Content Streams
// kurz und schwarzen Text rein schwarz.
func rgbOrGray(r, g, b float64) areatree.Color {
	if r == g && g == b {
		return areatree.GrayColor(round4(r))
	}
	return areatree.RGBColor(round4(r), round4(g), round4(b))
}

func round4(v float64) float64 {
	return float64(int64(v*10000+0.5)) / 10000
}

// splitValues trennt eine Liste an Leerzeichen, ohne Klammerinhalte zu
// zerlegen, etwa "1pt solid rgb(1, 2, 3)".
func splitValues(s string) []string {
	var out []string
	depth := 0
	start := -1
	for i, r := range s {
		switch {
		case r == '(':
			depth++
		case r == ')':
			depth--
		}
		space := r == ' ' || r == '\t' || r == '\n' || r == '\r'
		if space && depth == 0 {
			if start >= 0 {
				out = append(out, s[start:i])
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		out = append(out, s[start:])
	}
	return out
}

// expand4 verteilt 1 bis 4 Werte auf oben, rechts, unten, links wie in CSS.
func expand4(vs []string) ([4]string, error) {
	switch len(vs) {
	case 1:
		return [4]string{vs[0], vs[0], vs[0], vs[0]}, nil
	case 2:
		return [4]string{vs[0], vs[1], vs[0], vs[1]}, nil
	case 3:
		return [4]string{vs[0], vs[1], vs[2], vs[1]}, nil
	case 4:
		return [4]string{vs[0], vs[1], vs[2], vs[3]}, nil
	}
	return [4]string{}, fmt.Errorf("erwartet 1 bis 4 Werte, gefunden %d", len(vs))
}

var borderStyles = map[string]areatree.BorderStyle{
	"none": areatree.None, "hidden": areatree.None, "solid": areatree.Solid, "dashed": areatree.Dashed,
	"dotted": areatree.Dotted, "double": areatree.Double,
	// groove, ridge, inset und outset werden als durchgezogen gezeichnet.
	"groove": areatree.Solid, "ridge": areatree.Solid, "inset": areatree.Solid, "outset": areatree.Solid,
}

var borderWidths = map[string]Length{"thin": 500, "medium": 1000, "thick": 2000}

func parseBorderWidth(s string, fontSize Length) (Length, error) {
	if w, ok := borderWidths[s]; ok {
		return w, nil
	}
	return ParseLength(s, fontSize, -1)
}
