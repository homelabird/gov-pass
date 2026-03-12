package main

import (
	"strings"
	"testing"
)

func TestTUIActionsForStatus_BusyStateDisablesToggle(t *testing.T) {
	actions := tuiActionsForStatus(tuiStatus{
		RawState:     "reloading",
		ActiveKnown:  true,
		EnabledKnown: true,
		Capabilities: tuiCapabilities{Reload: true},
	})

	if actions[0].Label != "Service busy" {
		t.Fatalf("service action when busy = %q", actions[0].Label)
	}
}

func TestResolveServiceToggleAction_BusyStateErrors(t *testing.T) {
	_, err := resolveServiceToggleAction("gov-pass", tuiStatus{
		RawState:    "reloading",
		Active:      false,
		ActiveKnown: true,
	})
	if err == nil {
		t.Fatal("expected busy-state toggle to fail")
	}
	if !strings.Contains(err.Error(), "refresh and retry") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveServiceToggleAction_FailedStateStarts(t *testing.T) {
	command, err := resolveServiceToggleAction("gov-pass", tuiStatus{
		RawState:    "failed",
		Active:      false,
		ActiveKnown: true,
	})
	if err != nil {
		t.Fatalf("resolveServiceToggleAction unexpected error: %v", err)
	}
	if command != "start" {
		t.Fatalf("resolveServiceToggleAction = %q, want start", command)
	}
}
