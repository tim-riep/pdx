// Package notices enthält die Drittanbieterhinweise des Designers. Die
// Datei erzeugt make notices, die App zeigt sie unter „Über“.
package notices

import _ "embed"

// Text sind die Hinweise mit allen Lizenztexten.
//
//go:embed THIRD_PARTY_NOTICES.txt
var Text string
