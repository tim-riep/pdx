package app

import (
	"strings"
	"testing"

	"github.com/tim-riep/pdx"
)

func TestInfo(t *testing.T) {
	i := (&AppService{}).Info()
	if i.Version != Version || i.Engine != pdx.Version || i.Name == "" {
		t.Errorf("Info: %+v", i)
	}
}

// The Licenses dialog receives the Apache License and every third-party
// component of the designer with a detected license and full text, including
// the embedded fonts and the npm packages.
func TestLicenses(t *testing.T) {
	l := (&AppService{}).Licenses()
	if !strings.Contains(l.License, "Apache License") || !strings.Contains(l.Notice, "Tim Riep") {
		t.Error("eigene Lizenz fehlt")
	}
	names := map[string]string{}
	for _, c := range l.Components {
		if c.License == "" || len(c.Text) < 200 {
			t.Errorf("%s: Lizenz %q, %d Zeichen Text", c.Name, c.License, len(c.Text))
		}
		names[strings.Fields(c.Name)[0]] = c.License
	}
	for name, lic := range map[string]string{
		"github.com/wailsapp/wails/v3": "MIT",
		"svelte":                       "MIT",
		"pdfjs-dist":                   "Apache-2.0",
		"Liberation":                   "OFL-1.1",
		"golang.org/x/text":            "BSD-3-Clause",
	} {
		if names[name] != lic {
			t.Errorf("%s: %q statt %q", name, names[name], lic)
		}
	}
}
