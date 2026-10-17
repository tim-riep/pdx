// Package fopcompare vergleicht die Ausgabe von pdx mit Apache FOP. FOP und
// Poppler laufen ausschließlich in Docker und nur in Tests (PDX_FOP=1).
//
// Verglichen werden Seitenzahl, Zeileninhalte, Lage von Zeilen und Blöcken
// und gerasterte Seiten. Die Toleranzen stehen in tolerance.go.
package fopcompare

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

const (
	// FOPImage ist das lokal gebaute FOP-Image (testdata/docker/fop).
	FOPImage = "pdx-test/fop:2.11"
	// PopplerImage ist das lokal gebaute Poppler-Image (testdata/docker/poppler).
	PopplerImage = "pdx-test/poppler:2"
	// EnvEnable schaltet die Vergleichstests ein.
	EnvEnable = "PDX_FOP"
)

// Enabled meldet, ob Vergleiche mit FOP laufen sollen.
func Enabled() bool { return os.Getenv(EnvEnable) == "1" }

// RepoRoot sucht das Verzeichnis mit go.mod oberhalb des aktuellen.
func RepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("go.mod nicht gefunden")
		}
		dir = parent
	}
}

var buildOnce sync.Map // Image zu error

// EnsureImages baut die Test-Images, falls sie fehlen.
func EnsureImages(ctx context.Context) error {
	root, err := RepoRoot()
	if err != nil {
		return err
	}
	for _, img := range []struct{ tag, dockerfile, context string }{
		{FOPImage, "testdata/docker/fop/Dockerfile", "."},
		{PopplerImage, "testdata/docker/poppler/Dockerfile", "testdata/docker/poppler"},
	} {
		v, _ := buildOnce.LoadOrStore(img.tag, &sync.Once{})
		var berr error
		v.(*sync.Once).Do(func() {
			if exec.CommandContext(ctx, "docker", "image", "inspect", img.tag).Run() == nil {
				return
			}
			cmd := exec.CommandContext(ctx, "docker", "build", "-q", "-t", img.tag, "-f", img.dockerfile, img.context)
			cmd.Dir = root
			if out, err := cmd.CombinedOutput(); err != nil {
				berr = fmt.Errorf("docker build %s: %v\n%s", img.tag, err, out)
			}
		})
		if berr != nil {
			return berr
		}
	}
	return nil
}

func docker(ctx context.Context, args ...string) ([]byte, error) {
	var out, errOut bytes.Buffer
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("docker %v: %v\n%s", args[:min(len(args), 6)], err, lastLines(errOut.String(), 15))
	}
	return out.Bytes(), nil
}

func lastLines(s string, n int) string {
	lines := bytes.Split([]byte(s), []byte("\n"))
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return string(bytes.Join(lines, []byte("\n")))
}

// FOPResult enthält die Ausgaben von FOP.
type FOPResult struct {
	PDF      []byte
	AreaTree []byte
	Log      string
}

// RunFOP setzt eine FO-Datei mit FOP. Das Verzeichnis der Datei wird nur
// lesend eingebunden, damit relative Bilder gefunden werden.
func RunFOP(ctx context.Context, foFile string) (*FOPResult, error) {
	return runFOP(ctx, foFile, "")
}

// RunFOPRelaxed setzt wie RunFOP, aber mit gelockerter Prüfung (fop -r).
// FOP lehnt Attribute fremder Namespaces wie pdx:id sonst als ungültige
// Eigenschaft ab, obwohl XSL 1.1 sie erlaubt (siehe DEVIATIONS.md).
func RunFOPRelaxed(ctx context.Context, foFile string) (*FOPResult, error) {
	return runFOP(ctx, foFile, "-r ")
}

func runFOP(ctx context.Context, foFile, flags string) (*FOPResult, error) {
	if err := EnsureImages(ctx); err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(foFile)
	if err != nil {
		return nil, err
	}
	root, err := RepoRoot()
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return nil, err
	}
	out, err := os.MkdirTemp("", "pdx-fop-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(out)
	script := fmt.Sprintf("fop %s-c /etc/fop/fop.xconf -fo %q -pdf /out/a.pdf && fop %s-c /etc/fop/fop.xconf -fo %q -at application/pdf /out/a.at.xml",
		flags, filepath.ToSlash(rel), flags, filepath.ToSlash(rel))
	logOut, err := docker(ctx, "run", "--rm", "--network", "none",
		"-v", root+":/work:ro", "-v", out+":/out", "--entrypoint", "sh", FOPImage, "-c", script+" 2>&1")
	if err != nil {
		return nil, err
	}
	pdf, err := os.ReadFile(filepath.Join(out, "a.pdf"))
	if err != nil {
		return nil, err
	}
	at, err := os.ReadFile(filepath.Join(out, "a.at.xml"))
	if err != nil {
		return nil, err
	}
	return &FOPResult{PDF: pdf, AreaTree: at, Log: string(logOut)}, nil
}

// poppler führt ein Poppler-Werkzeug auf einer PDF-Datei aus und liefert das
// Arbeitsverzeichnis mit den Ergebnissen. Der Aufrufer räumt es auf.
func poppler(ctx context.Context, pdf []byte, args ...string) (string, error) {
	if err := EnsureImages(ctx); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp("", "pdx-poppler-")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "in.pdf"), pdf, 0o644); err != nil {
		return dir, err
	}
	full := append([]string{"run", "--rm", "--network", "none", "-v", dir + ":/work", PopplerImage}, args...)
	if _, err := docker(ctx, full...); err != nil {
		return dir, err
	}
	return dir, nil
}
