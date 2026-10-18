package pdx

import (
	"bytes"
	"strconv"
	"testing"

	"github.com/tim-riep/pdx/internal/testutil"
	"github.com/tim-riep/pdx/pdf"
	"github.com/tim-riep/pdx/pdf/incr"
	"github.com/tim-riep/pdx/pdf/reader"
)

// TestIncrementalUpdate hängt an die Musterrechnung ein Update an, das den
// Katalog ändert. Die alten Bytes bleiben, der Reader sieht die neue
// Fassung, veraPDF besteht weiter.
func TestIncrementalUpdate(t *testing.T) {
	p, data := loadMuster(t)
	for _, prof := range []string{"pdfa-3b", "pdfa-4f"} {
		t.Run(prof, func(t *testing.T) {
			res, err := RenderTemplate(p, data, TemplateOptions{Options: Options{CreationDate: templateDate, Profile: prof}})
			if err != nil {
				t.Fatal(err)
			}
			u, err := incr.Open(res.PDF)
			if err != nil {
				t.Fatal(err)
			}
			cat, err := u.Dict(u.Root())
			if err != nil {
				t.Fatal(err)
			}
			cat.Set("PageLayout", pdf.Name("OneColumn"))
			u.Set(u.Root(), cat)
			out, err := u.Bytes()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.HasPrefix(out.Data, res.PDF) || !bytes.HasSuffix(out.Data, []byte("%%EOF\n")) {
				t.Fatal("Ausgangsdatei verändert oder Ende falsch")
			}
			// Jeder neue Xref-Eintrag zeigt auf sein Objekt.
			for n, off := range out.Offsets {
				if !bytes.HasPrefix(out.Data[off:], []byte(strconv.Itoa(n)+" 0 obj\n")) {
					t.Errorf("Offset von Objekt %d falsch", n)
				}
			}
			f, err := reader.Open(out.Data)
			if err != nil {
				t.Fatal(err)
			}
			if f.Root()["PageLayout"] != reader.Name("OneColumn") {
				t.Error("neuer Katalog nicht gelesen")
			}
			if pages, _ := f.Pages(); len(pages) != 3 {
				t.Errorf("%d Seiten nach dem Update", len(pages))
			}
			ids := f.Trailer["ID"].(reader.Array)
			old, _ := reader.Open(res.PDF)
			if !bytes.Equal(ids[0].(reader.String), old.Trailer["ID"].(reader.Array)[0].(reader.String)) {
				t.Error("erster Teil der ID geändert")
			}
			testutil.Conform(t, "update-"+prof+".pdf", out.Data, prof)
		})
	}
}
