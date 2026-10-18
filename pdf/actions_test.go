package pdf_test

import (
	"strings"
	"testing"

	"github.com/tim-riep/pdx/pdf"
	"github.com/tim-riep/pdx/pdf/profile"
)

// doc baut ein Dokument mit Katalog, einer Seite und den übergebenen
// Zusatzobjekten unter dem Katalog.
func actionDoc(t *testing.T, spec string, extra func(d *pdf.Document, cat, page *pdf.Dict)) error {
	t.Helper()
	p, err := profile.Lookup(spec)
	if err != nil {
		t.Fatal(err)
	}
	d := pdf.NewDocument(p)
	pages := d.Reserve()
	page := pdf.D("Type", pdf.Name("Page"), "Parent", pages, "MediaBox", pdf.Array{pdf.Int(0), pdf.Int(0), pdf.Int(10), pdf.Int(10)})
	cat := pdf.D("Type", pdf.Name("Catalog"), "Pages", pages)
	extra(d, cat, page)
	d.Set(pages, pdf.D("Type", pdf.Name("Pages"), "Kids", pdf.Array{d.Add(page)}, "Count", pdf.Int(1)))
	d.Root = d.Add(cat)
	_, err = d.Bytes()
	return err
}

// Verbotene Aktionen in PDF/A werden alle auf einmal gemeldet, mit Klausel.
func TestForbiddenActions(t *testing.T) {
	bad := func(d *pdf.Document, cat, page *pdf.Dict) {
		page.Set("AA", pdf.D("O", pdf.D("S", pdf.Name("Launch"))))
		page.Set("Annots", pdf.Array{
			pdf.D("Type", pdf.Name("Annot"), "Subtype", pdf.Name("Link"), "A", pdf.D("S", pdf.Name("ResetForm"))),
			pdf.D("Type", pdf.Name("Annot"), "Subtype", pdf.Name("Link"), "A", pdf.D("S", pdf.Name("Named"), "N", pdf.Name("Print"))),
			pdf.D("Type", pdf.Name("Annot"), "Subtype", pdf.Name("Widget"), "FT", pdf.Name("Tx"), "A", pdf.D("S", pdf.Name("URI"), "URI", pdf.String("x"))),
		})
	}
	err := actionDoc(t, "pdfa-3b", bad)
	if err == nil {
		t.Fatal("verbotene Aktionen angenommen")
	}
	for _, want := range []string{
		"Aktion Launch ist verboten, ISO 19005-3 6.5.1",
		"Aktion ResetForm ist verboten",
		"Named-Aktion Print ist verboten",
		"ISO 19005-3 6.5.2 verbietet Zusatzaktionen (AA) an Seiten",
		"ISO 19005-3 6.4.1 verbietet Aktionen (A) an Formularfeldern",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%q fehlt in\n%v", want, err)
		}
	}
	// PDF/A-4 erlaubt AA an Seiten mit den Auslösern O und C nicht, an
	// Widgets dagegen beliebige.
	err = actionDoc(t, "pdfa-4", func(d *pdf.Document, cat, page *pdf.Dict) {
		page.Set("AA", pdf.D("O", pdf.D("S", pdf.Name("URI"), "URI", pdf.String("x"))))
		page.Set("Annots", pdf.Array{pdf.D("Type", pdf.Name("Annot"), "Subtype", pdf.Name("Widget"), "AA", pdf.D("K", pdf.D("S", pdf.Name("URI"), "URI", pdf.String("x"))))})
	})
	if err == nil || !strings.Contains(err.Error(), "Auslöser O in AA an Seiten sind verboten, ISO 19005-4 6.6.3") || strings.Contains(err.Error(), "Formularfeldern") {
		t.Errorf("PDF/A-4: %v", err)
	}
	// Erlaubte Aktionen gehen durch, ohne Profil prüft niemand.
	ok := func(d *pdf.Document, cat, page *pdf.Dict) {
		page.Set("Annots", pdf.Array{pdf.D("Type", pdf.Name("Annot"), "Subtype", pdf.Name("Link"), "A", pdf.D("S", pdf.Name("URI"), "URI", pdf.String("x")))})
		cat.Set("OpenAction", pdf.D("S", pdf.Name("Named"), "N", pdf.Name("FirstPage")))
	}
	for _, spec := range []string{"pdfa-3b", "pdfa-2u", "pdfa-4", "pdfa-3a,pdfua-1"} {
		if err := actionDoc(t, spec, ok); err != nil {
			t.Errorf("%s: %v", spec, err)
		}
	}
	if err := actionDoc(t, "pdf-1.7", bad); err != nil {
		t.Errorf("pdf-1.7: %v", err)
	}
	// NeedAppearances schreibt pdx in keinem Profil.
	if err := actionDoc(t, "pdf-1.7", func(d *pdf.Document, cat, page *pdf.Dict) {
		cat.Set("AcroForm", pdf.D("Fields", pdf.Array{}, "NeedAppearances", pdf.Bool(true)))
	}); err == nil || !strings.Contains(err.Error(), "NeedAppearances") {
		t.Errorf("NeedAppearances: %v", err)
	}
}
