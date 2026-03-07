package main

import (
	"strings"
	"testing"
)

func TestNormalizeAction_Valid(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "start", want: "start"},
		{in: " STOP ", want: "stop"},
		{in: "ReLoAd", want: "reload"},
		{in: "ENABLE", want: "enable"},
		{in: " disable ", want: "disable"},
		{in: "toggle", want: "toggle"},
		{in: "status", want: "status"},
	}

	for _, tt := range tests {
		got, err := normalizeAction(tt.in)
		if err != nil {
			t.Fatalf("normalizeAction(%q) unexpected error: %v", tt.in, err)
		}
		if got != tt.want {
			t.Fatalf("normalizeAction(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNormalizeAction_Invalid(t *testing.T) {
	tests := []string{"", "  ", "unknown", "toggle-now"}
	for _, in := range tests {
		if _, err := normalizeAction(in); err == nil {
			t.Fatalf("normalizeAction(%q) expected error", in)
		}
	}
}

func TestActionGate(t *testing.T) {
	var g actionGate
	if !g.tryBegin() {
		t.Fatal("first tryBegin should succeed")
	}
	if g.tryBegin() {
		t.Fatal("second tryBegin should fail while busy")
	}
	g.end()
	if !g.tryBegin() {
		t.Fatal("tryBegin should succeed after end")
	}
}

func TestNormalizeServiceName_Valid(t *testing.T) {
	tests := []string{"gov-pass", "gov_pass", "gov.pass", "gov-pass@prod", "GovPass1"}
	for _, in := range tests {
		got, err := normalizeServiceName(in)
		if err != nil {
			t.Fatalf("normalizeServiceName(%q) unexpected error: %v", in, err)
		}
		if got != strings.TrimSpace(in) {
			t.Fatalf("normalizeServiceName(%q) = %q", in, got)
		}
	}
}

func TestNormalizeServiceName_Invalid(t *testing.T) {
	tests := []string{"", "  ", "-gov-pass", "gov pass", "gov/pass", "gov\npass"}
	for _, in := range tests {
		if _, err := normalizeServiceName(in); err == nil {
			t.Fatalf("normalizeServiceName(%q) expected error", in)
		}
	}
}
