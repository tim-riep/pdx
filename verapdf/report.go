package verapdf

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// Report is the result of a check.
type Report struct {
	File         string
	Version      string
	Profile      string
	Compliant    bool
	PassedRules  int
	FailedRules  int
	PassedChecks int
	FailedChecks int
	Findings     []Finding
	// Exception contains an error message if veraPDF could not check the
	// file, for example because it is not readable.
	Exception string
}

// Finding is a failed rule.
type Finding struct {
	Specification string
	Clause        string
	TestNumber    string
	Description   string
	Object        string
	Test          string
	FailedChecks  int
	Checks        []Check
}

// Check is a single failed check with object context.
type Check struct {
	Context string
	Message string
}

// Failed reports whether at least one test failed or the check could
// not be performed.
func (r *Report) Failed() bool {
	return !r.Compliant || r.FailedChecks > 0 || r.FailedRules > 0 || r.Exception != ""
}

type mrrReport struct {
	Releases []struct {
		ID      string `xml:"id,attr"`
		Version string `xml:"version,attr"`
	} `xml:"buildInformation>releaseDetails"`
	Jobs []struct {
		Name       string `xml:"item>name"`
		Validation *struct {
			Profile   string `xml:"profileName,attr"`
			Compliant bool   `xml:"isCompliant,attr"`
			Details   struct {
				PassedRules  int `xml:"passedRules,attr"`
				FailedRules  int `xml:"failedRules,attr"`
				PassedChecks int `xml:"passedChecks,attr"`
				FailedChecks int `xml:"failedChecks,attr"`
				Rules        []struct {
					Specification string `xml:"specification,attr"`
					Clause        string `xml:"clause,attr"`
					TestNumber    string `xml:"testNumber,attr"`
					Status        string `xml:"status,attr"`
					FailedChecks  int    `xml:"failedChecks,attr"`
					Description   string `xml:"description"`
					Object        string `xml:"object"`
					Test          string `xml:"test"`
					Checks        []struct {
						Status  string `xml:"status,attr"`
						Context string `xml:"context"`
						Message string `xml:"errorMessage"`
					} `xml:"check"`
				} `xml:"rule"`
			} `xml:"details"`
		} `xml:"validationReport"`
		TaskException []struct {
			Type      string `xml:"type,attr"`
			Exception string `xml:"exceptionMessage"`
		} `xml:"taskException"`
	} `xml:"jobs>job"`
}

// ParseMRR reads a veraPDF report in mrr format and returns one
// Report per job.
func ParseMRR(r io.Reader) ([]*Report, error) {
	var m mrrReport
	if err := xml.NewDecoder(r).Decode(&m); err != nil {
		return nil, fmt.Errorf("mrr-Bericht: %w", err)
	}
	version := ""
	for _, rel := range m.Releases {
		if rel.ID == "core" {
			version = rel.Version
		}
	}
	var out []*Report
	for _, j := range m.Jobs {
		rep := &Report{File: j.Name, Version: version}
		for _, te := range j.TaskException {
			rep.Exception = strings.TrimSpace(te.Type + ": " + te.Exception)
		}
		if v := j.Validation; v != nil {
			rep.Profile = v.Profile
			rep.Compliant = v.Compliant
			d := v.Details
			rep.PassedRules, rep.FailedRules = d.PassedRules, d.FailedRules
			rep.PassedChecks, rep.FailedChecks = d.PassedChecks, d.FailedChecks
			for _, ru := range d.Rules {
				if ru.Status != "failed" {
					continue
				}
				f := Finding{
					Specification: ru.Specification, Clause: ru.Clause, TestNumber: ru.TestNumber,
					Description: strings.Join(strings.Fields(ru.Description), " "),
					Object:      ru.Object, Test: ru.Test, FailedChecks: ru.FailedChecks,
				}
				for _, c := range ru.Checks {
					if c.Status == "failed" {
						f.Checks = append(f.Checks, Check{Context: c.Context, Message: strings.TrimSpace(c.Message)})
					}
				}
				rep.Findings = append(rep.Findings, f)
			}
		} else if rep.Exception == "" {
			rep.Exception = "veraPDF hat keinen Prüfbericht geliefert"
		}
		out = append(out, rep)
	}
	return out, nil
}

// MaxContexts limits the reported object contexts per rule.
const MaxContexts = 5

// WriteText writes the report in readable form.
func (r *Report) WriteText(w io.Writer) {
	fmt.Fprintf(w, "veraPDF %s, %s\n", r.Version, r.Profile)
	if r.Exception != "" {
		fmt.Fprintf(w, "Prüfung nicht möglich: %s\n", r.Exception)
		return
	}
	if !r.Failed() {
		fmt.Fprintf(w, "Ergebnis: konform (%d Regeln, %d Prüfungen bestanden)\n", r.PassedRules, r.PassedChecks)
		return
	}
	fmt.Fprintf(w, "Ergebnis: nicht konform (%d Regeln und %d Prüfungen fehlgeschlagen)\n", r.FailedRules, r.FailedChecks)
	for _, f := range r.Findings {
		fmt.Fprintf(w, "\n%s Klausel %s Test %s (%d fehlgeschlagene Prüfungen)\n", f.Specification, f.Clause, f.TestNumber, f.FailedChecks)
		fmt.Fprintf(w, "  %s\n", f.Description)
		if f.Test != "" {
			fmt.Fprintf(w, "  Bedingung: %s (Objekt %s)\n", f.Test, f.Object)
		}
		for i, c := range f.Checks {
			if i == MaxContexts {
				fmt.Fprintf(w, "  und %d weitere\n", len(f.Checks)-MaxContexts)
				break
			}
			fmt.Fprintf(w, "  bei %s\n", c.Context)
			if c.Message != "" {
				fmt.Fprintf(w, "    %s\n", c.Message)
			}
		}
	}
}
