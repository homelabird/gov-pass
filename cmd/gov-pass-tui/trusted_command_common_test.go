package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCanonicalTrustedCommandPath_AllowsSymlinkedTrustedDir(t *testing.T) {
	actualDir := t.TempDir()
	trustedParent := t.TempDir()
	trustedDir := filepath.Join(trustedParent, "trusted-bin")

	target := filepath.Join(actualDir, "systemctl")
	if err := os.WriteFile(target, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write target command: %v", err)
	}
	if err := os.Symlink(actualDir, trustedDir); err != nil {
		t.Fatalf("create trusted dir symlink: %v", err)
	}

	got, ok := canonicalTrustedCommandPath(filepath.Join(trustedDir, "systemctl"), []string{trustedDir}, executableTestFile)
	if !ok {
		t.Fatal("expected command in symlinked trusted dir to be accepted")
	}
	if got != target {
		t.Fatalf("canonicalTrustedCommandPath returned %q, want %q", got, target)
	}
}

func TestCanonicalTrustedCommandPath_RejectsSymlinkEscapingTrustedDir(t *testing.T) {
	trustedDir := t.TempDir()
	otherDir := t.TempDir()

	target := filepath.Join(otherDir, "service")
	if err := os.WriteFile(target, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write target command: %v", err)
	}

	link := filepath.Join(trustedDir, "service")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("create escaping symlink: %v", err)
	}

	if got, ok := canonicalTrustedCommandPath(link, []string{trustedDir}, executableTestFile); ok {
		t.Fatalf("expected escaping symlink to be rejected, got %q", got)
	}
}

func executableTestFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode()&0o111 != 0
}
