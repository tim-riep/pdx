package fopcompare

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// RunFOPBatch setzt viele FO-Dateien mit FOP (fop -r), verteilt auf parallel
// Container. files sind Pfade unterhalb des Repositorys, die Ergebnisse
// landen als <name>.pdf und <name>.at.xml in out. Fehlt eines davon, ist FOP
// an der Datei gescheitert, der Grund steht in <name>.log.
func RunFOPBatch(ctx context.Context, files []string, out string, parallel int) error {
	if err := EnsureImages(ctx); err != nil {
		return err
	}
	root, err := RepoRoot()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	parallel = max(parallel, 1)
	chunks := make([][]string, parallel)
	for i, f := range files {
		abs, err := filepath.Abs(f)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, abs)
		if err != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("%s liegt nicht im Repository", f)
		}
		chunks[i%parallel] = append(chunks[i%parallel], filepath.ToSlash(rel))
	}
	var wg sync.WaitGroup
	errs := make([]error, parallel)
	for i, chunk := range chunks {
		if len(chunk) == 0 {
			continue
		}
		var b strings.Builder
		for _, rel := range chunk {
			name := strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
			fmt.Fprintf(&b, "timeout 120 fop -r -c /etc/fop/fop.xconf -fo %q -pdf /out/%s.pdf >/out/%s.log 2>&1 && ", rel, name, name)
			fmt.Fprintf(&b, "timeout 120 fop -r -c /etc/fop/fop.xconf -fo %q -at application/pdf /out/%s.at.xml >>/out/%s.log 2>&1\n", rel, name, name)
		}
		wg.Add(1)
		go func(i int, script string) {
			defer wg.Done()
			_, errs[i] = docker(ctx, "run", "--rm", "--network", "none", "-w", "/work",
				"-v", root+":/work:ro", "-v", out+":/out", "--entrypoint", "sh", FOPImage, "-c", script+"true")
		}(i, b.String())
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// PopplerBatch schreibt zu jeder PDF-Datei in dir die Ausgabe von
// pdftotext -bbox-layout als <name>.bbox.html.
func PopplerBatch(ctx context.Context, dir string) error {
	if err := EnsureImages(ctx); err != nil {
		return err
	}
	script := `for f in *.pdf; do pdftotext -bbox-layout -enc UTF-8 "$f" "${f%.pdf}.bbox.html" || true; done`
	_, err := docker(ctx, "run", "--rm", "--network", "none", "-w", "/work", "-v", dir+":/work",
		"--entrypoint", "sh", PopplerImage, "-c", script)
	return err
}
