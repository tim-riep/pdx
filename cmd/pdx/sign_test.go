package main

import (
	"bytes"
	"crypto/x509"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"software.sslmate.com/src/go-pkcs12"

	"github.com/tim-riep/pdx/internal/testpki"
)

// pdx sign liest die PKCS#12-Datei mit Passwort aus der Umgebung und
// signiert mit Zeitstempel. Das Passwort steht nie in der Ausgabe.
func TestSignCLI(t *testing.T) {
	pki := testpki.New(t)
	dir := t.TempDir()
	in := filepath.Join(dir, "r.pdf")
	var so, se bytes.Buffer
	if code := run([]string{"render", "../../testdata/fo/brief.fo", "-o", in, "--profile", "pdfa-3b", "--creation-date", "2026-01-01T00:00:00Z"}, &so, &se); code != 0 {
		t.Fatalf("render: %s", se.String())
	}
	p12, err := pkcs12.Modern.Encode(pki.Signer.Key, pki.Signer.Cert, []*x509.Certificate{pki.Inter.Cert}, "Sehr-Geheim-42")
	if err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(dir, "schluessel.p12")
	os.WriteFile(key, p12, 0o600)
	out := filepath.Join(dir, "s.pdf")
	args := []string{"sign", in, "-o", out, "--signer", "pkcs12:" + key, "--level", "B-T", "--tsa", pki.TSAURL(), "--reason", "Test"}

	t.Setenv(passwordEnv, "falsch")
	so.Reset()
	se.Reset()
	if code := run(args, &so, &se); code != 2 || !strings.Contains(se.String(), "PKCS#12") {
		t.Errorf("falsches Passwort: Exit-Code %d, %s", code, se.String())
	}
	t.Setenv(passwordEnv, "Sehr-Geheim-42")
	so.Reset()
	se.Reset()
	if code := run(args, &so, &se); code != 0 {
		t.Fatalf("Exit-Code %d: %s", code, se.String())
	}
	if strings.Contains(so.String()+se.String(), "Sehr-Geheim-42") {
		t.Error("Passwort in der Ausgabe")
	}
	a, _ := os.ReadFile(in)
	b, _ := os.ReadFile(out)
	if !bytes.HasPrefix(b, a) || !bytes.Contains(b[len(a):], []byte("/SubFilter /ETSI.CAdES.detached")) {
		t.Error("keine Signatur im inkrementellen Update")
	}
	if code := run([]string{"sign", in, "--signer", "hsm:x"}, &so, &se); code != 2 {
		t.Errorf("unbekannte Signer-Art: Exit-Code %d", code)
	}
}
