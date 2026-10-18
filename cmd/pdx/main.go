// Command pdx erzeugt PDF-Dateien aus XSL-FO oder Vorlagen und prüft sie mit
// veraPDF.
//
//	pdx render eingabe.fo -o ausgabe.pdf --profile pdfa-3b \
//	    --attach factur-x.xml:Alternative:text/xml \
//	    --creation-date 2026-01-01T00:00:00Z
//	pdx render --template vorlage.zip daten.xml -o ausgabe.pdf --strict
//	pdx compile vorlage.zip -o vorlage.xsl
//	pdx check vorlage.zip
//	pdx verify ausgabe.pdf --profile pdfa-3b
//
// Exit-Codes: 0 Erfolg, 1 Prüfung fehlgeschlagen oder Fehler beim Rendern,
// 2 Bedienfehler.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tim-riep/pdx"
	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/font"
	"github.com/tim-riep/pdx/pdf/profile"
	"github.com/tim-riep/pdx/verapdf"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

const usage = `pdx %s: XSL-FO und Vorlagen zu PDF bis PDF/A-3

Aufruf:
  pdx render <eingabe.fo> [-o ausgabe.pdf] [--profile NAME] [--attach DATEI:BEZIEHUNG:MIME[:BESCHREIBUNG]]...
             [--creation-date RFC3339] [--font DATEI]... [--area-tree DATEI]
  pdx render --template <vorlage> <daten.xml> [-o ausgabe.pdf] [--strict] [--lang SPRACHE]
             [--fo DATEI] und die übrigen Optionen von render
  pdx compile <vorlage> [-o vorlage.xsl] [--lang SPRACHE] [--source-map DATEI]
  pdx check <vorlage> [--format text|json]
  pdx fmt [-w] <template.xml>
  pdx pack <verzeichnis> [-o vorlage.zip]
  pdx verify <datei.pdf> [--profile NAME] [--format text|json]
  pdx structure <datei.pdf>
  pdx extract-data <formular.pdf> --template <vorlage> [-o daten.xml] [--data grunddaten.xml]
  pdx flatten <formular.pdf> --template <vorlage> [-o statisch.pdf] [--profile NAME] [--data grunddaten.xml]
  pdx sign <eingabe.pdf> --signer pkcs12:datei.p12 [-o signiert.pdf] [--level B-B|B-T|B-LT|B-LTA]
             [--tsa URL] [--field NAME] [--reason TEXT] [--location TEXT] [--contact TEXT]
             Passwort aus PDX_SIGN_PASSWORD oder Abfrage, nie als Argument
  pdx version

Eine Vorlage ist ein ZIP-Paket oder ein Verzeichnis mit template.xml.

Profile: %s
`

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, usage, pdx.Version, strings.Join(profile.Names(), ", "))
		return 2
	}
	switch args[0] {
	case "render":
		return cmdRender(args[1:], stdout, stderr)
	case "verify":
		return cmdVerify(args[1:], stdout, stderr)
	case "compile":
		return cmdCompile(args[1:], stdout, stderr)
	case "check":
		return cmdCheck(args[1:], stdout, stderr)
	case "fmt":
		return cmdFmt(args[1:], stdout, stderr)
	case "pack":
		return cmdPack(args[1:], stdout, stderr)
	case "structure":
		return cmdStructure(args[1:], stdout, stderr)
	case "extract-data":
		return cmdExtract(args[1:], stdout, stderr)
	case "flatten":
		return cmdFlatten(args[1:], stdout, stderr)
	case "sign":
		return cmdSign(args[1:], stdout, stderr)
	case "version":
		fmt.Fprintf(stdout, "pdx %s (veraPDF %s)\n", pdx.Version, verapdf.Version)
		return 0
	case "help", "-h", "--help":
		fmt.Fprintf(stdout, usage, pdx.Version, strings.Join(profile.Names(), ", "))
		return 0
	}
	fmt.Fprintf(stderr, "pdx: unbekannter Befehl %q\n", args[0])
	return 2
}

// multi sammelt wiederholbare Flags.
type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }

// parse erlaubt Flags vor und nach dem Positionsargument.
func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}

func cmdRender(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("o", "", "Ausgabedatei (Vorgabe: Eingabe mit Endung .pdf)")
	prof := fs.String("profile", "", "Konformitätsprofile mit Komma (Vorgabe: Profil der Vorlage, sonst "+pdx.DefaultProfile+")")
	date := fs.String("creation-date", "", "Erstellungsdatum RFC 3339 (Vorgabe: SOURCE_DATE_EPOCH oder jetzt)")
	tree := fs.String("area-tree", "", "Area Tree zusätzlich als Text in diese Datei schreiben")
	tpl := fs.String("template", "", "Vorlage (ZIP oder Verzeichnis), die Eingabe sind dann Daten")
	strict := fs.Bool("strict", false, "Daten vor dem Setzen gegen das Daten-XSD der Vorlage prüfen")
	lang := fs.String("lang", "", "Sprache der Texte und Formate einer Vorlage")
	foOut := fs.String("fo", "", "erzeugtes FO einer Vorlage zusätzlich in diese Datei schreiben")
	tagged := fs.Bool("tagged", false, "Tagged PDF mit Strukturbaum schreiben, auch wenn das Profil es nicht verlangt")
	var attach, fonts multi
	fs.Var(&attach, "attach", "Anhang DATEI:BEZIEHUNG:MIME[:BESCHREIBUNG], wiederholbar")
	fs.Var(&fonts, "font", "zusätzliche Schriftdatei, wiederholbar")
	pos, err := parse(fs, args)
	if err != nil {
		return 2
	}
	if len(pos) != 1 {
		fmt.Fprintln(stderr, "pdx render: genau eine Eingabedatei erwartet")
		return 2
	}
	in := pos[0]
	if *out == "" {
		*out = strings.TrimSuffix(in, filepath.Ext(in)) + ".pdf"
	}
	created, err := creationDate(*date)
	if err != nil {
		fmt.Fprintf(stderr, "pdx render: %v\n", err)
		return 2
	}
	opts := pdx.Options{Profile: *prof, CreationDate: created, Tagged: *tagged}
	for _, a := range attach {
		att, err := parseAttach(a)
		if err != nil {
			fmt.Fprintf(stderr, "pdx render: %v\n", err)
			return 2
		}
		opts.Attachments = append(opts.Attachments, att)
	}
	if len(fonts) > 0 {
		opts.Fonts = font.NewRegistry()
		for _, f := range fonts {
			if _, err := opts.Fonts.AddFile(f); err != nil {
				fmt.Fprintf(stderr, "pdx render: %v\n", err)
				return 1
			}
		}
	}
	if *tpl != "" {
		return renderTemplate(*tpl, in, *out, pdx.TemplateOptions{Options: opts, Lang: *lang, Strict: *strict}, *foOut, *tree, stdout, stderr)
	}
	if *strict || *lang != "" || *foOut != "" {
		fmt.Fprintln(stderr, "pdx render: --strict, --lang und --fo gelten nur mit --template")
		return 2
	}
	res, err := pdx.RenderFile(in, opts)
	if err != nil {
		fmt.Fprintf(stderr, "pdx render: %v\n", err)
		return 1
	}
	if err := os.WriteFile(*out, res.PDF, 0o644); err != nil {
		fmt.Fprintf(stderr, "pdx render: %v\n", err)
		return 1
	}
	if *tree != "" {
		if err := os.WriteFile(*tree, areatree.Dump(res.Tree), 0o644); err != nil {
			fmt.Fprintf(stderr, "pdx render: %v\n", err)
			return 1
		}
	}
	fmt.Fprintf(stdout, "%s: %d Seiten, %d Bytes, Profil %s\n", *out, len(res.Tree.Pages), len(res.PDF), *prof)
	return 0
}

// creationDate liest das Datum aus dem Flag, sonst aus SOURCE_DATE_EPOCH,
// sonst gilt die aktuelle Zeit (dann ist die Ausgabe nicht reproduzierbar).
func creationDate(v string) (time.Time, error) {
	if v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return time.Time{}, fmt.Errorf("--creation-date %q ist kein Datum nach RFC 3339", v)
		}
		return t, nil
	}
	if s := os.Getenv("SOURCE_DATE_EPOCH"); s != "" {
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("SOURCE_DATE_EPOCH %q ist keine Zahl", s)
		}
		return time.Unix(n, 0).UTC(), nil
	}
	return time.Now().UTC().Truncate(time.Second), nil
}

// parseAttach liest DATEI:BEZIEHUNG:MIME. Der Pfad darf Doppelpunkte
// enthalten, getrennt wird an den letzten beiden.
// parseAttach liest DATEI:BEZIEHUNG:MIME[:BESCHREIBUNG]. Der MIME-Typ
// enthält immer einen Schrägstrich, ein letzter Teil ohne Schrägstrich ist
// die Beschreibung (PDF/UA-2 verlangt sie, ISO 14289-2 8.14.1). Sie darf
// keinen Doppelpunkt enthalten.
func parseAttach(v string) (areatree.Attachment, error) {
	desc := ""
	if k := strings.LastIndex(v, ":"); k > 0 && !strings.Contains(v[k+1:], "/") {
		desc, v = v[k+1:], v[:k]
	}
	i := strings.LastIndex(v, ":")
	j := -1
	if i > 0 {
		j = strings.LastIndex(v[:i], ":")
	}
	if j <= 0 {
		return areatree.Attachment{}, fmt.Errorf("--attach %q: erwartet DATEI:BEZIEHUNG:MIME oder DATEI:BEZIEHUNG:MIME:BESCHREIBUNG", v)
	}
	file, rel, mime := v[:j], v[j+1:i], v[i+1:]
	data, err := os.ReadFile(file)
	if err != nil {
		return areatree.Attachment{}, fmt.Errorf("--attach: %w", err)
	}
	return areatree.Attachment{
		Name: filepath.Base(file), MIMEType: mime, Relationship: areatree.Relationship(rel), Data: data, Description: desc,
	}, nil
}

func cmdVerify(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(stderr)
	prof := fs.String("profile", pdx.DefaultProfile, "Konformitätsprofil")
	format := fs.String("format", "text", "Ausgabeformat text oder json")
	pos, err := parse(fs, args)
	if err != nil {
		return 2
	}
	if len(pos) != 1 {
		fmt.Fprintln(stderr, "pdx verify: genau eine PDF-Datei erwartet")
		return 2
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintf(stderr, "pdx verify: Format %q unbekannt\n", *format)
		return 2
	}
	flavours, err := verapdf.Flavours(*prof)
	if err != nil {
		fmt.Fprintf(stderr, "pdx verify: %v\n", err)
		return 2
	}
	ctx := context.Background()
	runner, err := verapdf.Detect(ctx, func(s string) { fmt.Fprintln(stderr, "pdx verify:", s) })
	if err != nil {
		fmt.Fprintf(stderr, "pdx verify: %v\n", err)
		return 1
	}
	// Ein Lauf je beanspruchtem Standard, das Ergebnis gilt nur, wenn alle
	// bestehen.
	var reports []*verapdf.Report
	failed := false
	for _, flavour := range flavours {
		rep, err := runner.Validate(ctx, pos[0], flavour)
		if err != nil {
			var pe *os.PathError
			if errors.As(err, &pe) {
				fmt.Fprintf(stderr, "pdx verify: %v\n", err)
				return 2
			}
			fmt.Fprintf(stderr, "pdx verify: %v\n", err)
			return 1
		}
		rep.File = pos[0]
		reports = append(reports, rep)
		failed = failed || rep.Failed()
	}
	if *format == "json" {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if len(reports) == 1 {
			enc.Encode(reports[0])
		} else {
			enc.Encode(reports)
		}
	} else {
		fmt.Fprintf(stdout, "%s\n", pos[0])
		for _, rep := range reports {
			rep.WriteText(stdout)
		}
	}
	if failed {
		return 1
	}
	return 0
}
