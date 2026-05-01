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

func TestLookTrustedFreeBSDCommand_RejectsPoisonedPATHEntry(t *testing.T) {
	dir := t.TempDir()
	cmd := filepath.Join(dir, "service")
	if err := os.WriteFile(cmd, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake command: %v", err)
	}
	t.Setenv("PATH", dir)

	origDirs := trustedFreeBSDCommandDirs
	trustedFreeBSDCommandDirs = []string{filepath.Join(dir, "missing")}
	defer func() {
		trustedFreeBSDCommandDirs = origDirs
	}()

	if got, ok := lookTrustedFreeBSDCommand("service"); ok {
		t.Fatalf("expected poisoned PATH entry to be rejected, got %q", got)
	}
}

func TestLookTrustedFreeBSDCommand_UsesTrustedAbsoluteDir(t *testing.T) {
	dir := t.TempDir()
	cmd := filepath.Join(dir, "service")
	if err := os.WriteFile(cmd, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake command: %v", err)
	}

	origDirs := trustedFreeBSDCommandDirs
	trustedFreeBSDCommandDirs = []string{dir}
	defer func() {
		trustedFreeBSDCommandDirs = origDirs
	}()

	got, ok := lookTrustedFreeBSDCommand("service")
	if !ok {
		t.Fatal("expected trusted command lookup to succeed")
	}
	if filepath.Clean(got) != filepath.Clean(cmd) {
		t.Fatalf("lookTrustedFreeBSDCommand returned %q, want %q", got, cmd)
	}
}

func TestResolveTrustedFreeBSDCommandAndPathCheck(t *testing.T) {
	dir := t.TempDir()
	cmd := filepath.Join(dir, "service")
	if err := os.WriteFile(cmd, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake command: %v", err)
	}

	origDirs := trustedFreeBSDCommandDirs
	trustedFreeBSDCommandDirs = []string{dir}
	defer func() {
		trustedFreeBSDCommandDirs = origDirs
	}()

	got, err := resolveTrustedFreeBSDCommand("service")
	if err != nil {
		t.Fatalf("resolveTrustedFreeBSDCommand: %v", err)
	}
	if filepath.Clean(got) != filepath.Clean(cmd) {
		t.Fatalf("resolveTrustedFreeBSDCommand returned %q, want %q", got, cmd)
	}
	if !isTrustedFreeBSDCommandPath(cmd) {
		t.Fatalf("isTrustedFreeBSDCommandPath(%q) = false, want true", cmd)
	}
}

func TestLookTrustedFreeBSDCommand_RejectsSymlinkOutsideTrustedDir(t *testing.T) {
	trustedDir := t.TempDir()
	outsideDir := t.TempDir()
	target := filepath.Join(outsideDir, "service")
	if err := os.WriteFile(target, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write target command: %v", err)
	}
	link := filepath.Join(trustedDir, "service")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	origDirs := trustedFreeBSDCommandDirs
	trustedFreeBSDCommandDirs = []string{trustedDir}
	defer func() {
		trustedFreeBSDCommandDirs = origDirs
	}()

	if got, ok := lookTrustedFreeBSDCommand("service"); ok {
		t.Fatalf("expected symlink target outside trusted dir to be rejected, got %q", got)
	}
}

func TestLookTrustedFreeBSDCommand_RejectsRelativePathName(t *testing.T) {
	parent := t.TempDir()
	sbin := filepath.Join(parent, "sbin")
	bin := filepath.Join(parent, "bin")
	if err := os.MkdirAll(sbin, 0o755); err != nil {
		t.Fatalf("mkdir sbin: %v", err)
	}
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	cmd := filepath.Join(bin, "service")
	if err := os.WriteFile(cmd, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write command: %v", err)
	}

	origDirs := trustedFreeBSDCommandDirs
	trustedFreeBSDCommandDirs = []string{sbin, bin}
	defer func() {
		trustedFreeBSDCommandDirs = origDirs
	}()

	if got, ok := lookTrustedFreeBSDCommand(filepath.Join("..", "bin", "service")); ok {
		t.Fatalf("expected relative path command name to be rejected, got %q", got)
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
