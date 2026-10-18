package main

import (
	"flag"
	"fmt"
	"io"

	"github.com/tim-riep/pdx"
)

// cmdLicenses gibt die Lizenz von pdx, den Hinweis und die Lizenztexte
// aller enthaltenen Fremdbestandteile aus. --list nennt nur die
// Bestandteile mit ihrer Lizenz.
func cmdLicenses(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("licenses", flag.ContinueOnError)
	fs.SetOutput(stderr)
	list := fs.Bool("list", false, "nur Bestandteile und Lizenzen auflisten")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	return printLicenses(stdout, *list)
}

func printLicenses(w io.Writer, list bool) int {
	if list {
		fmt.Fprintln(w, "pdx: Apache-2.0")
		for _, c := range pdx.Components(pdx.ThirdPartyNotices) {
			fmt.Fprintf(w, "%s: %s\n", c.Name, c.License)
		}
		return 0
	}
	fmt.Fprint(w, pdx.License)
	fmt.Fprint(w, "\n\n")
	fmt.Fprint(w, pdx.ThirdPartyNotices)
	return 0
}
