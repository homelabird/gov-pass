//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

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

func TestRunActionToggleCallsExpectedSC(t *testing.T) {
	for _, test := range []struct {
		state string
		want  string
	}{
		{state: "stopped", want: "start"},
		{state: "running", want: "stop"},
	} {
		t.Run(test.state, func(t *testing.T) {
			orig := windowsRunSC
			t.Cleanup(func() { windowsRunSC = orig })

			state := test.state
			var calls [][]string
			windowsRunSC = func(args ...string) (string, error) {
				calls = append(calls, append([]string(nil), args...))
				switch args[0] {
				case "query":
					return fmt.Sprintf("STATE : 4 %s", strings.ToUpper(state)), nil
				case "start":
					state = "running"
				case "stop":
					state = "stopped"
				}
				return "", nil
			}

			if err := runAction("gov-pass", "toggle"); err != nil {
				t.Fatalf("toggle unexpected error: %v", err)
			}
			if len(calls) != 3 || calls[0][0] != "query" || calls[1][0] != test.want || calls[2][0] != "query" {
				t.Fatalf("sc calls = %v, want query, %s, query", calls, test.want)
			}
		})
	}
}

func TestResolveTrustedWindowsCommandRejectsPoisonedPath(t *testing.T) {
	root := t.TempDir()
	poisonDir := t.TempDir()
	poisonPath := filepath.Join(poisonDir, "sc.exe")
	if err := os.WriteFile(poisonPath, []byte("test"), 0o644); err != nil {
		t.Fatalf("write poison sc.exe: %v", err)
	}

	origDirs := windowsCommandDirsProvider
	origStat := windowsCommandStat
	defer func() {
		windowsCommandDirsProvider = origDirs
		windowsCommandStat = origStat
	}()

	windowsCommandDirsProvider = func() []string { return []string{filepath.Join(root, "System32")} }
	windowsCommandStat = os.Stat

	if got, err := resolveTrustedWindowsCommand("sc"); err == nil {
		t.Fatalf("expected poisoned path to be rejected, got %q", got)
	}
}

func TestResolveTrustedWindowsCommandRejectsReparsePoint(t *testing.T) {
	root := t.TempDir()
	system32 := filepath.Join(root, "System32")
	if err := os.MkdirAll(system32, 0o755); err != nil {
		t.Fatalf("mkdir system32: %v", err)
	}
	scPath := filepath.Join(system32, "sc.exe")
	if err := os.WriteFile(scPath, []byte("test"), 0o644); err != nil {
		t.Fatalf("write sc.exe: %v", err)
	}

	origDirs := windowsCommandDirsProvider
	origStat := windowsCommandStat
	origAttrs := windowsCommandGetFileAttributes
	defer func() {
		windowsCommandDirsProvider = origDirs
		windowsCommandStat = origStat
		windowsCommandGetFileAttributes = origAttrs
	}()

	windowsCommandDirsProvider = func() []string { return []string{system32} }
	windowsCommandStat = os.Stat
	windowsCommandGetFileAttributes = func(path *uint16) (uint32, error) {
		if filepath.Clean(windows.UTF16PtrToString(path)) == scPath {
			return windows.FILE_ATTRIBUTE_REPARSE_POINT, nil
		}
		return 0, windows.ERROR_FILE_NOT_FOUND
	}

	_, err := resolveTrustedWindowsCommand("sc")
	if err == nil || !strings.Contains(err.Error(), "reparse point") {
		t.Fatalf("expected reparse point rejection, got %v", err)
	}
}
