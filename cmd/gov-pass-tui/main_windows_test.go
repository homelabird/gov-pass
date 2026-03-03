//go:build windows

package main

import (
	"testing"

	"golang.org/x/sys/windows/svc"
)

func TestQuoteArg(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "", want: `""`},
		{in: "simple", want: "simple"},
		{in: "two words", want: `"two words"`},
		{in: `a"b`, want: `"a\"b"`},
	}

	for _, tt := range tests {
		got := quoteArg(tt.in)
		if got != tt.want {
			t.Fatalf("quoteArg(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestQuoteArgs(t *testing.T) {
	got := quoteArgs([]string{"--service-name", "gov pass", "--action", "reload"})
	want := `--service-name "gov pass" --action reload`
	if got != want {
		t.Fatalf("quoteArgs mismatch: got %q want %q", got, want)
	}
}

func TestStateString(t *testing.T) {
	if got := stateString(svc.Running); got != "running" {
		t.Fatalf("running mismatch: %q", got)
	}
	if got := stateString(svc.Stopped); got != "stopped" {
		t.Fatalf("stopped mismatch: %q", got)
	}
	if got := stateString(svc.StartPending); got != "start-pending" {
		t.Fatalf("start-pending mismatch: %q", got)
	}
}
