package main

import (
	"errors"
	"strings"
	"testing"
)

func TestBuildStatusWarning(t *testing.T) {
	summary, details := buildStatusWarning(
		statusIssue{Label: "service", Err: errors.New("query failed")},
		statusIssue{Label: "boot", Err: errors.New("config failed")},
	)

	if summary != "2 checks need attention." {
		t.Fatalf("unexpected summary: %q", summary)
	}
	for _, want := range []string{
		"Service check: query failed",
		"Boot check: config failed",
	} {
		found := false
		for _, detail := range details {
			if strings.Contains(detail, want) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("warning details missing %q: %#v", want, details)
		}
	}
}

func TestNewTUIStatus_DefaultsUnknownState(t *testing.T) {
	status := newTUIStatus(tuiStatusInput{
		Platform:     "Linux",
		ServiceName:  "gov-pass",
		RawState:     "",
		Active:       false,
		ActiveKnown:  false,
		Enabled:      true,
		EnabledKnown: true,
	}, statusIssue{Label: "service", Err: errors.New("systemctl missing")})

	if status.RawState != "unknown" {
		t.Fatalf("unexpected raw state: %q", status.RawState)
	}
	if status.StateLabel != "Unknown" {
		t.Fatalf("unexpected state label: %q", status.StateLabel)
	}
	if status.WarningSummary != "Service check needs attention." {
		t.Fatalf("unexpected warning summary: %q", status.WarningSummary)
	}
	if status.BootLabel != "Enabled" {
		t.Fatalf("unexpected boot label: %q", status.BootLabel)
	}
}
