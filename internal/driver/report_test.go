package driver

import "testing"

func TestReportIssues(t *testing.T) {
	report := Report{
		FilesPresent:                 false,
		ServiceExists:                true,
		ServiceBinPath:               "",
		ServiceBinPathExists:         false,
		ServiceBinPathMatchesDesired: false,
	}
	issues := report.Issues()
	if len(issues) != 4 {
		t.Fatalf("expected 4 issues, got %d: %v", len(issues), issues)
	}
}

func TestReportMissingServiceShortCircuits(t *testing.T) {
	report := Report{
		FilesPresent: false,
	}
	issues := report.Issues()
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d: %v", len(issues), issues)
	}
}
