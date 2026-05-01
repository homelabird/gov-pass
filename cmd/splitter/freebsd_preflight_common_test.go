package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLookTrustedFreeBSDSplitterCommandRejectsPoisonedPATHEntry(t *testing.T) {
	dir := t.TempDir()
	cmd := filepath.Join(dir, "pfctl")
	if err := os.WriteFile(cmd, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake command: %v", err)
	}
	t.Setenv("PATH", dir)

	origDirs := trustedFreeBSDSplitterCommandDirs
	trustedFreeBSDSplitterCommandDirs = []string{filepath.Join(dir, "missing")}
	defer func() {
		trustedFreeBSDSplitterCommandDirs = origDirs
	}()

	if got, ok := lookTrustedFreeBSDSplitterCommand("pfctl"); ok {
		t.Fatalf("expected poisoned PATH entry to be rejected, got %q", got)
	}
}

func TestLookTrustedFreeBSDSplitterCommandUsesTrustedAbsoluteDir(t *testing.T) {
	dir := t.TempDir()
	cmd := filepath.Join(dir, "pfctl")
	if err := os.WriteFile(cmd, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake command: %v", err)
	}

	origDirs := trustedFreeBSDSplitterCommandDirs
	trustedFreeBSDSplitterCommandDirs = []string{dir}
	defer func() {
		trustedFreeBSDSplitterCommandDirs = origDirs
	}()

	got, ok := lookTrustedFreeBSDSplitterCommand("pfctl")
	if !ok {
		t.Fatal("expected trusted command lookup to succeed")
	}
	if filepath.Clean(got) != filepath.Clean(cmd) {
		t.Fatalf("lookTrustedFreeBSDSplitterCommand returned %q, want %q", got, cmd)
	}

	resolved, err := resolveTrustedFreeBSDSplitterCommand("pfctl")
	if err != nil {
		t.Fatalf("resolveTrustedFreeBSDSplitterCommand: %v", err)
	}
	if filepath.Clean(resolved) != filepath.Clean(cmd) {
		t.Fatalf("resolveTrustedFreeBSDSplitterCommand returned %q, want %q", resolved, cmd)
	}
	if !isTrustedFreeBSDSplitterCommandPath(cmd) {
		t.Fatalf("isTrustedFreeBSDSplitterCommandPath(%q) = false, want true", cmd)
	}
}

func TestLookTrustedFreeBSDSplitterCommandRejectsSymlinkOutsideTrustedDir(t *testing.T) {
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

	if got, ok := lookTrustedFreeBSDSplitterCommand("pfctl"); ok {
		t.Fatalf("expected symlink target outside trusted dir to be rejected, got %q", got)
	}
}

func TestLookTrustedFreeBSDSplitterCommandRejectsRelativePathName(t *testing.T) {
	parent := t.TempDir()
	sbin := filepath.Join(parent, "sbin")
	bin := filepath.Join(parent, "bin")
	if err := os.MkdirAll(sbin, 0o755); err != nil {
		t.Fatalf("mkdir sbin: %v", err)
	}
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	cmd := filepath.Join(bin, "pfctl")
	if err := os.WriteFile(cmd, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write command: %v", err)
	}

	origDirs := trustedFreeBSDSplitterCommandDirs
	trustedFreeBSDSplitterCommandDirs = []string{sbin, bin}
	defer func() {
		trustedFreeBSDSplitterCommandDirs = origDirs
	}()

	if got, ok := lookTrustedFreeBSDSplitterCommand(filepath.Join("..", "bin", "pfctl")); ok {
		t.Fatalf("expected relative path command name to be rejected, got %q", got)
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
