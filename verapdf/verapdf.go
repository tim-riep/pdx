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
	"runtime"
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
	// Bin ist der Pfad zur Docker-CLI, leer bedeutet docker aus dem PATH.
	Bin string
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
	bin := d.Bin
	if bin == "" {
		bin = "docker"
	}
	cmd := exec.CommandContext(ctx, bin, "run", "--rm", "--network", "none",
		"--platform", d.Platform,
		"-v", filepath.Dir(abs)+":/data:ro",
		d.Image, "--format", "mrr", "--flavour", flavour, "/data/"+filepath.Base(abs))
	if d.Bin != "" {
		// Hilfsprogramme von Docker Desktop (Anmeldedaten) liegen neben der
		// CLI und müssen im PATH stehen, auch wenn die CLI dort fehlt.
		cmd.Env = withPath(filepath.Dir(d.Bin))
	}
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

// Options steuert DetectWith.
type Options struct {
	// Path ist ein ausdrücklich gewählter Pfad zur veraPDF-CLI, etwa aus den
	// Einstellungen des Designers. Er geht EnvRunner vor.
	Path string
}

// Detect wählt einen Runner nach EnvRunner, sonst lokale CLI der richtigen
// Version, sonst Docker. warn erhält Hinweise, etwa auf eine abweichende
// lokale Version, und darf nil sein.
func Detect(ctx context.Context, warn func(string)) (Runner, error) {
	return DetectWith(ctx, Options{}, warn)
}

// DetectWith wählt einen Runner: zuerst opts.Path, dann EnvRunner, sonst
// automatisch. Automatisch sucht es veraPDF und Docker im PATH und in den
// üblichen Installationsordnern, denn Programme aus Finder, Launchpad oder
// dem Startmenü sehen den PATH der Shell nicht.
func DetectWith(ctx context.Context, opts Options, warn func(string)) (Runner, error) {
	if warn == nil {
		warn = func(string) {}
	}
	if opts.Path != "" {
		return explicit(ctx, opts.Path, "Pfad zu veraPDF", warn)
	}
	switch v := os.Getenv(EnvRunner); v {
	case "docker":
		bin, ok := findDocker()
		if !ok {
			return nil, fmt.Errorf("%s=docker, aber Docker ist nicht installiert", EnvRunner)
		}
		return Docker{Image: DockerImage, Platform: DockerPlatform, Bin: bin}, nil
	case "", "local":
		for _, path := range verapdfCandidates() {
			ver, err := localVersion(ctx, path)
			if err != nil {
				continue
			}
			if ver == Version {
				return Local{Path: path}, nil
			}
			warn(fmt.Sprintf("veraPDF unter %s hat Version %s statt %s, nicht genutzt", path, ver, Version))
		}
		if v == "local" {
			return nil, fmt.Errorf("keine lokale veraPDF-CLI in Version %s gefunden", Version)
		}
		bin, ok := findDocker()
		if !ok {
			return nil, errors.New("weder veraPDF " + Version + " noch Docker gefunden. " +
				"Installation: https://verapdf.org/software/ oder Docker mit Image " + DockerImage)
		}
		return Docker{Image: DockerImage, Platform: DockerPlatform, Bin: bin}, nil
	default:
		return explicit(ctx, v, EnvRunner+"="+v, warn)
	}
}

// explicit nutzt eine ausdrücklich genannte CLI, eine andere Version ergibt
// nur einen Hinweis.
func explicit(ctx context.Context, path, what string, warn func(string)) (Runner, error) {
	ver, err := localVersion(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	if ver != Version {
		warn(fmt.Sprintf("veraPDF unter %s hat Version %s statt %s", path, ver, Version))
	}
	return Local{Path: path}, nil
}

// searchDirs liefert die Ordner, in denen Installationen außerhalb des PATH
// liegen. In Tests ersetzbar.
var searchDirs = func(prog string) []string {
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "windows" {
		pf := os.Getenv("ProgramFiles")
		switch prog {
		case "verapdf":
			return []string{filepath.Join(home, "verapdf"), filepath.Join(os.Getenv("LOCALAPPDATA"), "verapdf"), filepath.Join(pf, "verapdf"), filepath.Join(pf, "veraPDF")}
		case "docker":
			return []string{filepath.Join(pf, "Docker", "Docker", "resources", "bin")}
		}
		return nil
	}
	switch prog {
	case "verapdf":
		// Der Installer von veraPDF legt die CLI unter ~/verapdf ab.
		return []string{filepath.Join(home, "verapdf"), "/usr/local/bin", "/opt/homebrew/bin", "/usr/local/verapdf", "/opt/verapdf"}
	case "docker":
		return []string{"/usr/local/bin", "/opt/homebrew/bin", filepath.Join(home, ".docker", "bin"), "/Applications/Docker.app/Contents/Resources/bin", "/usr/bin"}
	}
	return nil
}

// candidates liefert ausführbare Dateien eines Programms: zuerst aus dem
// PATH, dann aus searchDirs, ohne Doppelte.
func candidates(prog string) []string {
	names := []string{prog}
	if runtime.GOOS == "windows" {
		names = []string{prog + ".bat", prog + ".cmd", prog + ".exe"}
	}
	var out []string
	seen := map[string]bool{}
	add := func(p string) {
		if abs, err := filepath.Abs(p); err == nil && !seen[abs] {
			if st, err := os.Stat(abs); err == nil && !st.IsDir() && (runtime.GOOS == "windows" || st.Mode()&0o111 != 0) {
				seen[abs] = true
				out = append(out, abs)
			}
		}
	}
	for _, n := range names {
		if p, err := exec.LookPath(n); err == nil {
			add(p)
		}
	}
	for _, dir := range searchDirs(prog) {
		if dir == "" {
			continue
		}
		for _, n := range names {
			add(filepath.Join(dir, n))
		}
	}
	return out
}

func verapdfCandidates() []string { return candidates("verapdf") }

func findDocker() (string, bool) {
	c := candidates("docker")
	if len(c) == 0 {
		return "", false
	}
	return c[0], true
}

// withPath liefert die Umgebung mit dir vorn im PATH.
func withPath(dir string) []string {
	env := os.Environ()
	for i, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && strings.EqualFold(k, "PATH") {
			env[i] = k + "=" + dir + string(os.PathListSeparator) + v
			return env
		}
	}
	return append(env, "PATH="+dir)
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
