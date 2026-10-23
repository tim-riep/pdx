// Package fopcompare compares the output of pdx with Apache FOP. FOP and
// Poppler run exclusively in Docker and only in tests (PDX_FOP=1).
//
// It compares the page count, line contents, positions of lines and blocks
// and rasterized pages. The tolerances are in tolerance.go.
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
	// FOPImage is the locally built FOP image (testdata/docker/fop).
	FOPImage = "pdx-test/fop:2.11"
	// PopplerImage is the locally built Poppler image (testdata/docker/poppler).
	PopplerImage = "pdx-test/poppler:2"
	// EnvEnable enables the comparison tests.
	EnvEnable = "PDX_FOP"
)

// Enabled reports whether comparisons with FOP should run.
func Enabled() bool { return os.Getenv(EnvEnable) == "1" }

// RepoRoot searches for the directory with go.mod above the current one.
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

var buildOnce sync.Map // image to error

// EnsureImages builds the test images if they are missing.
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

// FOPResult contains the outputs of FOP.
type FOPResult struct {
	PDF      []byte
	AreaTree []byte
	Log      string
}

// RunFOP typesets an FO file with FOP. The directory of the file is mounted
// read-only so that relative images are found.
func RunFOP(ctx context.Context, foFile string) (*FOPResult, error) {
	return runFOP(ctx, foFile, "")
}

// RunFOPRelaxed typesets like RunFOP, but with relaxed validation (fop -r).
// Otherwise FOP rejects attributes of foreign namespaces such as pdx:id as
// invalid properties, although XSL 1.1 allows them (see DEVIATIONS.md).
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

// poppler runs a Poppler tool on a PDF file and returns the working
// directory with the results. The caller cleans it up.
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
