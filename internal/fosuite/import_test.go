package fosuite

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tim-riep/pdx/internal/fopcompare"
)

const disabledHeader = `Deaktivierte Fälle der FO-Testsuiten mit Begründung, eine Zeile je Fall.
Zeilen mit den Gründen des Imports und der Referenz schreiben die Werkzeuge
neu (siehe internal/fosuite), alle anderen bleiben stehen.`

// TestImportFOP importiert die Testfälle aus einem FOP-Quellbaum:
// PDX_SUITE_IMPORT_FOP=<pfad>/fop-2.11/fop/test/layoutengine.
func TestImportFOP(t *testing.T) {
	src := os.Getenv("PDX_SUITE_IMPORT_FOP")
	if src == "" {
		t.Skip("PDX_SUITE_IMPORT_FOP auf test/layoutengine eines FOP-Quellbaums setzen")
	}
	root, err := fopcompare.RepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(DataDir(root), "disabled.txt")
	disabled, err := ReadList(path)
	if err != nil {
		t.Fatal(err)
	}
	n, err := ImportFOP(src, root, disabled)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteList(path, disabledHeader, disabled); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d Fälle importiert, %d deaktiviert", n, len(disabled))
}

// TestImportW3C lädt die W3C XSL Test Suite und legt die Fälle als FO ab
// (PDX_W3C=1, braucht Netzzugang beim ersten Lauf).
func TestImportW3C(t *testing.T) {
	if os.Getenv("PDX_W3C") != "1" {
		t.Skip("PDX_W3C=1 setzen, um die W3C-Suite zu laden")
	}
	root, err := fopcompare.RepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(DataDir(root), "disabled.txt")
	disabled, err := ReadList(path)
	if err != nil {
		t.Fatal(err)
	}
	n, err := PrepareW3C(root, disabled)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteList(path, disabledHeader, disabled); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d W3C-Fälle bereit", n)
}
