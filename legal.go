package pdx

import (
	_ "embed"
	"strings"
)

// License ist der Text der Apache License 2.0, unter der pdx steht.
//
//go:embed LICENSE
var License string

// Notice ist der Hinweis von pdx nach Abschnitt 4 d der Apache License.
//
//go:embed NOTICE
var Notice string

// ThirdPartyNotices sind die Lizenztexte aller Fremdbestandteile von pdx und
// pdx-server, erzeugt mit make notices.
//
//go:embed THIRD_PARTY_NOTICES.txt
var ThirdPartyNotices string

// Component ist ein Abschnitt der Drittanbieterhinweise.
type Component struct {
	// Name ist die Überschrift, etwa "golang.org/x/text v0.43.0".
	Name string `json:"name"`
	// License ist die erkannte Lizenz in Kurzform, etwa "MIT", leer wenn
	// unbekannt.
	License string `json:"license"`
	Text    string `json:"text"`
}

const sectionRule = "========================================================================"

// Components zerlegt Drittanbieterhinweise in ihre Abschnitte. Der Kopf
// vor dem ersten Abschnitt entfällt.
func Components(notices string) []Component {
	parts := strings.Split(notices, "\n"+sectionRule+"\n")
	var out []Component
	// Auf jede Überschrift folgt eine weitere Linie, dann der Text.
	for i := 1; i+1 < len(parts); i += 2 {
		name := strings.TrimSpace(parts[i])
		text := strings.TrimSpace(parts[i+1])
		out = append(out, Component{Name: name, License: detectLicense(name, text), Text: text})
	}
	return out
}

// detectLicense erkennt die Lizenz an der Überschrift oder am Text.
func detectLicense(name, text string) string {
	if i := strings.LastIndex(name, ", "); i > 0 && strings.HasSuffix(name, ")") && strings.Contains(name, "(npm") {
		return strings.TrimSuffix(name[i+2:], ")")
	}
	head := text
	if len(head) > 3000 {
		head = head[:3000]
	}
	switch {
	case strings.Contains(name, "NOTICE"):
		return "Apache-2.0"
	case strings.Contains(head, "SIL OPEN FONT LICENSE") || strings.Contains(head, "Open Font License"):
		return "OFL-1.1"
	case strings.Contains(name, "hyph-utf8"):
		return "MIT, frei mit Hinweis"
	case strings.Contains(head, "Apache License") && strings.Contains(head, "Version 2.0"):
		return "Apache-2.0"
	case strings.Contains(head, "Mozilla Public License"):
		return "MPL-2.0"
	case strings.Contains(head, "Permission is hereby granted, free of charge"):
		return "MIT"
	case strings.Contains(head, "Redistribution and use in source and binary forms") && strings.Contains(head, "Neither the name"):
		return "BSD-3-Clause"
	case strings.Contains(head, "Redistribution and use in source and binary forms"):
		return "BSD-2-Clause"
	case strings.Contains(head, "Permission to use, copy, modify, and/or distribute"):
		return "ISC"
	}
	return ""
}
