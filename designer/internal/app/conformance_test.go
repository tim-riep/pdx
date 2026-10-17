package app

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/tim-riep/pdx/verapdf"
)

// fakeRunner meldet je Operator der ersten Seite eine verletzte Regel.
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

// TestConformanceVeraPDF prüft die Musterrechnung mit dem echten veraPDF
// (lokal oder Docker). Nur mit PDX_VERAPDF_TEST=1, der Lauf dauert.
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
