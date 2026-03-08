package main

import (
	"strings"
	"testing"
)

func TestRenderPlainTUIView(t *testing.T) {
	view := renderPlainTUIView(tuiStatus{
		Platform:    "Linux",
		ServiceName: "gov-pass",
		State:       "active",
		Active:      true,
		Enabled:     false,
		Warning:     "service: systemctl is-active gov-pass failed",
	}, "Service restarted: gov-pass")

	for _, want := range []string{
		"GOV-PASS CONTROL",
		"STATUS WARNING",
		"Platform : LINUX",
		"Service  : gov-pass",
		"State    : ACTIVE",
		"Signals  : [ACTIVE] [BOOT OFF]",
		"[1] Toggle service",
		"[r] Refresh",
		"Tone    : INFO",
		"Notice  : service: systemctl is-active gov-pass failed",
		"Text    : Service restarted: gov-pass",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("rendered view missing %q:\n%s", want, view)
		}
	}
}

func TestRenderPlainTUIViewForWidth_WrapsLongLines(t *testing.T) {
	view := renderPlainTUIViewForWidth(tuiStatus{
		Platform:    "Linux",
		ServiceName: "gov-pass",
		State:       "active",
		Active:      true,
		Enabled:     true,
		Warning:     "service: a very long status error that should wrap cleanly across multiple lines",
	}, "Service restart requested while the operator panel is still open.", 44)

	for _, want := range []string{
		"Notice  : service: a very long status",
		"error that should wrap cleanly",
		"across multiple lines",
		"Text    : Service restart requested",
		"while the operator panel is",
		"still open.",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("wrapped view missing %q:\n%s", want, view)
		}
	}
}

func TestBuildCompactStatusSummary(t *testing.T) {
	summary := buildCompactStatusSummary(tuiStatus{
		Platform:    "Windows",
		ServiceName: "gov-pass",
		State:       "running",
		Active:      true,
		Enabled:     true,
		Warning:     "service: query failed\nboot: config failed",
	})

	for _, want := range []string{
		"Platform: Windows",
		"Service: gov-pass",
		"State: running",
		"Signals: [ACTIVE] [BOOT ON]",
		"Warning: service: query failed",
		"         boot: config failed",
		"Choose an action:",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, summary)
		}
	}
}
