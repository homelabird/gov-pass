//go:build linux

package main

import (
	"strings"
	"testing"
)

func TestSanitizedLinuxTUICommandEnvDoesNotInheritCallerEnv(t *testing.T) {
	t.Setenv("LD_PRELOAD", "bad")
	t.Setenv("TERM", "xterm-256color")
	env := sanitizedLinuxTUICommandEnv()
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "LD_PRELOAD=bad") {
		t.Fatalf("sanitized env leaked caller environment: %v", env)
	}
	for _, want := range []string{"PATH=", "LANG=C", "LC_ALL=C", "TERM=xterm-256color"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("sanitized env missing %q: %v", want, env)
		}
	}
}

func TestSafeLinuxTUITermRejectsUnsafeValues(t *testing.T) {
	for _, value := range []string{"xterm;bad", "../xterm", strings.Repeat("a", 65), ""} {
		if got := safeLinuxTUITerm(value); got != "xterm-256color" {
			t.Fatalf("safeLinuxTUITerm(%q) = %q, want fallback", value, got)
		}
	}
	if got := safeLinuxTUITerm("screen.xterm-256color"); got != "screen.xterm-256color" {
		t.Fatalf("safeLinuxTUITerm preserved value = %q", got)
	}
}
