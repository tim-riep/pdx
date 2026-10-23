package pdx

import (
	"strings"
	"testing"
)

// The embedded texts are complete and every section of the third-party
// notices has a recognized license.
func TestLegalTexts(t *testing.T) {
	if !strings.Contains(License, "Apache License") || !strings.Contains(License, "Version 2.0") {
		t.Error("LICENSE ist nicht die Apache License 2.0")
	}
	if !strings.Contains(Notice, "Copyright 2026 Tim Riep") {
		t.Error("NOTICE ohne Copyright")
	}
	cs := Components(ThirdPartyNotices)
	want := map[string]string{
		"pdx (NOTICE)":                         "Apache-2.0",
		"Go (Standardbibliothek und Laufzeit)": "BSD-3-Clause",
		"golang.org/x/text":                    "BSD-3-Clause",
		"github.com/boombuler/barcode":         "MIT",
		"github.com/tim-riep/go-xslt":          "Apache-2.0",
		"software.sslmate.com/src/go-pkcs12":   "BSD-3-Clause",
		"Liberation Fonts":                     "OFL-1.1",
	}
	found := map[string]bool{}
	for _, c := range cs {
		if c.License == "" {
			t.Errorf("%s: Lizenz nicht erkannt", c.Name)
		}
		if len(c.Text) < 200 {
			t.Errorf("%s: Lizenztext fehlt", c.Name)
		}
		for prefix, lic := range want {
			if strings.HasPrefix(c.Name, prefix) {
				found[prefix] = true
				if c.License != lic {
					t.Errorf("%s: %s statt %s", c.Name, c.License, lic)
				}
			}
		}
	}
	for prefix := range want {
		if !found[prefix] {
			t.Errorf("Abschnitt %s fehlt", prefix)
		}
	}
	if !strings.Contains(ThirdPartyNotices, "Reserved Font Name") && !strings.Contains(ThirdPartyNotices, "SIL OPEN FONT LICENSE Version 1.1") {
		t.Error("voller Text der OFL fehlt")
	}
}

// Notices checked out with CRLF yield the same sections.
func TestComponentsCRLF(t *testing.T) {
	lf := Components(ThirdPartyNotices)
	crlf := Components(strings.ReplaceAll(ThirdPartyNotices, "\n", "\r\n"))
	if len(lf) == 0 || len(crlf) != len(lf) || crlf[0].License != lf[0].License {
		t.Errorf("%d Abschnitte mit LF, %d mit CRLF", len(lf), len(crlf))
	}
}
