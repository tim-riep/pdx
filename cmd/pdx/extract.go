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

// cmdExtract reads the values of a filled-in form back into the data and
// checks them against the data XSD of the template. If the data is invalid,
// it writes it anyway, reports all errors and exits with 1.
func cmdExtract(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("extract-data", flag.ContinueOnError)
	fs.SetOutput(stderr)
	tplPath := fs.String("template", "", "Vorlage (ZIP oder Verzeichnis), mit der das Formular erzeugt wurde")
	out := fs.String("o", "", "Ausgabedatei (Vorgabe: Eingabe mit Endung .xml)")
	base := fs.String("data", "", "Grunddaten, in die die Werte eingesetzt werden (Vorgabe: im PDF eingebettete Daten)")
	lang := fs.String("lang", "", "Sprache der Vorlage, wie beim Rendern")
	pos, err := parse(fs, args)
	if err != nil {
		return 2
	}
	if len(pos) != 1 || *tplPath == "" {
		fmt.Fprintln(stderr, "pdx extract-data: genau eine PDF-Datei und --template erwartet")
		return 2
	}
	pdf, err := os.ReadFile(pos[0])
	if err != nil {
		fmt.Fprintf(stderr, "pdx extract-data: %v\n", err)
		return 2
	}
	var baseData []byte
	if *base != "" {
		if baseData, err = os.ReadFile(*base); err != nil {
			fmt.Fprintf(stderr, "pdx extract-data: %v\n", err)
			return 2
		}
	}
	p, err := pkg.Open(*tplPath)
	if err != nil {
		printErr(stderr, "extract-data", err)
		return 2
	}
	tpl, err := pdx.CompileTemplate(p, *lang)
	if err != nil {
		printErr(stderr, "extract-data", err)
		return 2
	}
	if *out == "" {
		*out = strings.TrimSuffix(pos[0], filepath.Ext(pos[0])) + ".xml"
	}
	data, err := tpl.ExtractData(pdf, baseData)
	if e, ok := pdx.IsExtractError(err); ok {
		if werr := os.WriteFile(*out, e.Data, 0o644); werr != nil {
			fmt.Fprintf(stderr, "pdx extract-data: %v\n", werr)
			return 2
		}
		fmt.Fprintf(stderr, "pdx extract-data: %s geschrieben, aber ungültig:\n%v\n", *out, err)
		return 1
	}
	if err != nil {
		printErr(stderr, "extract-data", err)
		return 1
	}
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		fmt.Fprintf(stderr, "pdx extract-data: %v\n", err)
		return 2
	}
	fmt.Fprintf(stdout, "%s: Daten gültig nach dem Daten-XSD\n", *out)
	return 0
}
