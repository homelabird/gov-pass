package main

import (
	"errors"
	"strings"
	"testing"
)

func TestRenderPlainTUIView(t *testing.T) {
	status := newTUIStatus(tuiStatusInput{
		Platform:     "Linux",
		ServiceName:  "gov-pass",
		RawState:     "active",
		Active:       true,
		ActiveKnown:  true,
		Enabled:      false,
		EnabledKnown: true,
		Capabilities: tuiCapabilities{Reload: true},
	}, statusIssue{Label: "service", Err: errors.New("systemctl is-active gov-pass failed")})

	view := renderPlainTUIViewForSize(status, successTUIFeedback("Service restart requested.", "Target service: gov-pass", false), 100, 40)

	for _, want := range []string{
		"GOV-PASS OPERATOR PANEL [DEGRADED]",
		"Platform : Linux",
		"Service  : gov-pass",
		"Health   : DEGRADED",
		"State    : Running",
		"Boot     : Disabled",
		"Reload   : Supported",
		"[1] Stop service",
		"[3] Enable boot",
		"[4] Reload service",
		"Summary  : Service check needs attention.",
		"Detail   : Service check: systemctl is-active",
		"gov-pass failed",
		"Level    : OK",
		"Summary  : Service restart requested.",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("rendered view missing %q:\n%s", want, view)
		}
	}
}

func TestRenderPlainTUIViewForWidth_WrapsLongLines(t *testing.T) {
	status := newTUIStatus(tuiStatusInput{
		Platform:     "Linux",
		ServiceName:  "gov-pass",
		RawState:     "active",
		Active:       true,
		ActiveKnown:  true,
		Enabled:      true,
		EnabledKnown: true,
		Capabilities: tuiCapabilities{Reload: true},
	}, statusIssue{Label: "service", Err: errors.New("a very long status error that should wrap cleanly across multiple lines")})

	view := renderPlainTUIViewForSize(status, infoTUIFeedback("Service restart requested.", "The operator panel should wrap this detail line cleanly across multiple rows."), 44, 36)

	for _, want := range []string{
		"Detail   : The operator panel should",
		"wrap this detail line cleanly",
		"across multiple rows.",
		"Summary  : Service check needs attention.",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("wrapped view missing %q:\n%s", want, view)
		}
	}
}

func TestBuildCompactStatusSummary(t *testing.T) {
	status := newTUIStatus(tuiStatusInput{
		Platform:     "Windows",
		ServiceName:  "gov-pass",
		RawState:     "running",
		Active:       true,
		ActiveKnown:  true,
		Enabled:      true,
		EnabledKnown: true,
		Capabilities: tuiCapabilities{Reload: true},
	}, statusIssue{Label: "service", Err: errors.New("query failed")}, statusIssue{Label: "boot", Err: errors.New("config failed")})

	summary := buildCompactStatusSummary(status, successTUIFeedback("Service reload requested.", "", false))

	for _, want := range []string{
		"Health: DEGRADED",
		"Service: gov-pass",
		"State: Running",
		"Boot: Enabled",
		"Reload: Supported",
		"Warning: 2 checks need attention.",
		"Last: Service reload requested.",
		"Choose an action:",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, summary)
		}
	}
}

func TestTUIActionsForStatus_ReflectsServiceAndBootState(t *testing.T) {
	activeActions := tuiActionsForStatus(tuiStatus{
		Active:       true,
		ActiveKnown:  true,
		Enabled:      true,
		EnabledKnown: true,
		Capabilities: tuiCapabilities{Reload: true},
	})
	if activeActions[0].Label != "Stop service" {
		t.Fatalf("service action when active = %q", activeActions[0].Label)
	}
	if activeActions[2].Label != "Disable boot" {
		t.Fatalf("boot action when enabled = %q", activeActions[2].Label)
	}
	if activeActions[3].Label != "Reload service" {
		t.Fatalf("reload action when supported = %q", activeActions[3].Label)
	}

	unknownActions := tuiActionsForStatus(tuiStatus{
		ActiveKnown:  false,
		EnabledKnown: false,
		Capabilities: tuiCapabilities{Reload: false},
	})
	if unknownActions[0].Label != "Toggle service" {
		t.Fatalf("service action when unknown = %q", unknownActions[0].Label)
	}
	if unknownActions[2].Label != "Toggle boot" {
		t.Fatalf("boot action when unknown = %q", unknownActions[2].Label)
	}
	for _, action := range unknownActions {
		if action.Label == "Reload service" {
			t.Fatal("reload action should be omitted when unsupported")
		}
	}
}
