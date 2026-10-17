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
		// Ausdrücke wie "0.5in + 1mm" (XSL 1.1 5.9) wertet der
		// Ausdrucksauswerter aus. Einzelne Token, die er selbst hierher
		// reicht, enthalten keine Operatoren.
		if strings.ContainsAny(s, " (*+") || strings.LastIndex(s, "-") > 0 {
			return EvalLength(s, fontSize, percentBase, nil)
		}
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

// namedColors sind die 16 Farbnamen aus CSS2, auf die XSL 1.1 5.11 verweist,
// erweitert um die Namen aus CSS3 und SVG 1.1, die auch FOP versteht.
var namedColors = map[string]uint32{
	"aliceblue": 0xf0f8ff, "antiquewhite": 0xfaebd7, "aqua": 0x00ffff, "aquamarine": 0x7fffd4,
	"azure": 0xf0ffff, "beige": 0xf5f5dc, "bisque": 0xffe4c4, "black": 0x000000,
	"blanchedalmond": 0xffebcd, "blue": 0x0000ff, "blueviolet": 0x8a2be2, "brown": 0xa52a2a,
	"burlywood": 0xdeb887, "cadetblue": 0x5f9ea0, "chartreuse": 0x7fff00, "chocolate": 0xd2691e,
	"coral": 0xff7f50, "cornflowerblue": 0x6495ed, "cornsilk": 0xfff8dc, "crimson": 0xdc143c,
	"cyan": 0x00ffff, "darkblue": 0x00008b, "darkcyan": 0x008b8b, "darkgoldenrod": 0xb8860b,
	"darkgray": 0xa9a9a9, "darkgreen": 0x006400, "darkgrey": 0xa9a9a9, "darkkhaki": 0xbdb76b,
	"darkmagenta": 0x8b008b, "darkolivegreen": 0x556b2f, "darkorange": 0xff8c00,
	"darkorchid": 0x9932cc, "darkred": 0x8b0000, "darksalmon": 0xe9967a, "darkseagreen": 0x8fbc8f,
	"darkslateblue": 0x483d8b, "darkslategray": 0x2f4f4f, "darkslategrey": 0x2f4f4f,
	"darkturquoise": 0x00ced1, "darkviolet": 0x9400d3, "deeppink": 0xff1493, "deepskyblue": 0x00bfff,
	"dimgray": 0x696969, "dimgrey": 0x696969, "dodgerblue": 0x1e90ff, "firebrick": 0xb22222,
	"floralwhite": 0xfffaf0, "forestgreen": 0x228b22, "fuchsia": 0xff00ff, "gainsboro": 0xdcdcdc,
	"ghostwhite": 0xf8f8ff, "gold": 0xffd700, "goldenrod": 0xdaa520, "gray": 0x808080,
	"green": 0x008000, "greenyellow": 0xadff2f, "grey": 0x808080, "honeydew": 0xf0fff0,
	"hotpink": 0xff69b4, "indianred": 0xcd5c5c, "indigo": 0x4b0082, "ivory": 0xfffff0,
	"khaki": 0xf0e68c, "lavender": 0xe6e6fa, "lavenderblush": 0xfff0f5, "lawngreen": 0x7cfc00,
	"lemonchiffon": 0xfffacd, "lightblue": 0xadd8e6, "lightcoral": 0xf08080, "lightcyan": 0xe0ffff,
	"lightgoldenrodyellow": 0xfafad2, "lightgray": 0xd3d3d3, "lightgreen": 0x90ee90,
	"lightgrey": 0xd3d3d3, "lightpink": 0xffb6c1, "lightsalmon": 0xffa07a, "lightseagreen": 0x20b2aa,
	"lightskyblue": 0x87cefa, "lightslategray": 0x778899, "lightslategrey": 0x778899,
	"lightsteelblue": 0xb0c4de, "lightyellow": 0xffffe0, "lime": 0x00ff00, "limegreen": 0x32cd32,
	"linen": 0xfaf0e6, "magenta": 0xff00ff, "maroon": 0x800000, "mediumaquamarine": 0x66cdaa,
	"mediumblue": 0x0000cd, "mediumorchid": 0xba55d3, "mediumpurple": 0x9370db,
	"mediumseagreen": 0x3cb371, "mediumslateblue": 0x7b68ee, "mediumspringgreen": 0x00fa9a,
	"mediumturquoise": 0x48d1cc, "mediumvioletred": 0xc71585, "midnightblue": 0x191970,
	"mintcream": 0xf5fffa, "mistyrose": 0xffe4e1, "moccasin": 0xffe4b5, "navajowhite": 0xffdead,
	"navy": 0x000080, "oldlace": 0xfdf5e6, "olive": 0x808000, "olivedrab": 0x6b8e23,
	"orange": 0xffa500, "orangered": 0xff4500, "orchid": 0xda70d6, "palegoldenrod": 0xeee8aa,
	"palegreen": 0x98fb98, "paleturquoise": 0xafeeee, "palevioletred": 0xdb7093,
	"papayawhip": 0xffefd5, "peachpuff": 0xffdab9, "peru": 0xcd853f, "pink": 0xffc0cb,
	"plum": 0xdda0dd, "powderblue": 0xb0e0e6, "purple": 0x800080, "red": 0xff0000,
	"rosybrown": 0xbc8f8f, "royalblue": 0x4169e1, "saddlebrown": 0x8b4513, "salmon": 0xfa8072,
	"sandybrown": 0xf4a460, "seagreen": 0x2e8b57, "seashell": 0xfff5ee, "sienna": 0xa0522d,
	"silver": 0xc0c0c0, "skyblue": 0x87ceeb, "slateblue": 0x6a5acd, "slategray": 0x708090,
	"slategrey": 0x708090, "snow": 0xfffafa, "springgreen": 0x00ff7f, "steelblue": 0x4682b4,
	"tan": 0xd2b48c, "teal": 0x008080, "thistle": 0xd8bfd8, "tomato": 0xff6347, "turquoise": 0x40e0d0,
	"violet": 0xee82ee, "wheat": 0xf5deb3, "white": 0xffffff, "whitesmoke": 0xf5f5f5,
	"yellow": 0xffff00, "yellowgreen": 0x9acd32,
}

// ParseColor liest eine Farbe. transparent liefert ok=false ohne Fehler.
// Unterstützt werden #rgb, #rrggbb, rgb(), rgb-icc() mit RGB-Rückfall und
// die Farbnamen aus CSS3 und SVG 1.1.
func ParseColor(s string) (c areatree.Color, ok bool, err error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "transparent" {
		return c, false, nil
	}
	if n, found := namedColors[s]; found {
		return rgbOrGray(float64(n>>16)/255, float64(n>>8&255)/255, float64(n&255)/255), true, nil
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
