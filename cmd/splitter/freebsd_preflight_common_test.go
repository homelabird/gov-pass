package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveTrustedFreeBSDSplitterCommandRejectsSymlinkOutsideTrustedDir(t *testing.T) {
	trustedDir := t.TempDir()
	outsideDir := t.TempDir()
	target := filepath.Join(outsideDir, "pfctl")
	if err := os.WriteFile(target, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write target command: %v", err)
	}
	link := filepath.Join(trustedDir, "pfctl")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	origDirs := trustedFreeBSDSplitterCommandDirs
	trustedFreeBSDSplitterCommandDirs = []string{trustedDir}
	defer func() {
		trustedFreeBSDSplitterCommandDirs = origDirs
	}()

	if got, err := resolveTrustedFreeBSDSplitterCommand("pfctl"); err == nil {
		t.Fatalf("expected symlink target outside trusted dir to be rejected, got %q", got)
	}
}

func TestSanitizedFreeBSDSplitterCommandEnvDoesNotInheritCallerEnv(t *testing.T) {
	t.Setenv("LD_PRELOAD", "bad")
	env := sanitizedFreeBSDSplitterCommandEnv()
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "LD_PRELOAD=bad") {
		t.Fatalf("sanitized env leaked caller environment: %v", env)
	}
	if !strings.Contains(joined, "PATH=") || !strings.Contains(joined, "HOME=/root") {
		t.Fatalf("sanitized env missing required defaults: %v", env)
	}
}

func TestFreeBSDPFStatusEnabled(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{
			name: "enabled",
			text: "Status: Enabled for 0 days 00:10:00\nDebug: Urgent",
			want: true,
		},
		{
			name: "disabled",
			text: "Status: Disabled\nDebug: Urgent",
			want: false,
		},
		{
			name: "missing",
			text: "No ALTQ support in kernel",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := freeBSDPFStatusEnabled(tt.text); got != tt.want {
				t.Fatalf("freeBSDPFStatusEnabled() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestFreeBSDPFAnchorRulesLoaded(t *testing.T) {
	if !freeBSDPFAnchorRulesLoaded("pass out quick inet proto tcp to any port = https divert-to 127.0.0.1 port 10000\n") {
		t.Fatal("expected non-empty rules to be loaded")
	}
	if freeBSDPFAnchorRulesLoaded(" \n\t") {
		t.Fatal("expected empty rules output to be unloaded")
	}
}
