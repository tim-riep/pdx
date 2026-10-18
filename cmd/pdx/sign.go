package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tim-riep/pdx/sign"
)

// passwordEnv ist die Umgebungsvariable für das Passwort der Schlüsseldatei.
// Als Kommandozeilenargument wäre es in der Prozessliste sichtbar.
const passwordEnv = "PDX_SIGN_PASSWORD"

// tokenEnv ist die Umgebungsvariable für den Token eines Signaturdienstes.
const tokenEnv = "PDX_SIGN_TOKEN"

// cmdSign signiert ein fertiges PDF nach PAdES per inkrementellem Update.
func cmdSign(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("sign", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("o", "", "Ausgabedatei (Vorgabe: Eingabe mit Endung -signiert.pdf)")
	level := fs.String("level", string(sign.LevelB), "PAdES-Stufe: B-B, B-T, B-LT oder B-LTA")
	signer := fs.String("signer", "", "Schlüssel: pkcs12:datei.p12 (Passwort aus "+passwordEnv+" oder Abfrage) oder remote:https://dienst (Token aus "+tokenEnv+")")
	tsa := fs.String("tsa", "", "Adresse des Zeitstempeldienstes nach RFC 3161, Pflicht ab B-T")
	field := fs.String("field", "", "vorhandenes Signaturfeld (Vorgabe: neues unsichtbares Feld)")
	reason := fs.String("reason", "", "Grund der Signatur")
	location := fs.String("location", "", "Ort der Signatur")
	contact := fs.String("contact", "", "Kontaktangabe des Unterzeichners")
	pos, err := parse(fs, args)
	if err != nil {
		return 2
	}
	if len(pos) != 1 || *signer == "" {
		fmt.Fprintln(stderr, "pdx sign: genau eine PDF-Datei und --signer erwartet")
		return 2
	}
	in, err := os.ReadFile(pos[0])
	if err != nil {
		fmt.Fprintf(stderr, "pdx sign: %v\n", err)
		return 2
	}
	s, err := loadSigner(*signer, os.Stdin, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "pdx sign: %v\n", err)
		return 2
	}
	opt := sign.PDFOptions{Level: sign.Level(*level), Field: *field, Reason: *reason, Location: *location, ContactInfo: *contact}
	if *tsa != "" {
		opt.TSA = &sign.TSA{URL: *tsa}
	}
	signed, err := sign.SignPDF(context.Background(), in, s, opt)
	if err != nil {
		printErr(stderr, "sign", err)
		return 1
	}
	if *out == "" {
		*out = strings.TrimSuffix(pos[0], filepath.Ext(pos[0])) + "-signiert.pdf"
	}
	if err := os.WriteFile(*out, signed, 0o644); err != nil {
		fmt.Fprintf(stderr, "pdx sign: %v\n", err)
		return 2
	}
	fmt.Fprintf(stdout, "%s: signiert mit PAdES %s\n", *out, opt.Level)
	return 0
}

// loadSigner liest einen Signer aus der Angabe ART:ORT.
func loadSigner(spec string, stdin *os.File, stderr io.Writer) (sign.Signer, error) {
	kind, loc, ok := strings.Cut(spec, ":")
	if !ok || loc == "" {
		return nil, fmt.Errorf("Signer %q hat nicht die Form ART:ORT, etwa pkcs12:datei.p12", spec)
	}
	switch kind {
	case "pkcs12":
		data, err := os.ReadFile(loc)
		if err != nil {
			return nil, err
		}
		pw, err := password(stdin, stderr)
		if err != nil {
			return nil, err
		}
		return sign.LoadPKCS12(data, pw)
	case "remote":
		return sign.NewRemoteSigner(context.Background(), loc, os.Getenv(tokenEnv), nil)
	}
	return nil, fmt.Errorf("Signer-Art %q ist unbekannt, erlaubt sind pkcs12 und remote", kind)
}

// password liest das Passwort aus der Umgebung oder fragt es am Terminal
// ohne Echo ab.
func password(stdin *os.File, stderr io.Writer) (string, error) {
	if pw, ok := os.LookupEnv(passwordEnv); ok {
		return pw, nil
	}
	if fi, err := stdin.Stat(); err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return "", errors.New("kein Passwort: " + passwordEnv + " setzen oder am Terminal aufrufen")
	}
	echo := func(on bool) error {
		arg := "-echo"
		if on {
			arg = "echo"
		}
		cmd := exec.Command("stty", arg)
		cmd.Stdin = stdin
		return cmd.Run()
	}
	if err := echo(false); err != nil {
		return "", errors.New("Abfrage ohne Echo nicht möglich, " + passwordEnv + " setzen")
	}
	defer func() {
		echo(true)
		fmt.Fprintln(stderr)
	}()
	fmt.Fprint(stderr, "Passwort der Schlüsseldatei: ")
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}
