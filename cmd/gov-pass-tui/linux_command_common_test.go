//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLookTrustedLinuxTUICommand_RejectsSymlinkOutsideTrustedDir(t *testing.T) {
	trustedDir := t.TempDir()
	outsideDir := t.TempDir()
	target := filepath.Join(outsideDir, "systemctl")
	if err := os.WriteFile(target, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write target command: %v", err)
	}
	link := filepath.Join(trustedDir, "systemctl")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	origDirs := trustedLinuxTUICommandDirs
	trustedLinuxTUICommandDirs = []string{trustedDir}
	defer func() {
		trustedLinuxTUICommandDirs = origDirs
	}()

	if got, ok := lookTrustedLinuxTUICommand("systemctl"); ok {
		t.Fatalf("expected symlink target outside trusted dir to be rejected, got %q", got)
	}
}

func TestLookTrustedLinuxTUICommand_RejectsRelativePathName(t *testing.T) {
	parent := t.TempDir()
	sbin := filepath.Join(parent, "sbin")
	bin := filepath.Join(parent, "bin")
	if err := os.MkdirAll(sbin, 0o755); err != nil {
		t.Fatalf("mkdir sbin: %v", err)
	}
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	cmd := filepath.Join(bin, "systemctl")
	if err := os.WriteFile(cmd, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write command: %v", err)
	}

	origDirs := trustedLinuxTUICommandDirs
	trustedLinuxTUICommandDirs = []string{sbin, bin}
	defer func() {
		trustedLinuxTUICommandDirs = origDirs
	}()

	if got, ok := lookTrustedLinuxTUICommand(filepath.Join("..", "bin", "systemctl")); ok {
		t.Fatalf("expected relative path command name to be rejected, got %q", got)
	}
}

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
