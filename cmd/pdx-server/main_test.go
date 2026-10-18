package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/tim-riep/pdx/server"
)

func TestHashKey(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"hash-key"}, strings.NewReader("ein-langer-schluessel\n"), &out, &errb); code != 0 {
		t.Fatal(errb.String())
	}
	if strings.TrimSpace(out.String()) != server.HashKey("ein-langer-schluessel") {
		t.Error("falscher Hash")
	}
	if code := run([]string{"hash-key"}, strings.NewReader("kurz\n"), &out, &errb); code != 2 {
		t.Error("kurzer Schlüssel angenommen")
	}
	if code := run(nil, strings.NewReader(""), &out, &errb); code != 2 {
		t.Error("ohne Konfiguration gestartet")
	}
}
