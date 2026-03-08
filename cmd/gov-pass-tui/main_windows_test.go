//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"
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

func TestReloadServiceUsesParamChange(t *testing.T) {
	orig := windowsRunSC
	defer func() {
		windowsRunSC = orig
	}()

	var got []string
	windowsRunSC = func(args ...string) (string, error) {
		got = append([]string(nil), args...)
		return "", nil
	}

	if err := reloadService("gov-pass"); err != nil {
		t.Fatalf("reloadService unexpected error: %v", err)
	}
	want := []string{"control", "gov-pass", "paramchange"}
	if len(got) != len(want) {
		t.Fatalf("reload args length mismatch: got=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("reload arg %d mismatch: got=%v want=%v", i, got, want)
		}
	}
}

func TestReloadServicePropagatesError(t *testing.T) {
	orig := windowsRunSC
	defer func() {
		windowsRunSC = orig
	}()

	windowsRunSC = func(args ...string) (string, error) {
		return "", errors.New("boom")
	}

	if err := reloadService("gov-pass"); err == nil || err.Error() != "boom" {
		t.Fatalf("reloadService error mismatch: %v", err)
	}
}

func TestResolveTrustedWindowsCommand(t *testing.T) {
	root := t.TempDir()
	system32 := filepath.Join(root, "System32")
	if err := os.MkdirAll(system32, 0o755); err != nil {
		t.Fatalf("mkdir system32: %v", err)
	}
	scPath := filepath.Join(system32, "sc.exe")
	if err := os.WriteFile(scPath, []byte("test"), 0o644); err != nil {
		t.Fatalf("write sc.exe: %v", err)
	}

	origRoot := windowsSystemRootProvider
	origLookPath := windowsCommandLookPath
	origStat := windowsCommandStat
	defer func() {
		windowsSystemRootProvider = origRoot
		windowsCommandLookPath = origLookPath
		windowsCommandStat = origStat
	}()

	windowsSystemRootProvider = func() string { return root }
	windowsCommandLookPath = func(name string) (string, error) { return scPath, nil }
	windowsCommandStat = os.Stat

	got, err := resolveTrustedWindowsCommand("sc")
	if err != nil {
		t.Fatalf("resolveTrustedWindowsCommand unexpected error: %v", err)
	}
	if got != scPath {
		t.Fatalf("resolveTrustedWindowsCommand = %q, want %q", got, scPath)
	}
}

func TestResolveTrustedWindowsCommandRejectsPoisonedPath(t *testing.T) {
	root := t.TempDir()
	poisonDir := t.TempDir()
	poisonPath := filepath.Join(poisonDir, "sc.exe")
	if err := os.WriteFile(poisonPath, []byte("test"), 0o644); err != nil {
		t.Fatalf("write poison sc.exe: %v", err)
	}

	origRoot := windowsSystemRootProvider
	origLookPath := windowsCommandLookPath
	origStat := windowsCommandStat
	defer func() {
		windowsSystemRootProvider = origRoot
		windowsCommandLookPath = origLookPath
		windowsCommandStat = origStat
	}()

	windowsSystemRootProvider = func() string { return root }
	windowsCommandLookPath = func(name string) (string, error) { return poisonPath, nil }
	windowsCommandStat = os.Stat

	if got, err := resolveTrustedWindowsCommand("sc"); err == nil {
		t.Fatalf("expected poisoned path to be rejected, got %q", got)
	}
}
