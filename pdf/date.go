package pdf

import (
	"fmt"
	"time"
)

// Date formats a point in time as a PDF date according to ISO 32000 7.9.4,
// for example D:20260101000000+00'00'. The time zone of the time is kept.
func Date(t time.Time) String {
	_, off := t.Zone()
	sign := byte('+')
	if off < 0 {
		sign = '-'
		off = -off
	}
	return String(fmt.Sprintf("D:%s%c%02d'%02d'", t.Format("20060102150405"), sign, off/3600, off%3600/60))
}
