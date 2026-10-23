package pdx

import (
	_ "embed"
	"strings"
)

// License is the text of the Apache License 2.0, under which pdx is licensed.
//
//go:embed LICENSE
var License string

// Notice is the notice of pdx according to section 4 d of the Apache License.
//
//go:embed NOTICE
var Notice string

// ThirdPartyNotices are the license texts of all third-party components of
// pdx and pdx-server, generated with make notices.
//
//go:embed THIRD_PARTY_NOTICES.txt
var ThirdPartyNotices string

// Component is a section of the third-party notices.
type Component struct {
	// Name is the heading, for example "golang.org/x/text v0.43.0".
	Name string `json:"name"`
	// License is the detected license in short form, for example "MIT", empty
	// if unknown.
	License string `json:"license"`
	Text    string `json:"text"`
}

const sectionRule = "========================================================================"

// Components splits third-party notices into their sections. The header
// before the first section is dropped.
func Components(notices string) []Component {
	// On Windows, Git may check out the file with CRLF.
	notices = strings.ReplaceAll(notices, "\r\n", "\n")
	parts := strings.Split(notices, "\n"+sectionRule+"\n")
	var out []Component
	// Each heading is followed by another rule line, then the text.
	for i := 1; i+1 < len(parts); i += 2 {
		name := strings.TrimSpace(parts[i])
		text := strings.TrimSpace(parts[i+1])
		out = append(out, Component{Name: name, License: detectLicense(name, text), Text: text})
	}
	return out
}

// detectLicense detects the license from the heading or the text.
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
