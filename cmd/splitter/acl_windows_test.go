//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureSecureWindowsDirRejectsReparseBeforeMkdir(t *testing.T) {
	installWindowsPathTestStubs(t,
		map[string]bool{
			filepath.Clean(`C:\`):            true,
			filepath.Clean(`C:\ProgramData`): true,
		},
		map[string]bool{
			filepath.Clean(`C:\ProgramData\gov-pass`): true,
		},
	)

	origMkdirAll := windowsMkdirAll
	mkdirCalled := false
	windowsMkdirAll = func(string, os.FileMode) error {
		mkdirCalled = true
		return nil
	}
	t.Cleanup(func() { windowsMkdirAll = origMkdirAll })

	err := ensureSecureWindowsDir(`C:\ProgramData\gov-pass`)
	if err == nil || !strings.Contains(err.Error(), "reparse") {
		t.Fatalf("expected reparse rejection, got %v", err)
	}
	if mkdirCalled {
		t.Fatal("MkdirAll was called after pre-existing reparse point")
	}
}

func TestEnsureSecureWindowsDirRejectsReparseAfterMkdir(t *testing.T) {
	existing := map[string]bool{
		filepath.Clean(`C:\`):            true,
		filepath.Clean(`C:\ProgramData`): true,
	}
	reparse := map[string]bool{}
	installWindowsPathTestStubs(t, existing, reparse)

	origMkdirAll := windowsMkdirAll
	mkdirCalled := false
	windowsMkdirAll = func(path string, _ os.FileMode) error {
		mkdirCalled = true
		clean := filepath.Clean(path)
		existing[clean] = true
		reparse[clean] = true
		return nil
	}
	t.Cleanup(func() { windowsMkdirAll = origMkdirAll })

	err := ensureSecureWindowsDir(`C:\ProgramData\gov-pass`)
	if err == nil || !strings.Contains(err.Error(), "reparse") {
		t.Fatalf("expected post-create reparse rejection, got %v", err)
	}
	if !mkdirCalled {
		t.Fatal("MkdirAll was not called")
	}
}

func TestResolveSystem32WindowsCommandRejectsNonBareName(t *testing.T) {
	if _, err := resolveSystem32WindowsCommand(`C:\Windows\System32\icacls.exe`); err == nil {
		t.Fatal("expected non-bare System32 command name to fail")
	}
}
