package main

import (
	"errors"
	"strings"
	"testing"
)

func TestBuildStatusWarning(t *testing.T) {
	warning := buildStatusWarning(
		statusIssue{Label: "service", Err: errors.New("query failed")},
		statusIssue{Label: "boot", Err: errors.New("config failed")},
	)

	for _, want := range []string{
		"service: query failed",
		"boot: config failed",
	} {
		if !strings.Contains(warning, want) {
			t.Fatalf("warning missing %q: %q", want, warning)
		}
	}
}

func TestNewTUIStatus_DefaultsUnknownState(t *testing.T) {
	status := newTUIStatus(
		"Linux",
		"gov-pass",
		"",
		false,
		true,
		statusIssue{Label: "service", Err: errors.New("systemctl missing")},
	)

	if status.State != "unknown" {
		t.Fatalf("unexpected state: %q", status.State)
	}
	if status.Warning != "service: systemctl missing" {
		t.Fatalf("unexpected warning: %q", status.Warning)
	}
}
