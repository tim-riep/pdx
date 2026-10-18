package verapdf

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
)

func TestParseNonconforming(t *testing.T) {
	f, err := os.Open("testdata/nonconforming.mrr.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	reps, err := ParseMRR(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(reps) != 1 {
		t.Fatalf("%d Berichte", len(reps))
	}
	r := reps[0]
	if r.Version != "1.30.3" || r.Compliant || !r.Failed() || r.FailedRules != 2 || r.FailedChecks != 7 {
		t.Errorf("unerwartete Kopfdaten: %+v", r)
	}
	if len(r.Findings) != 2 {
		t.Fatalf("%d Befunde statt 2", len(r.Findings))
	}
	f0 := r.Findings[0]
	if f0.Specification != "ISO 19005-3:2012" || f0.Clause != "6.6.2.1" || f0.TestNumber != "1" || len(f0.Checks) != 1 {
		t.Errorf("erster Befund: %+v", f0)
	}
	if !strings.HasPrefix(f0.Checks[0].Context, "root/document[0]") {
		t.Errorf("Kontext fehlt: %+v", f0.Checks)
	}
	var b bytes.Buffer
	r.WriteText(&b)
	for _, want := range []string{"nicht konform", "Klausel 6.2.4.3 Test 4", "und 1 weitere", "operators[1]/colorSpace[0]"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("Textausgabe ohne %q:\n%s", want, b.String())
		}
	}
}

func TestParseConformingAndException(t *testing.T) {
	const ok = `<report><buildInformation><releaseDetails id="core" version="1.30.3"/></buildInformation><jobs>
<job><item><name>/data/a.pdf</name></item><validationReport profileName="PDF/A-3b validation profile" isCompliant="true">
<details passedRules="146" failedRules="0" passedChecks="900" failedChecks="0"/></validationReport></job>
<job><item><name>/data/b.pdf</name></item><taskException type="PARSE" isExecuted="true" isSuccess="false"><exceptionMessage>kaputt</exceptionMessage></taskException></job>
</jobs></report>`
	reps, err := ParseMRR(strings.NewReader(ok))
	if err != nil {
		t.Fatal(err)
	}
	if reps[0].Failed() {
		t.Errorf("konformer Bericht gilt als fehlgeschlagen: %+v", reps[0])
	}
	if !reps[1].Failed() || !strings.Contains(reps[1].Exception, "kaputt") {
		t.Errorf("Ausnahme nicht erkannt: %+v", reps[1])
	}
}

func TestFlavour(t *testing.T) {
	if f, err := Flavour("pdfa-3b"); err != nil || f != "3b" {
		t.Errorf("Flavour(pdfa-3b) = %q, %v", f, err)
	}
	if _, err := Flavour("pdf-1.7"); err == nil {
		t.Error("pdf-1.7 hat keinen Flavour")
	}
}

// TestRunnerLive läuft nur mit PDX_CONFORMANCE=1, weil er Docker oder eine
// lokale veraPDF-CLI braucht.
func TestRunnerLive(t *testing.T) {
	if os.Getenv("PDX_CONFORMANCE") != "1" {
		t.Skip("PDX_CONFORMANCE=1 setzen, um veraPDF aufzurufen")
	}
	r, err := Detect(context.Background(), func(s string) { t.Log(s) })
	if err != nil {
		t.Fatal(err)
	}
	rep, err := r.Validate(context.Background(), "../pdf/render/testdata/text.pdf", "3b")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Version != Version || !rep.Failed() {
		t.Errorf("unerwarteter Bericht: %+v", rep)
	}
}

func TestFlavours(t *testing.T) {
	got, err := Flavours("pdfa-3b")
	if err != nil || len(got) != 1 || got[0] != "3b" {
		t.Fatalf("pdfa-3b: %v %v", got, err)
	}
	if _, err := Flavours("pdfa-3b,pdf-1.7,gibtsnicht"); err == nil || !strings.Contains(err.Error(), "pdf-1.7") || !strings.Contains(err.Error(), "gibtsnicht") {
		t.Fatalf("Fehler nicht vollständig: %v", err)
	}
	if _, err := Flavours(" , "); err == nil {
		t.Fatal("leere Angabe angenommen")
	}
}
