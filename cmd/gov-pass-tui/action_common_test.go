package main

import "testing"

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
