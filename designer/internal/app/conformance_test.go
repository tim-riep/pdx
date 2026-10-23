package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tim-riep/pdx/verapdf"
)

// fakeRunner reports one violated rule per operator of the first page.
type fakeRunner struct{ ops int }

func (fakeRunner) Name() string { return "Attrappe" }

func (f fakeRunner) Validate(ctx context.Context, file, flavour string) (*verapdf.Report, error) {
	if _, err := os.Stat(file); err != nil {
		return nil, err
	}
	rep := &verapdf.Report{Compliant: false}
	for op := range f.ops {
		rep.Findings = append(rep.Findings, verapdf.Finding{Clause: "6.2.4.3", TestNumber: fmt.Sprint(op), Description: "Farbraum", FailedChecks: 1,
			Checks: []verapdf.Check{{Context: fmt.Sprintf("root/document[0]/pages[0](13 0 obj PDPage)/contentStream[0](12 0 obj PDContentStream)/operators[%d]/fillCS[0]", op)}}})
	}
	rep.Findings = append(rep.Findings, verapdf.Finding{Clause: "6.6.2.1", Description: "Metadaten", Checks: []verapdf.Check{{Context: "root/document[0]/metadata[0]"}}})
	return rep, nil
}

func waitConformance(t *testing.T, r *recorder, timeout time.Duration) Conformance {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		d, ok := r.last[EventConformance].(Conformance)
		r.mu.Unlock()
		if ok && d.State != ConformanceRunning {
			return d
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("Konformitätsprüfung endet nicht")
	return Conformance{}
}

func TestConformanceMapping(t *testing.T) {
	rec := &recorder{}
	c := NewCore(rec, nil, t.TempDir())
	if _, err := NewProjectService(c).Open(muster); err != nil {
		t.Fatal(err)
	}
	c.conf.detect = func(context.Context, func(string)) (verapdf.Runner, error) { return fakeRunner{ops: 2000}, nil }
	cs := NewConformanceService(c)
	st, err := cs.Check()
	if err != nil || st.State != ConformanceRunning || st.Sample != "samples/rechnung.xml" || st.Profile != "pdfa-3b" {
		t.Fatalf("Start: %+v %v", st, err)
	}
	st = waitConformance(t, rec, 10*time.Second)
	if st.State != ConformanceDone || st.Compliant || st.Runner != "Attrappe" {
		t.Fatalf("Ergebnis: %+v", st)
	}
	refs := map[string]bool{}
	var doc *ConformanceItem
	for _, f := range st.Findings {
		for i, it := range f.Items {
			if it.Ref != "" {
				refs[it.Ref] = true
				if it.Page != 1 {
					t.Fatalf("Seite %d", it.Page)
				}
			}
			if f.Clause == "6.6.2.1" {
				doc = &f.Items[i]
			}
		}
	}
	for _, id := range []string{"titel", "z-betrag-f", "positionen"} {
		if !refs[id] {
			t.Errorf("kein Befund an %s, zugeordnet: %v", id, refs)
		}
	}
	if doc == nil || doc.Ref != "" || doc.Page != 0 {
		t.Fatalf("Befund ohne Seite: %+v", doc)
	}
	if got := cs.State(); got.State != ConformanceDone || len(got.Findings) != len(st.Findings) {
		t.Fatalf("State %+v", got.State)
	}
}

// flavourRunner remembers the checked flavours and reports one finding
// per flavour.
type flavourRunner struct {
	mu   sync.Mutex
	seen []string
}

func (*flavourRunner) Name() string { return "Attrappe" }

func (f *flavourRunner) Validate(ctx context.Context, file, flavour string) (*verapdf.Report, error) {
	f.mu.Lock()
	f.seen = append(f.seen, flavour)
	f.mu.Unlock()
	return &verapdf.Report{Compliant: flavour != "ua1", Findings: []verapdf.Finding{{Specification: "Norm " + flavour, Clause: "1"}}}, nil
}

// The check follows the profile of the template: one run per claimed
// standard, compliant only if all pass.
func TestConformanceProfile(t *testing.T) {
	rec := &recorder{}
	c := NewCore(rec, nil, t.TempDir())
	if _, err := NewProjectService(c).Open(muster); err != nil {
		t.Fatal(err)
	}
	c.doc.Pkg.Template.Root.SetAttr("profile", "pdfa-3a,pdfua-1")
	fr := &flavourRunner{}
	c.conf.detect = func(context.Context, func(string)) (verapdf.Runner, error) { return fr, nil }
	st, err := NewConformanceService(c).Check()
	if err != nil || st.Profile != "pdfa-3a,pdfua-1" {
		t.Fatalf("Start: %+v %v", st, err)
	}
	st = waitConformance(t, rec, 10*time.Second)
	if st.State != ConformanceDone || st.Compliant || len(st.Findings) != 2 || st.Findings[1].Specification != "Norm ua1" {
		t.Fatalf("Ergebnis: %+v", st)
	}
	if !slices.Equal(fr.seen, []string{"3a", "ua1"}) {
		t.Errorf("Flavours %v", fr.seen)
	}
}

// TestConformanceVeraPDF checks the sample invoice with the real veraPDF
// (local or Docker). Only with PDX_VERAPDF_TEST=1, the run takes a while.
func TestConformanceVeraPDF(t *testing.T) {
	if os.Getenv("PDX_VERAPDF_TEST") == "" {
		t.Skip("PDX_VERAPDF_TEST nicht gesetzt")
	}
	rec := &recorder{}
	c := NewCore(rec, nil, t.TempDir())
	if _, err := NewProjectService(c).Open(muster); err != nil {
		t.Fatal(err)
	}
	if _, err := NewConformanceService(c).Check(); err != nil {
		t.Fatal(err)
	}
	st := waitConformance(t, rec, 5*time.Minute)
	if st.State != ConformanceDone || !st.Compliant {
		t.Fatalf("veraPDF: %+v", st)
	}
	t.Logf("%s, %d ms", st.Runner, st.Millis)
}

// Without data XSD and samples the placeholder is built from the data root
// of the template with its namespace.
func TestPlaceholderFromDataRoot(t *testing.T) {
	c := NewCore(&recorder{}, nil, t.TempDir())
	if _, err := NewProjectService(c).Open("../../../testdata/templates/cii-rechnung"); err != nil {
		t.Fatal(err)
	}
	got := string(c.placeholderLocked())
	want := `<rsm:CrossIndustryInvoice xmlns:rsm="urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100"/>`
	if got != want {
		t.Errorf("%s", got)
	}
}

// A chosen path to veraPDF is checked, stored and used, an invalid
// one is not accepted, empty switches back to the search.
func TestVeraPDFSetting(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skript mit Shebang")
	}
	dir := t.TempDir()
	cli := filepath.Join(dir, "verapdf")
	os.WriteFile(cli, []byte("#!/bin/sh\necho \"veraPDF "+verapdf.Version+"\"\n"), 0o755)
	c := NewCore(nil, nil, t.TempDir())
	s := NewConformanceService(c)
	st, err := s.SetVeraPDF(cli)
	if err != nil || st.Path != cli || !strings.Contains(st.Runner, cli) || st.Error != "" {
		t.Fatalf("gültiger Pfad: %+v %v", st, err)
	}
	if got := NewConformanceService(c).VeraPDF(); got.Path != cli {
		t.Errorf("nicht gespeichert: %+v", got)
	}
	if _, err := s.SetVeraPDF(filepath.Join(dir, "fehlt")); err == nil {
		t.Error("ungültiger Pfad angenommen")
	}
	if s.VeraPDF().Path != cli {
		t.Error("ungültiger Pfad hat die Einstellung überschrieben")
	}
	if st, err := s.SetVeraPDF(""); err != nil || st.Path != "" {
		t.Errorf("zurück auf automatisch: %+v %v", st, err)
	}
}
