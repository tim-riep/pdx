package pdx

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"github.com/tim-riep/pdx/internal/testutil"
	"github.com/tim-riep/pdx/template"
	"github.com/tim-riep/pdx/template/pkg"
)

const ciiDir = "testdata/templates/cii-rechnung"

// TestEInvoice renders the CII template with data of the EN 16931 and
// XRECHNUNG levels. The XML is attached under the file name of the level,
// the XMP carries the fx schema, veraPDF checks PDF/A-3a and PDF/UA-1.
func TestEInvoice(t *testing.T) {
	p, err := pkg.ReadDir(ciiDir)
	if err != nil {
		t.Fatal(err)
	}
	for sample, want := range map[string][2]string{
		"en16931":   {"factur-x.xml", "EN 16931"},
		"xrechnung": {"xrechnung.xml", "XRECHNUNG"},
	} {
		data, _ := p.File("samples/" + sample + ".xml")
		res, err := RenderTemplate(p, data, TemplateOptions{Options: Options{CreationDate: templateDate}})
		if err != nil {
			t.Fatalf("%s: %v", sample, err)
		}
		a := res.Tree.Attachments
		if len(a) != 1 || a[0].Name != want[0] || a[0].Relationship != "Alternative" || !bytes.Equal(a[0].Data, data) {
			t.Errorf("%s: Anhang %+v", sample, a)
		}
		for _, s := range []string{"<fx:ConformanceLevel>" + want[1] + "</fx:ConformanceLevel>", "<fx:DocumentFileName>" + want[0] + "<",
			"<pdfaSchema:namespaceURI>urn:factur-x:pdfa:CrossIndustryDocument:invoice:1p0#</pdfaSchema:namespaceURI>"} {
			if !bytes.Contains(res.PDF, []byte(s)) {
				t.Errorf("%s: %q fehlt", sample, s)
			}
		}
		structure(t, res.PDF)
		testutil.Conform(t, "cii-"+sample+".pdf", res.PDF, "pdfa-3a,pdfua-1")
	}

	data, _ := p.File("samples/en16931.xml")
	// Factur-X and ZUGFeRD only describe PDF/A-3.
	_, err = RenderTemplate(p, data, TemplateOptions{Options: Options{CreationDate: templateDate, Profile: "pdfa-4f,pdfua-2"}})
	var e *template.Error
	if !errors.As(err, &e) || e.ID != "erechnung" {
		t.Errorf("PDF/A-4f nicht abgelehnt: %v", err)
	}
	// UBL data with a template ID is rejected.
	_, err = RenderTemplate(p, []byte(`<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"/>`), TemplateOptions{Options: Options{CreationDate: templateDate}})
	if !errors.As(err, &e) || e.ID != "erechnung" {
		t.Errorf("UBL nicht abgelehnt: %v", err)
	}
	// The layout preview needs no e-invoice and takes placeholders.
	tpl, err := CompileTemplate(p, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tpl.Layout([]byte(`<rsm:CrossIndustryInvoice xmlns:rsm="urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100"/>`), TemplateOptions{}); err != nil {
		t.Errorf("Layout mit Platzhalter: %v", err)
	}
	if d := os.Getenv("PDX_OUT"); d != "" {
		res, _ := RenderTemplate(p, data, TemplateOptions{Options: Options{CreationDate: templateDate}})
		os.WriteFile(d+"/cii-rechnung.pdf", res.PDF, 0o644)
	}
}
