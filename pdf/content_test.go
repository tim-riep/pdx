package pdf

import "testing"

// countOps zerlegt einen Content Stream grob und zählt die Operatoren: alle
// Wörter, die keine Zahl, kein Name und kein Teil eines Arrays oder
// Hex-Strings sind.
func countOps(b []byte) int {
	n := 0
	for i := 0; i < len(b); {
		c := b[i]
		switch {
		case c == ' ' || c == '\n':
			i++
		case c == '[':
			for i < len(b) && b[i] != ']' {
				i++
			}
			i++
		case c == '<':
			for i < len(b) && b[i] != '>' {
				i++
			}
			i++
		case c == '/' || c == '-' || c == '.' || c >= '0' && c <= '9':
			for i < len(b) && b[i] != ' ' && b[i] != '\n' {
				i++
			}
		default:
			for i < len(b) && b[i] != ' ' && b[i] != '\n' {
				i++
			}
			n++
		}
	}
	return n
}

// TestContentOps prüft die Zählung der Operatoren für jede Methode.
func TestContentOps(t *testing.T) {
	var c Content
	c.Save()
	c.Transform(1, 0, 0, 1, 10, 20)
	c.LineWidth(0.5)
	c.Dash([]float64{3, 1}, 0)
	c.Dash(nil, 0)
	c.LineCap(1)
	c.FillRGB(1, 0, 0)
	c.StrokeRGB(0, 0, 1)
	c.FillGray(0.5)
	c.StrokeGray(0)
	c.MoveTo(0, 0)
	c.LineTo(10, 10)
	c.Rect(0, 0, 5, 5)
	c.Fill()
	c.Stroke()
	c.Clip()
	c.ExtGState(Name("G0"))
	c.XObject(Name("Im0"))
	c.BeginText()
	c.Font(Name("F0"), 12)
	c.TextMatrix(1, 0, 0, 1, 0, 0)
	c.TextRise(2)
	c.ShowGlyphs([]uint16{1, 2, 3}, []float64{0, -20, 0})
	c.EndText()
	c.Restore()
	if got, want := c.Ops(), countOps(c.Bytes()); got != want {
		t.Errorf("Ops %d, im Stream %d:\n%s", got, want, c.Bytes())
	}
}
