package driver

import "testing"

func TestReportHealthy(t *testing.T) {
	report := Report{
		FilesPresent:                 true,
		ServiceExists:                true,
		ServiceBinPath:               `C:\Program Files\gov-pass\WinDivert64.sys`,
		ServiceBinPathExists:         true,
		ServiceBinPathMatchesDesired: true,
	}
	if !report.Healthy() {
		t.Fatalf("expected healthy report, got issues: %v", report.Issues())
	}
}

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
