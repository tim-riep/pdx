// Package verapdf invokes veraPDF and evaluates the machine-readable report
// (mrr format). veraPDF is the authoritative validator for the
// conformance of the output of pdx.
//
// Version and Docker image are configured exclusively here.
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
	// Version is the authoritative veraPDF version.
	Version = "1.30.3"
	// DockerImage is the release image of the version, pinned by digest. On
	// ghcr.io there are only development builds, the release is on Docker Hub.
	DockerImage = "docker.io/verapdf/cli:v1.30.3@sha256:d565095c67a083e693aebf7d1a15784007370571053fc1aa5b502665b9a78f9c"
	// DockerPlatform is the platform of the image (only amd64 available).
	DockerPlatform = "linux/amd64"
	// EnvRunner selects the runner: "local", "docker" or a path to the
	// veraPDF CLI. If unset, a local CLI of the right version is
	// preferred, otherwise Docker is used.
	EnvRunner = "PDX_VERAPDF"
)

// flavours maps pdx profile names to veraPDF flavours.
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

// Flavour returns the veraPDF flavour for a single pdx profile.
func Flavour(profile string) (string, error) {
	f, ok := flavours[profile]
	if !ok {
		return "", fmt.Errorf("für Profil %q gibt es keine veraPDF-Prüfung", profile)
	}
	return f, nil
}

// Flavours returns the veraPDF flavours for a profile specification such as
// "pdfa-3a,pdfua-1", one per claimed standard. veraPDF checks exactly one
// flavour per run with --flavour (docs.verapdf.org/cli/validation), so a
// combination needs several runs.
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

// Runner checks a file with veraPDF.
type Runner interface {
	// Name describes the runner for output.
	Name() string
	// Validate checks the file against the flavour and returns the report.
	Validate(ctx context.Context, file, flavour string) (*Report, error)
}

// Local uses an installed veraPDF CLI.
type Local struct {
	Path string
}

// Name describes the runner.
func (l Local) Name() string { return "veraPDF lokal (" + l.Path + ")" }

// Validate invokes the local CLI.
func (l Local) Validate(ctx context.Context, file, flavour string) (*Report, error) {
	abs, err := filepath.Abs(file)
	if err != nil {
		return nil, err
	}
	return run(ctx, exec.CommandContext(ctx, l.Path, "--format", "mrr", "--flavour", flavour, abs))
}

// Docker uses the pinned Docker image.
type Docker struct {
	Image, Platform string
	// Bin is the path to the Docker CLI, empty means docker from the PATH.
	Bin string
}

// Name describes the runner.
func (d Docker) Name() string { return "veraPDF in Docker (" + d.Image + ")" }

// Validate starts a container that gets the directory of the file mounted
// read-only.
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
		// Docker Desktop helper programs (credentials) live next to the CLI
		// and must be in the PATH, even if the CLI is missing there.
		cmd.Env = withPath(filepath.Dir(d.Bin))
	}
	return run(ctx, cmd)
}

// run executes veraPDF. veraPDF exits with exit code 1 for non-conforming
// files, which is not an error of the call. The report is what counts.
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

// Options controls DetectWith.
type Options struct {
	// Path is an explicitly chosen path to the veraPDF CLI, for example from
	// the designer settings. It takes precedence over EnvRunner.
	Path string
}

// Detect selects a runner according to EnvRunner, otherwise a local CLI of
// the right version, otherwise Docker. warn receives hints, for example
// about a different local version, and may be nil.
func Detect(ctx context.Context, warn func(string)) (Runner, error) {
	return DetectWith(ctx, Options{}, warn)
}

// DetectWith selects a runner: first opts.Path, then EnvRunner, otherwise
// automatically. Automatic detection looks for veraPDF and Docker in the
// PATH and in the usual installation folders, because programs started
// from Finder, Launchpad or the Start menu do not see the shell's PATH.
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

// explicit uses an explicitly named CLI, a different version only
// results in a hint.
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

// searchDirs returns the folders where installations outside the PATH
// reside. Replaceable in tests.
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
		// The veraPDF installer puts the CLI under ~/verapdf.
		return []string{filepath.Join(home, "verapdf"), "/usr/local/bin", "/opt/homebrew/bin", "/usr/local/verapdf", "/opt/verapdf"}
	case "docker":
		return []string{"/usr/local/bin", "/opt/homebrew/bin", filepath.Join(home, ".docker", "bin"), "/Applications/Docker.app/Contents/Resources/bin", "/usr/bin"}
	}
	return nil
}

// candidates returns executables of a program: first from the PATH,
// then from searchDirs, without duplicates.
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

// withPath returns the environment with dir prepended to the PATH.
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
