package pdf

import (
	"fmt"
	"time"
)

// Date formatiert einen Zeitpunkt als PDF-Datum nach ISO 32000 7.9.4,
// etwa D:20260101000000+00'00'. Die Zeitzone des Zeitpunkts bleibt erhalten.
func Date(t time.Time) String {
	_, off := t.Zone()
	sign := byte('+')
	if off < 0 {
		sign = '-'
		off = -off
	}
	return String(fmt.Sprintf("D:%s%c%02d'%02d'", t.Format("20060102150405"), sign, off/3600, off%3600/60))
}
