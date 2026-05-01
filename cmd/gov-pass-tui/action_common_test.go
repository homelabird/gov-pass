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

func TestNormalizeUnixServiceName_Valid(t *testing.T) {
	tests := []string{"gov-pass", "gov_pass", "gov.pass", "gov-pass@prod", "GovPass1"}
	for _, in := range tests {
		got, err := normalizeUnixServiceName(in)
		if err != nil {
			t.Fatalf("normalizeUnixServiceName(%q) unexpected error: %v", in, err)
		}
		if got != strings.TrimSpace(in) {
			t.Fatalf("normalizeUnixServiceName(%q) = %q", in, got)
		}
	}
}

func TestNormalizeUnixServiceName_Invalid(t *testing.T) {
	tests := []string{"", "  ", "-gov-pass", ".service", "service.", "@gov-pass", "gov-pass@", ".", "..", "gov pass", "gov/pass", "gov\npass"}
	for _, in := range tests {
		if _, err := normalizeUnixServiceName(in); err == nil {
			t.Fatalf("normalizeUnixServiceName(%q) expected error", in)
		}
	}
}

func TestNormalizeWindowsServiceName_Valid(t *testing.T) {
	tests := []string{"gov-pass", "GovPass_Prod", "GovPass1"}
	for _, in := range tests {
		got, err := normalizeWindowsServiceName(in)
		if err != nil {
			t.Fatalf("normalizeWindowsServiceName(%q) unexpected error: %v", in, err)
		}
		if got != strings.TrimSpace(in) {
			t.Fatalf("normalizeWindowsServiceName(%q) = %q", in, got)
		}
	}
}

func TestNormalizeWindowsServiceName_Invalid(t *testing.T) {
	tests := []string{"", "  ", "-gov-pass", "Gov Pass", "Gov.Pass", "GovPass@Lab", "bad\nname", "bad\rname", strings.Repeat("a", 64)}
	for _, in := range tests {
		if _, err := normalizeWindowsServiceName(in); err == nil {
			t.Fatalf("normalizeWindowsServiceName(%q) expected error", in)
		}
	}
}
