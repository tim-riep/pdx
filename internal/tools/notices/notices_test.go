package main

import (
	"bytes"
	"os"
	"testing"
)

// THIRD_PARTY_NOTICES.txt im Wurzelordner passt zu den gelinkten Modulen.
// Nach einer Änderung der Abhängigkeiten: make notices.
func TestRootNoticesCurrent(t *testing.T) {
	if testing.Short() {
		t.Skip("ruft go list auf")
	}
	got, err := generate([]string{"../../../cmd/pdx", "../../../cmd/pdx-server"}, false, "", "", "pdx")
	if err != nil {
		t.Fatal(err)
	}
	old, err := os.ReadFile("../../../THIRD_PARTY_NOTICES.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, old) {
		t.Error("THIRD_PARTY_NOTICES.txt ist veraltet, bitte make notices ausführen")
	}
}
