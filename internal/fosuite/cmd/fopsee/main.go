// Command fopsee shows the lines from FOP's area tree for a case (for
// analysis only, reads out/suites/ref).
package main

import (
	"fmt"
	"os"

	"github.com/tim-riep/pdx/internal/fopcompare"
)

func main() {
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_, lines, _, _ := fopcompare.ParseAreaTreeLines(data)
	for _, l := range lines {
		fmt.Printf("%d %.2f %q\n", l.Page, l.Baseline, l.Text)
	}
}
