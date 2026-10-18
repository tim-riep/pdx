package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/tim-riep/pdx"
	"github.com/tim-riep/pdx/template/pkg"
)

// cmdFlatten wandelt ein ausgefülltes Formular in ein statisches PDF im
// gewählten Profil. Die Eingaben werden wie bei extract-data gelesen und
// geprüft, ungültige Daten werden nicht abgeflacht.
func cmdFlatten(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("flatten", flag.ContinueOnError)
	fs.SetOutput(stderr)
	tplPath := fs.String("template", "", "Vorlage (ZIP oder Verzeichnis), mit der das Formular erzeugt wurde")
	out := fs.String("o", "", "Ausgabedatei (Vorgabe: Eingabe mit Endung -statisch.pdf)")
	prof := fs.String("profile", "", "Konformitätsprofile mit Komma (Vorgabe: Profil der Vorlage)")
	base := fs.String("data", "", "Grunddaten, in die die Werte eingesetzt werden (Vorgabe: im PDF eingebettete Daten)")
	lang := fs.String("lang", "", "Sprache der Vorlage, wie beim Rendern")
	date := fs.String("creation-date", "", "Erstellungsdatum RFC 3339 (Vorgabe: SOURCE_DATE_EPOCH oder jetzt)")
	pos, err := parse(fs, args)
	if err != nil {
		return 2
	}
	if len(pos) != 1 || *tplPath == "" {
		fmt.Fprintln(stderr, "pdx flatten: genau eine PDF-Datei und --template erwartet")
		return 2
	}
	created, err := creationDate(*date)
	if err != nil {
		fmt.Fprintf(stderr, "pdx flatten: %v\n", err)
		return 2
	}
	in, err := os.ReadFile(pos[0])
	if err != nil {
		fmt.Fprintf(stderr, "pdx flatten: %v\n", err)
		return 2
	}
	var baseData []byte
	if *base != "" {
		if baseData, err = os.ReadFile(*base); err != nil {
			fmt.Fprintf(stderr, "pdx flatten: %v\n", err)
			return 2
		}
	}
	p, err := pkg.Open(*tplPath)
	if err != nil {
		printErr(stderr, "flatten", err)
		return 2
	}
	tpl, err := pdx.CompileTemplate(p, *lang)
	if err != nil {
		printErr(stderr, "flatten", err)
		return 2
	}
	res, err := tpl.Flatten(in, baseData, pdx.TemplateOptions{Options: pdx.Options{Profile: *prof, CreationDate: created}})
	if err != nil {
		printErr(stderr, "flatten", err)
		return 1
	}
	if *out == "" {
		*out = strings.TrimSuffix(pos[0], filepath.Ext(pos[0])) + "-statisch.pdf"
	}
	if err := os.WriteFile(*out, res.PDF, 0o644); err != nil {
		fmt.Fprintf(stderr, "pdx flatten: %v\n", err)
		return 2
	}
	fmt.Fprintf(stdout, "%s: %d Seiten, statisch\n", *out, len(res.Tree.Pages))
	return 0
}
