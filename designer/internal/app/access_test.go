package app

import (
	"testing"

	"github.com/tim-riep/pdx/pdf/render"
	"github.com/tim-riep/pdx/template"
)

func findNode(n *render.StructNode, id string) *render.StructNode {
	if n == nil {
		return nil
	}
	if n.ID == id {
		return n
	}
	for _, k := range n.Kids {
		if f := findNode(k, id); f != nil {
			return f
		}
	}
	return nil
}

// The sample invoice does not require accessibility, so the check falls
// back to PDF/A-3a with PDF/UA-1. The tree carries template IDs.
func TestAccess(t *testing.T) {
	c := NewCore(&recorder{}, nil, t.TempDir())
	if _, err := NewProjectService(c).Open(muster); err != nil {
		t.Fatal(err)
	}
	a, err := NewRenderService(c).Access()
	if err != nil {
		t.Fatal(err)
	}
	if !a.Assumed || a.Profile != "pdfa-3a,pdfua-1" || len(a.Problems) != 0 || len(a.Findings) != 0 {
		t.Fatalf("%+v", a)
	}
	if n := findNode(a.Tree, "titel"); n == nil || n.Type != "H1" || n.Text != "Rechnung" {
		t.Errorf("Überschrift %+v", n)
	}
	// Without alternative text on the GiroCode the pre-check reports it.
	var strip func(n *template.Node)
	strip = func(n *template.Node) {
		if n.ID() == "girocode" {
			n.RemoveAttr("altBind")
		}
		for _, k := range n.Children {
			strip(k)
		}
	}
	strip(c.doc.Pkg.Template.Root)
	a, err = NewRenderService(c).Access()
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Findings) != 1 || a.Findings[0].ID != "girocode" {
		t.Errorf("Befunde %+v", a.Findings)
	}
}

// The e-invoice of the CII template shows the level of the sample data.
func TestEInvoiceInfo(t *testing.T) {
	c := NewCore(&recorder{}, nil, t.TempDir())
	if _, err := NewProjectService(c).Open("../../../testdata/templates/cii-rechnung"); err != nil {
		t.Fatal(err)
	}
	rs := NewRenderService(c)
	e, err := rs.EInvoice()
	if err != nil || !e.Present || e.Level != "EN 16931" || e.FileName != "factur-x.xml" || e.Relationship != "Alternative" || e.ProfileError != "" {
		t.Fatalf("%+v %v", e, err)
	}
	rs.SetView(ViewPreview, "samples/xrechnung.xml")
	c.doc.Pkg.Template.Root.SetAttr("profile", "pdfa-4f,pdfua-2")
	e, _ = rs.EInvoice()
	if e.Level != "XRECHNUNG" || e.FileName != "xrechnung.xml" || e.ProfileError == "" {
		t.Errorf("%+v", e)
	}
	// The sample invoice has no e-invoice.
	if _, err := NewProjectService(c).Open(muster); err != nil {
		t.Fatal(err)
	}
	if e, _ := rs.EInvoice(); e.Present {
		t.Errorf("%+v", e)
	}
}
