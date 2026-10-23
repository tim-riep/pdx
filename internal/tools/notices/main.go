// Command notices writes third-party notices with all license texts for the
// programs of this repository: the notice of pdx, the third-party components
// embedded in the programs (fonts, hyphenation patterns), the Go runtime,
// all linked Go modules across macOS, Linux and Windows and, on request,
// the npm packages of a frontend bundle.
//
//	go run ./internal/tools/notices -pkgs ./cmd/pdx,./cmd/pdx-server -o THIRD_PARTY_NOTICES.txt
//	cd designer && go run ../internal/tools/notices -cgo -npm frontend/bundled-packages.json \
//	    -node frontend/node_modules -title "pdx Designer" -o notices/THIRD_PARTY_NOTICES.txt
//
// With -check it only compares and fails on a difference.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type module struct{ path, version, dir string }

// own lists the modules of this repository, the notice of pdx applies to them.
var own = map[string]bool{"github.com/tim-riep/pdx": true, "github.com/tim-riep/pdx/designer": true}

// assets lists third-party components that are embedded as files in pdx
// packages. They appear when the package is linked.
var assets = []struct {
	pkg, title string
	files      []string
}{
	{"github.com/tim-riep/pdx/font/fonts", "Liberation Fonts 2.1.5 (eingebettet, SIL Open Font License 1.1)", []string{"font/fonts/liberation/LICENSE"}},
	{"github.com/tim-riep/pdx/hyph", "Trennmuster aus hyph-utf8 (eingebettet, de-1996 MIT, en-us freie Weitergabe mit Hinweis)", []string{"hyph/patterns/README.md"}},
}

var licenseFile = regexp.MustCompile(`(?i)^(licen[cs]e|copying|notice)(\.(txt|md))?$`)

// sameProject maps npm packages without a license file to the Go module of
// the same project, whose license applies to them.
var sameProject = map[string]string{"@wailsio/runtime": "github.com/wailsapp/wails/v3"}

func main() {
	pkgs := flag.String("pkgs", ".", "Pakete der Programme, mit Komma")
	cgo := flag.Bool("cgo", false, "mit cgo bauen (Desktop-App)")
	npm := flag.String("npm", "", "bundled-packages.json des Frontends")
	node := flag.String("node", "", "node_modules des Frontends")
	title := flag.String("title", "pdx", "Name der Programme in der Überschrift")
	out := flag.String("o", "THIRD_PARTY_NOTICES.txt", "Ausgabedatei")
	check := flag.Bool("check", false, "nur vergleichen, Exit-Code 1 bei Abweichung")
	flag.Parse()
	b, err := generate(strings.Split(*pkgs, ","), *cgo, *npm, *node, *title)
	if err != nil {
		fmt.Fprintln(os.Stderr, "notices:", err)
		os.Exit(1)
	}
	if *check {
		old, _ := os.ReadFile(*out)
		if !bytes.Equal(old, b) {
			fmt.Fprintf(os.Stderr, "notices: %s ist veraltet, bitte neu erzeugen\n", *out)
			os.Exit(1)
		}
		return
	}
	if err := os.WriteFile(*out, b, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "notices:", err)
		os.Exit(1)
	}
}

func generate(pkgs []string, cgo bool, npm, node, title string) ([]byte, error) {
	root, err := goOut("list", "-m", "-f", "{{.Dir}}", "github.com/tim-riep/pdx")
	if err != nil {
		return nil, err
	}
	mods, linked, err := goModules(pkgs, cgo)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	b.WriteString(title + ": Drittanbieterhinweise\n")
	b.WriteString("Erzeugt mit make notices, nicht von Hand ändern.\n\n")
	b.WriteString("Die Programme stehen unter der Apache License 2.0 (LICENSE). Diese Datei\n")
	b.WriteString("enthält den Hinweis von pdx und die Lizenztexte aller Fremdbestandteile,\n")
	b.WriteString("die in den Programmen enthalten sind.\n")

	notice, err := os.ReadFile(filepath.Join(root, "NOTICE"))
	if err != nil {
		return nil, err
	}
	section(&b, "pdx (NOTICE)")
	b.Write(notice)

	for _, a := range assets {
		if !linked[a.pkg] {
			continue
		}
		section(&b, a.title)
		for i, f := range a.files {
			text, err := os.ReadFile(filepath.Join(root, f))
			if err != nil {
				return nil, err
			}
			if i > 0 {
				b.WriteString("\n")
			}
			b.Write(bytes.TrimSpace(text))
			b.WriteString("\n")
		}
	}

	goroot, err := goOut("env", "GOROOT")
	if err != nil {
		return nil, err
	}
	// Some installations (such as Homebrew) place the license above GOROOT.
	var goLicense []byte
	for _, dir := range []string{goroot, filepath.Dir(goroot)} {
		if goLicense, err = os.ReadFile(filepath.Join(dir, "LICENSE")); err == nil {
			break
		}
	}
	if goLicense == nil {
		return nil, fmt.Errorf("Lizenz von Go unter %s nicht gefunden", goroot)
	}
	section(&b, "Go (Standardbibliothek und Laufzeit)")
	b.Write(goLicense)

	dirs := map[string]string{}
	for _, m := range mods {
		text, err := licenses(m.dir)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", m.path, err)
		}
		section(&b, m.path+" "+m.version)
		b.WriteString(text)
		dirs[m.path] = m.dir
	}

	if npm == "" {
		return b.Bytes(), nil
	}
	raw, err := os.ReadFile(npm)
	if err != nil {
		return nil, fmt.Errorf("%v (erst das Frontend bauen)", err)
	}
	var packages []struct{ Name, Version, License string }
	if err := json.Unmarshal(raw, &packages); err != nil {
		return nil, err
	}
	for _, p := range packages {
		text, err := licenses(filepath.Join(node, filepath.FromSlash(p.Name)))
		if err != nil && sameProject[p.Name] != "" && dirs[sameProject[p.Name]] != "" {
			text, err = licenses(dirs[sameProject[p.Name]])
			text = "Das Paket enthält keine Lizenzdatei, es gehört zum Projekt " + sameProject[p.Name] + " mit dieser Lizenz:\n\n" + text
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p.Name, err)
		}
		section(&b, fmt.Sprintf("%s %s (npm, %s)", p.Name, p.Version, p.License))
		b.WriteString(text)
	}
	return b.Bytes(), nil
}

func goOut(args ...string) (string, error) {
	var errOut bytes.Buffer
	cmd := exec.Command("go", args...)
	cmd.Stderr = &errOut
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("go %s: %v %s", strings.Join(args, " "), err, errOut.String())
	}
	return strings.TrimSpace(string(out)), nil
}

func section(b *bytes.Buffer, title string) {
	b.WriteString("\n" + strings.Repeat("=", 72) + "\n" + title + "\n" + strings.Repeat("=", 72) + "\n\n")
}

// goModules returns the modules and packages that are linked into the
// programs, merged across the three target systems.
func goModules(pkgs []string, cgo bool) ([]module, map[string]bool, error) {
	seen := map[string]module{}
	linked := map[string]bool{}
	cgoEnv := "CGO_ENABLED=0"
	if cgo {
		cgoEnv = "CGO_ENABLED=1"
	}
	for _, goos := range []string{"darwin", "linux", "windows"} {
		args := append([]string{"list", "-deps", "-f", "{{.ImportPath}}\t{{with .Module}}{{.Path}}\t{{.Version}}\t{{.Dir}}{{end}}"}, pkgs...)
		cmd := exec.Command("go", args...)
		cmd.Env = append(os.Environ(), "GOOS="+goos, cgoEnv)
		var errOut bytes.Buffer
		cmd.Stderr = &errOut
		out, err := cmd.Output()
		if err != nil {
			return nil, nil, fmt.Errorf("go list (%s): %v %s", goos, err, errOut.String())
		}
		for _, line := range strings.Split(string(out), "\n") {
			f := strings.Split(line, "\t")
			if len(f) != 4 {
				continue
			}
			linked[f[0]] = true
			if f[1] == "" || own[f[1]] {
				continue
			}
			seen[f[1]] = module{f[1], f[2], f[3]}
		}
	}
	out := make([]module, 0, len(seen))
	for _, m := range seen {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out, linked, nil
}

// licenses returns the license and notice files in the root folder of a
// package.
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
