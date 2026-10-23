package main

import (
	"flag"
	"fmt"
	"io"

	"github.com/tim-riep/pdx"
)

// cmdLicenses prints the license of pdx, the notice and the license texts
// of all included third-party components. --list names only the
// components with their license.
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
