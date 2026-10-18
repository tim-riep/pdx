// Package verapdf ruft veraPDF auf und wertet den maschinenlesbaren Bericht
// (Format mrr) aus. veraPDF ist die verbindliche Prüfinstanz für die
// Konformität der Ausgabe von pdx.
//
// Version und Docker-Image sind ausschließlich hier konfiguriert.
package verapdf

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	// Version ist die verbindliche veraPDF-Version.
	Version = "1.30.3"
	// DockerImage ist das Release-Image der Version, per Digest gepinnt. Auf
	// ghcr.io gibt es nur Entwicklerstände, das Release liegt auf Docker Hub.
	DockerImage = "docker.io/verapdf/cli:v1.30.3@sha256:d565095c67a083e693aebf7d1a15784007370571053fc1aa5b502665b9a78f9c"
	// DockerPlatform ist die Plattform des Images (nur amd64 verfügbar).
	DockerPlatform = "linux/amd64"
	// EnvRunner wählt den Runner: "local", "docker" oder ein Pfad zur
	// veraPDF-CLI. Ohne Angabe wird eine lokale CLI der richtigen Version
	// bevorzugt, sonst Docker genutzt.
	EnvRunner = "PDX_VERAPDF"
)

// flavours bildet pdx-Profilnamen auf veraPDF-Flavours ab.
var flavours = map[string]string{
	"pdfa-2b": "2b",
	"pdfa-2u": "2u",
	"pdfa-3b": "3b",
	"pdfa-3u": "3u",
	"pdfa-3a": "3a",
	"pdfua-1": "ua1",
	"pdfa-4":  "4",
	"pdfa-4f": "4f",
	"pdfua-2": "ua2",
}

// Flavour liefert den veraPDF-Flavour zu einem einzelnen pdx-Profil.
func Flavour(profile string) (string, error) {
	f, ok := flavours[profile]
	if !ok {
		return "", fmt.Errorf("für Profil %q gibt es keine veraPDF-Prüfung", profile)
	}
	return f, nil
}

// Flavours liefert die veraPDF-Flavours zu einer Profilangabe wie
// "pdfa-3a,pdfua-1", einen je beanspruchtem Standard. veraPDF prüft mit
// --flavour genau einen Flavour je Lauf (docs.verapdf.org/cli/validation),
// also braucht eine Kombination mehrere Läufe.
func Flavours(spec string) ([]string, error) {
	var out []string
	var errs []error
	for _, n := range strings.Split(spec, ",") {
		if n = strings.TrimSpace(n); n == "" {
			continue
		}
		f, err := Flavour(n)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		out = append(out, f)
	}
	if len(out) == 0 && len(errs) == 0 {
		errs = append(errs, fmt.Errorf("kein Profil angegeben"))
	}
	return out, errors.Join(errs...)
}

// Runner prüft eine Datei mit veraPDF.
type Runner interface {
	// Name beschreibt den Runner für Ausgaben.
	Name() string
	// Validate prüft die Datei gegen den Flavour und liefert den Bericht.
	Validate(ctx context.Context, file, flavour string) (*Report, error)
}

// Local nutzt eine installierte veraPDF-CLI.
type Local struct {
	Path string
}

// Name beschreibt den Runner.
func (l Local) Name() string { return "veraPDF lokal (" + l.Path + ")" }

// Validate ruft die lokale CLI auf.
func (l Local) Validate(ctx context.Context, file, flavour string) (*Report, error) {
	abs, err := filepath.Abs(file)
	if err != nil {
		return nil, err
	}
	return run(ctx, exec.CommandContext(ctx, l.Path, "--format", "mrr", "--flavour", flavour, abs))
}

// Docker nutzt das gepinnte Docker-Image.
type Docker struct {
	Image, Platform string
}

// Name beschreibt den Runner.
func (d Docker) Name() string { return "veraPDF in Docker (" + d.Image + ")" }

// Validate startet einen Container, der das Verzeichnis der Datei nur lesend
// eingebunden bekommt.
func (d Docker) Validate(ctx context.Context, file, flavour string) (*Report, error) {
	abs, err := filepath.Abs(file)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(abs); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "docker", "run", "--rm", "--network", "none",
		"--platform", d.Platform,
		"-v", filepath.Dir(abs)+":/data:ro",
		d.Image, "--format", "mrr", "--flavour", flavour, "/data/"+filepath.Base(abs))
	return run(ctx, cmd)
}

// run führt veraPDF aus. veraPDF endet bei nicht konformen Dateien mit
// Exit-Code 1, das ist kein Fehler des Aufrufs. Maßgeblich ist der Bericht.
func run(ctx context.Context, cmd *exec.Cmd) (*Report, error) {
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	runErr := cmd.Run()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	reports, err := ParseMRR(&out)
	if err != nil || len(reports) == 0 {
		msg := strings.TrimSpace(errOut.String())
		if runErr != nil {
			return nil, fmt.Errorf("veraPDF-Aufruf fehlgeschlagen: %v %s", runErr, msg)
		}
		return nil, fmt.Errorf("veraPDF-Bericht nicht lesbar: %v %s", err, msg)
	}
	return reports[0], nil
}

var reVersion = regexp.MustCompile(`veraPDF (\d+\.\d+\.\d+)`)

// Detect wählt einen Runner nach EnvRunner, sonst lokale CLI der richtigen
// Version, sonst Docker. warn erhält Hinweise, etwa auf eine abweichende
// lokale Version, und darf nil sein.
func Detect(ctx context.Context, warn func(string)) (Runner, error) {
	if warn == nil {
		warn = func(string) {}
	}
	docker := Docker{Image: DockerImage, Platform: DockerPlatform}
	switch v := os.Getenv(EnvRunner); v {
	case "docker":
		return docker, nil
	case "", "local":
		path, err := exec.LookPath("verapdf")
		if err == nil {
			if ver, verr := localVersion(ctx, path); verr == nil && ver == Version {
				return Local{Path: path}, nil
			} else if verr == nil {
				warn(fmt.Sprintf("lokale veraPDF-Version %s weicht von %s ab, nutze Docker", ver, Version))
			}
		}
		if v == "local" {
			return nil, fmt.Errorf("keine lokale veraPDF-CLI in Version %s gefunden", Version)
		}
		if _, err := exec.LookPath("docker"); err != nil {
			return nil, errors.New("weder veraPDF " + Version + " noch Docker gefunden. " +
				"Installation: https://verapdf.org/software/ oder Docker mit Image " + DockerImage)
		}
		return docker, nil
	default:
		ver, err := localVersion(ctx, v)
		if err != nil {
			return nil, fmt.Errorf("%s=%s: %w", EnvRunner, v, err)
		}
		if ver != Version {
			warn(fmt.Sprintf("veraPDF unter %s hat Version %s statt %s", v, ver, Version))
		}
		return Local{Path: v}, nil
	}
}

func localVersion(ctx context.Context, path string) (string, error) {
	out, err := exec.CommandContext(ctx, path, "--version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("veraPDF --version: %w", err)
	}
	m := reVersion.FindSubmatch(out)
	if m == nil {
		return "", fmt.Errorf("Version von veraPDF nicht erkennbar")
	}
	return string(m[1]), nil
}
