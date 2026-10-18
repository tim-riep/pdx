package dssval

import "testing"

// parseReport liest Signaturen samt eingebetteter Zeitstempel und
// Dokument-Zeitstempel aus dem einfachen Bericht.
func TestParseReport(t *testing.T) {
	raw := []byte(`{"SimpleReport":{"signatureOrTimestampOrEvidenceRecord":[
		{"Signature":{"Id":"S-1","SignatureFormat":"PAdES-BASELINE-LTA","Indication":"TOTAL_PASSED",
			"AdESValidationDetails":{"Warning":[{"value":"w"}]},
			"Timestamps":{"timestamp":[{"Id":"T-1","Indication":"PASSED"}]}}},
		{"Timestamp":{"Id":"T-2","Indication":"INDETERMINATE","SubIndication":"NO_POE",
			"AdESValidationDetails":{"Error":[{"value":"e"}]}}}]}}`)
	sigs, err := parseReport(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(sigs) != 3 {
		t.Fatalf("%d Einträge", len(sigs))
	}
	if s := sigs[0]; s.Type != "Signature" || s.Format != "PAdES-BASELINE-LTA" || s.Indication != "TOTAL_PASSED" || len(s.Warnings) != 1 {
		t.Errorf("Signatur: %+v", s)
	}
	if s := sigs[1]; s.Type != "Timestamp" || s.ID != "T-1" || s.Indication != "PASSED" {
		t.Errorf("eingebetteter Zeitstempel: %+v", s)
	}
	if s := sigs[2]; s.SubIndication != "NO_POE" || len(s.Errors) != 1 {
		t.Errorf("Dokument-Zeitstempel: %+v", s)
	}
	if _, err := parseReport([]byte(`{"SimpleReport":{}}`)); err == nil {
		t.Error("leerer Bericht nicht gemeldet")
	}
}
