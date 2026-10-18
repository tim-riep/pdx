package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/tim-riep/pdx/pdf/reader"
)

// cmdStructure gibt den logischen Strukturbaum einer PDF-Datei aus, mit
// Rolle, Sprache, Alternativtext und Text je Element. Widersprüche zwischen
// Baum, ParentTree und Inhalt erscheinen als Befund und führen zu Exit-Code 1.
func cmdStructure(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("structure", flag.ContinueOnError)
	fs.SetOutput(stderr)
	pos, err := parse(fs, args)
	if err != nil {
		return 2
	}
	if len(pos) != 1 {
		fmt.Fprintln(stderr, "pdx structure: genau eine PDF-Datei erwartet")
		return 2
	}
	data, err := os.ReadFile(pos[0])
	if err != nil {
		fmt.Fprintf(stderr, "pdx structure: %v\n", err)
		return 2
	}
	f, err := reader.Open(data)
	if err != nil {
		fmt.Fprintf(stderr, "pdx structure: %v\n", err)
		return 2
	}
	st, err := f.ReadStructure()
	if err != nil {
		fmt.Fprintf(stderr, "pdx structure: %v\n", err)
		return 2
	}
	if err := st.Format(stdout); err != nil {
		return 2
	}
	if len(st.Problems) > 0 {
		return 1
	}
	return 0
}
