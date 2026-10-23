package main

import (
	"os"
	"path/filepath"
	"testing"
)

// --attach optionally takes a description as the fourth part.
func TestParseAttach(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "daten.xml")
	os.WriteFile(f, []byte("<x/>"), 0o644)
	a, err := parseAttach(f + ":Data:text/xml:Rechnungsdaten")
	if err != nil || a.Description != "Rechnungsdaten" || a.MIMEType != "text/xml" || a.Relationship != "Data" || a.Name != "daten.xml" {
		t.Errorf("%+v %v", a, err)
	}
	a, err = parseAttach(f + ":Alternative:application/xml")
	if err != nil || a.Description != "" || a.MIMEType != "application/xml" {
		t.Errorf("%+v %v", a, err)
	}
	if _, err := parseAttach(f + ":Data"); err == nil {
		t.Error("unvollständige Angabe angenommen")
	}
}
