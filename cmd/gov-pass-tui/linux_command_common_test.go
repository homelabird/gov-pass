//go:build linux

package main

import (
	"os"
	"path/filepath"
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
