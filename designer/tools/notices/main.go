// Befehl notices schreibt die Drittanbieterhinweise des Designers nach
// notices/THIRD_PARTY_NOTICES.txt: den Hinweis der Engine, die Go-Module,
// die in das App-Binary gelinkt werden (macOS, Linux, Windows), und die
// npm-Pakete im Bündel des Frontends (aus frontend/bundled-packages.json,
// das der Vite-Build schreibt). Aufruf im Ordner designer: make notices.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type module struct{ path, version, dir string }

// own sind Module dieses Repositorys, sie stehen im Hinweis der Engine.
var own = map[string]bool{"github.com/tim-riep/pdx": true, "github.com/tim-riep/pdx/designer": true}

var licenseFile = regexp.MustCompile(`(?i)^(licen[cs]e|copying|notice)(\.(txt|md))?$`)

// sameProject ordnet npm-Pakete ohne Lizenzdatei dem Go-Modul desselben
// Projekts zu, dessen Lizenz für sie gilt.
var sameProject = map[string]string{"@wailsio/runtime": "github.com/wailsapp/wails/v3"}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "notices:", err)
		os.Exit(1)
	}
}

func run() error {
	var b bytes.Buffer
	b.WriteString("pdx Designer: Drittanbieterhinweise\n")
	b.WriteString("Erzeugt mit make notices, nicht von Hand ändern.\n\n")

	engine, err := os.ReadFile("../NOTICE")
	if err != nil {
		return err
	}
	section(&b, "pdx (Engine)")
	b.Write(engine)

	mods, err := goModules()
	if err != nil {
		return err
	}
	goroot, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		return err
	}
	// Manche Pakete (etwa Homebrew) legen die Lizenz über GOROOT ab.
	root := strings.TrimSpace(string(goroot))
	var goLicense []byte
	for _, dir := range []string{root, filepath.Dir(root)} {
		if goLicense, err = os.ReadFile(filepath.Join(dir, "LICENSE")); err == nil {
			break
		}
	}
	if goLicense == nil {
		return fmt.Errorf("Lizenz von Go unter %s nicht gefunden", root)
	}
	section(&b, "Go (Standardbibliothek und Laufzeit)")
	b.Write(goLicense)
	for _, m := range mods {
		text, err := licenses(m.dir)
		if err != nil {
			return fmt.Errorf("%s: %w", m.path, err)
		}
		section(&b, m.path+" "+m.version)
		b.WriteString(text)
	}

	pkgs, err := npmPackages()
	if err != nil {
		return err
	}
	dirs := map[string]string{}
	for _, m := range mods {
		dirs[m.path] = m.dir
	}
	for _, p := range pkgs {
		text, err := licenses(filepath.Join("frontend", "node_modules", filepath.FromSlash(p.Name)))
		if err != nil && sameProject[p.Name] != "" && dirs[sameProject[p.Name]] != "" {
			text, err = licenses(dirs[sameProject[p.Name]])
			text = "Das Paket enthält keine Lizenzdatei, es gehört zum Projekt " + sameProject[p.Name] + " mit dieser Lizenz:\n\n" + text
		}
		if err != nil {
			return fmt.Errorf("%s: %w", p.Name, err)
		}
		section(&b, fmt.Sprintf("%s %s (npm, %s)", p.Name, p.Version, p.License))
		b.WriteString(text)
	}
	return os.WriteFile(filepath.Join("notices", "THIRD_PARTY_NOTICES.txt"), b.Bytes(), 0o644)
}

func section(b *bytes.Buffer, title string) {
	b.WriteString("\n" + strings.Repeat("=", 72) + "\n" + title + "\n" + strings.Repeat("=", 72) + "\n\n")
}

// goModules liefert die Module, die in das Binary des Designers gelinkt
// werden, vereint über die drei Zielsysteme.
func goModules() ([]module, error) {
	seen := map[string]module{}
	for _, goos := range []string{"darwin", "linux", "windows"} {
		cmd := exec.Command("go", "list", "-deps", "-f", "{{with .Module}}{{.Path}}\t{{.Version}}\t{{.Dir}}{{end}}", ".")
		cmd.Env = append(os.Environ(), "GOOS="+goos, "CGO_ENABLED=1")
		var errOut bytes.Buffer
		cmd.Stderr = &errOut
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("go list (%s): %v %s", goos, err, errOut.String())
		}
		for _, line := range strings.Split(string(out), "\n") {
			f := strings.Split(line, "\t")
			if len(f) != 3 || own[f[0]] {
				continue
			}
			seen[f[0]] = module{f[0], f[1], f[2]}
		}
	}
	out := make([]module, 0, len(seen))
	for _, m := range seen {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out, nil
}

type npmPackage struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	License string `json:"license"`
}

func npmPackages() ([]npmPackage, error) {
	b, err := os.ReadFile(filepath.Join("frontend", "bundled-packages.json"))
	if err != nil {
		return nil, fmt.Errorf("%v (erst das Frontend bauen)", err)
	}
	var out []npmPackage
	return out, json.Unmarshal(b, &out)
}

// licenses liefert die Lizenz- und Hinweisdateien im Wurzelordner eines
// Pakets.
func licenses(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, e := range entries {
		if e.IsDir() || !licenseFile.MatchString(e.Name()) {
			continue
		}
		text, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return "", err
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.Write(bytes.TrimSpace(text))
		b.WriteString("\n")
	}
	if b.Len() == 0 {
		return "", fmt.Errorf("keine Lizenzdatei in %s", dir)
	}
	return b.String(), nil
}
