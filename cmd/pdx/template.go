package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/tim-riep/pdx/check"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/tim-riep/pdx"
	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/compile"
	"github.com/tim-riep/pdx/dataschema"
	"github.com/tim-riep/pdx/template"
	"github.com/tim-riep/pdx/template/pkg"
)

// printErr gibt einen Fehler aus, Vorlagenfehler je Zeile mit ID.
func printErr(w io.Writer, cmd string, err error) {
	var es template.Errors
	if errors.As(err, &es) {
		for _, e := range es {
			fmt.Fprintf(w, "pdx %s: %v\n", cmd, e)
		}
		return
	}
	fmt.Fprintf(w, "pdx %s: %v\n", cmd, err)
}

// stripExt entfernt .zip oder einen Schrägstrich am Ende.
func stripExt(p string) string {
	p = strings.TrimSuffix(p, string(filepath.Separator))
	return strings.TrimSuffix(p, ".zip")
}

func cmdCompile(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("compile", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("o", "", "Ausgabedatei (Vorgabe: Vorlage mit Endung .xsl)")
	lang := fs.String("lang", "", "Sprache der Texte und Formate (Vorgabe: Sprache der Vorlage)")
	srcmap := fs.String("source-map", "", "Quellzuordnung zusätzlich als JSON in diese Datei schreiben")
	pos, err := parse(fs, args)
	if err != nil {
		return 2
	}
	if len(pos) != 1 {
		fmt.Fprintln(stderr, "pdx compile: genau eine Vorlage (ZIP oder Verzeichnis) erwartet")
		return 2
	}
	p, err := pkg.Open(pos[0])
	if err != nil {
		printErr(stderr, "compile", err)
		return 1
	}
	res, err := compile.Compile(p.Template, compile.Options{Lang: *lang, Enums: dataschema.PackageEnums(p)})
	if err != nil {
		printErr(stderr, "compile", err)
		return 1
	}
	if *out == "" {
		*out = stripExt(pos[0]) + ".xsl"
	}
	if err := os.WriteFile(*out, res.XSLT, 0o644); err != nil {
		printErr(stderr, "compile", err)
		return 1
	}
	if *srcmap != "" {
		b, _ := json.MarshalIndent(res.SourceMap, "", "  ")
		if err := os.WriteFile(*srcmap, append(b, '\n'), 0o644); err != nil {
			printErr(stderr, "compile", err)
			return 1
		}
	}
	fmt.Fprintf(stdout, "%s: %d Zeilen XSLT\n", *out, strings.Count(string(res.XSLT), "\n"))
	return 0
}

// renderTemplate setzt Daten mit einer Vorlage (pdx render --template).
func renderTemplate(tplPath, data, out string, opts pdx.TemplateOptions, foOut, treeOut string, stdout, stderr io.Writer) int {
	p, err := pkg.Open(tplPath)
	if err != nil {
		printErr(stderr, "render", err)
		return 1
	}
	b, err := os.ReadFile(data)
	if err != nil {
		printErr(stderr, "render", err)
		return 2
	}
	tpl, err := pdx.CompileTemplate(p, opts.Lang)
	if err != nil {
		printErr(stderr, "render", err)
		return 1
	}
	if foOut != "" {
		fo, err := tpl.Compiled.Transform(b)
		if err != nil {
			printErr(stderr, "render", err)
			return 1
		}
		if err := os.WriteFile(foOut, fo, 0o644); err != nil {
			printErr(stderr, "render", err)
			return 1
		}
	}
	if opts.Profile == "" {
		opts.Profile = tpl.Profile()
	}
	res, err := tpl.Render(b, opts)
	if err != nil {
		printErr(stderr, "render", err)
		return 1
	}
	if out == "" {
		out = strings.TrimSuffix(data, filepath.Ext(data)) + ".pdf"
	}
	if err := os.WriteFile(out, res.PDF, 0o644); err != nil {
		printErr(stderr, "render", err)
		return 1
	}
	if treeOut != "" {
		if err := os.WriteFile(treeOut, areatree.Dump(res.Tree), 0o644); err != nil {
			printErr(stderr, "render", err)
			return 1
		}
	}
	fmt.Fprintf(stdout, "%s: %d Seiten, %d Bytes, Profil %s\n", out, len(res.Tree.Pages), len(res.PDF), opts.Profile)
	return 0
}

// cmdCheck prüft eine Vorlage: Schema der Vorlage, Verweise im Paket,
// Übersetzung, Bindungen gegen das Daten-XSD und die Beispieldaten.
func cmdCheck(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	format := fs.String("format", "text", "Ausgabeformat text oder json")
	pos, err := parse(fs, args)
	if err != nil {
		return 2
	}
	if len(pos) != 1 {
		fmt.Fprintln(stderr, "pdx check: genau eine Vorlage (ZIP oder Verzeichnis) erwartet")
		return 2
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintf(stderr, "pdx check: Format %q unbekannt\n", *format)
		return 2
	}
	findings := check.Path(pos[0])
	if *format == "json" {
		if findings == nil {
			findings = []dataschema.Finding{}
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		enc.Encode(findings)
	} else {
		for _, f := range findings {
			fmt.Fprintln(stdout, f)
		}
		if len(findings) == 0 {
			fmt.Fprintf(stdout, "%s: keine Befunde\n", pos[0])
		}
	}
	if dataschema.HasErrors(findings) {
		return 1
	}
	return 0
}

// cmdFmt schreibt template.xml kanonisch.
func cmdFmt(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("fmt", flag.ContinueOnError)
	fs.SetOutput(stderr)
	write := fs.Bool("w", false, "Datei überschreiben statt auf die Standardausgabe zu schreiben")
	pos, err := parse(fs, args)
	if err != nil {
		return 2
	}
	if len(pos) != 1 {
		fmt.Fprintln(stderr, "pdx fmt: genau eine template.xml erwartet")
		return 2
	}
	src, err := os.ReadFile(pos[0])
	if err != nil {
		printErr(stderr, "fmt", err)
		return 2
	}
	tpl, err := template.Parse(src)
	if err != nil {
		printErr(stderr, "fmt", err)
		return 1
	}
	out := template.Write(tpl)
	if !*write {
		stdout.Write(out)
		return 0
	}
	if string(out) == string(src) {
		return 0
	}
	if err := os.WriteFile(pos[0], out, 0o644); err != nil {
		printErr(stderr, "fmt", err)
		return 1
	}
	fmt.Fprintf(stdout, "%s: neu formatiert\n", pos[0])
	return 0
}

// cmdPack packt ein Vorlagenverzeichnis deterministisch als ZIP.
func cmdPack(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("pack", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("o", "", "Ausgabedatei (Vorgabe: Verzeichnis mit Endung .zip)")
	pos, err := parse(fs, args)
	if err != nil {
		return 2
	}
	if len(pos) != 1 {
		fmt.Fprintln(stderr, "pdx pack: genau ein Vorlagenverzeichnis erwartet")
		return 2
	}
	p, err := pkg.ReadDir(pos[0])
	if err != nil {
		printErr(stderr, "pack", err)
		return 1
	}
	b, err := p.Bytes()
	if err != nil {
		printErr(stderr, "pack", err)
		return 1
	}
	if *out == "" {
		*out = stripExt(pos[0]) + ".zip"
	}
	if err := os.WriteFile(*out, b, 0o644); err != nil {
		printErr(stderr, "pack", err)
		return 1
	}
	fmt.Fprintf(stdout, "%s: %d Dateien, %d Bytes\n", *out, len(p.Files)+1, len(b))
	return 0
}
