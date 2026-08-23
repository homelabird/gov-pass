package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFreeBSDRcVarName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "default", in: "gov-pass", want: "gov_pass_enable"},
		{name: "already safe", in: "gov_pass", want: "gov_pass_enable"},
		{name: "mixed punctuation", in: "gov.pass@lab", want: "gov_pass_lab_enable"},
		{name: "trimmed", in: "  gov-pass  ", want: "gov_pass_enable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := freeBSDRcVarName(tt.in); got != tt.want {
				t.Fatalf("freeBSDRcVarName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestResolveTrustedFreeBSDCommandRejectsSymlinkEscape(t *testing.T) {
	trustedDir := t.TempDir()
	target := filepath.Join(t.TempDir(), "service")
	if err := os.WriteFile(target, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write target command: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(trustedDir, "service")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	origDirs := trustedFreeBSDCommandDirs
	trustedFreeBSDCommandDirs = []string{trustedDir}
	t.Cleanup(func() { trustedFreeBSDCommandDirs = origDirs })

	if path, err := resolveTrustedFreeBSDCommand("service"); err == nil {
		t.Fatalf("expected symlink escape rejection, got %q", path)
	}
}

func TestSanitizedFreeBSDCommandEnvDoesNotInheritCallerEnv(t *testing.T) {
	t.Setenv("LD_PRELOAD", "bad")
	env := sanitizedFreeBSDCommandEnv()
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "LD_PRELOAD=bad") {
		t.Fatalf("sanitized env leaked caller environment: %v", env)
	}
	if !strings.Contains(joined, "PATH=") || !strings.Contains(joined, "HOME=/root") {
		t.Fatalf("sanitized env missing required defaults: %v", env)
	}
}
