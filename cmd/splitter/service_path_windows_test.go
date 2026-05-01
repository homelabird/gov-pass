//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

type fakeWindowsFileInfo struct {
	name string
	dir  bool
}

func (f fakeWindowsFileInfo) Name() string       { return f.name }
func (f fakeWindowsFileInfo) Size() int64        { return 0 }
func (f fakeWindowsFileInfo) Mode() os.FileMode  { return 0 }
func (f fakeWindowsFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeWindowsFileInfo) IsDir() bool        { return f.dir }
func (f fakeWindowsFileInfo) Sys() any           { return nil }

func installWindowsPathTestStubs(t *testing.T, existing map[string]bool, reparse map[string]bool) {
	t.Helper()
	origLstat := windowsPathLstat
	origEval := windowsPathEvalSymlinks
	origAttrs := windowsGetFileAttributes
	t.Cleanup(func() {
		windowsPathLstat = origLstat
		windowsPathEvalSymlinks = origEval
		windowsGetFileAttributes = origAttrs
	})

	normalize := func(path string) string {
		return filepath.Clean(path)
	}

	windowsPathLstat = func(path string) (os.FileInfo, error) {
		if existing[normalize(path)] {
			return fakeWindowsFileInfo{name: filepath.Base(path), dir: true}, nil
		}
		return nil, os.ErrNotExist
	}
	windowsPathEvalSymlinks = func(path string) (string, error) {
		if existing[normalize(path)] {
			return normalize(path), nil
		}
		return "", os.ErrNotExist
	}
	windowsGetFileAttributes = func(path *uint16) (uint32, error) {
		name := normalize(windows.UTF16PtrToString(path))
		if reparse[name] {
			return windows.FILE_ATTRIBUTE_REPARSE_POINT, nil
		}
		if existing[name] {
			return windows.FILE_ATTRIBUTE_DIRECTORY, nil
		}
		return 0, windows.ERROR_PATH_NOT_FOUND
	}
}

func TestValidateWindowsServiceConfigPath(t *testing.T) {
	t.Setenv("ProgramData", `C:\ProgramData`)

	if err := validateWindowsServiceConfigPath(`C:\ProgramData\gov-pass\config.json`); err != nil {
		t.Fatalf("expected ProgramData config path to pass: %v", err)
	}
	if err := validateWindowsServiceConfigPath(`config.json`); err == nil {
		t.Fatal("expected relative config path to fail")
	}
	if err := validateWindowsServiceConfigPath(`C:\Temp\config.json`); err == nil {
		t.Fatal("expected config path outside ProgramData to fail")
	}
}

func TestValidateWindowsServiceLogPath(t *testing.T) {
	t.Setenv("ProgramData", `C:\ProgramData`)

	if err := validateWindowsServiceLogPath(`C:\ProgramData\gov-pass\splitter.log`); err != nil {
		t.Fatalf("expected ProgramData log path to pass: %v", err)
	}
	if err := validateWindowsServiceLogPath(`C:\Logs\splitter.log`); err == nil {
		t.Fatal("expected log path outside ProgramData to fail")
	}
}

func TestValidateWindowsServiceDriverDir(t *testing.T) {
	t.Setenv("ProgramData", `C:\ProgramData`)
	t.Setenv("ProgramFiles", `C:\Program Files`)

	if err := validateWindowsServiceDriverDir(`C:\Program Files\gov-pass`); err != nil {
		t.Fatalf("expected install-root driver dir to pass: %v", err)
	}
	if err := validateWindowsServiceDriverDir(`C:\ProgramData\gov-pass\windivert`); err != nil {
		t.Fatalf("expected ProgramData driver dir to pass: %v", err)
	}
	if err := validateWindowsServiceDriverDir(`C:\Temp\windivert`); err == nil {
		t.Fatal("expected driver dir outside managed roots to fail")
	}
}

func TestValidateWindowsServiceConfigPath_AllowsMissingLeafUnderSafeRoot(t *testing.T) {
	t.Setenv("ProgramData", `C:\ProgramData`)
	installWindowsPathTestStubs(t,
		map[string]bool{
			filepath.Clean(`C:\`):                     true,
			filepath.Clean(`C:\ProgramData`):          true,
			filepath.Clean(`C:\ProgramData\gov-pass`): true,
		},
		map[string]bool{},
	)

	if err := validateWindowsServiceConfigPath(`C:\ProgramData\gov-pass\config.json`); err != nil {
		t.Fatalf("expected managed config path to pass: %v", err)
	}
}

func TestValidateWindowsServiceConfigPath_RejectsReparsePoint(t *testing.T) {
	t.Setenv("ProgramData", `C:\ProgramData`)
	installWindowsPathTestStubs(t,
		map[string]bool{
			filepath.Clean(`C:\`):            true,
			filepath.Clean(`C:\ProgramData`): true,
		},
		map[string]bool{
			filepath.Clean(`C:\ProgramData\gov-pass`): true,
		},
	)

	if err := validateWindowsServiceConfigPath(`C:\ProgramData\gov-pass\config.json`); err == nil {
		t.Fatal("expected reparse-point config path to fail")
	}
}

func TestPrepareWindowsServiceLogDirRejectsReparseBeforeMkdir(t *testing.T) {
	installWindowsPathTestStubs(t,
		map[string]bool{
			filepath.Clean(`C:\`): true,
		},
		map[string]bool{
			filepath.Clean(`C:\Logs`): true,
		},
	)

	origMkdirAll := windowsMkdirAll
	mkdirCalled := false
	windowsMkdirAll = func(string, os.FileMode) error {
		mkdirCalled = true
		return nil
	}
	t.Cleanup(func() { windowsMkdirAll = origMkdirAll })

	err := prepareWindowsServiceLogDir(`C:\Logs`, false)
	if err == nil || !strings.Contains(err.Error(), "reparse") {
		t.Fatalf("expected reparse rejection, got %v", err)
	}
	if mkdirCalled {
		t.Fatal("MkdirAll was called after pre-existing log directory reparse point")
	}
}

func TestPrepareWindowsServiceLogDirRejectsReparseAfterMkdir(t *testing.T) {
	existing := map[string]bool{
		filepath.Clean(`C:\`): true,
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

	err := prepareWindowsServiceLogDir(`C:\Logs`, false)
	if err == nil || !strings.Contains(err.Error(), "reparse") {
		t.Fatalf("expected post-create reparse rejection, got %v", err)
	}
	if !mkdirCalled {
		t.Fatal("MkdirAll was not called")
	}
}

func TestPrepareWindowsServiceLogDirUsesSecureHelperForManagedPath(t *testing.T) {
	origEnsure := ensureSecureWindowsServiceLogDir
	origMkdirAll := windowsMkdirAll
	secureCalled := false
	mkdirCalled := false
	ensureSecureWindowsServiceLogDir = func(path string) error {
		secureCalled = path == `C:\ProgramData\gov-pass`
		return nil
	}
	windowsMkdirAll = func(string, os.FileMode) error {
		mkdirCalled = true
		return nil
	}
	t.Cleanup(func() {
		ensureSecureWindowsServiceLogDir = origEnsure
		windowsMkdirAll = origMkdirAll
	})

	if err := prepareWindowsServiceLogDir(`C:\ProgramData\gov-pass`, true); err != nil {
		t.Fatalf("prepare managed log dir: %v", err)
	}
	if !secureCalled {
		t.Fatal("secure log dir helper was not called")
	}
	if mkdirCalled {
		t.Fatal("direct MkdirAll should not be used for managed log directories")
	}
}

func TestEffectiveServiceLogLimits(t *testing.T) {
	maxBytes, maxFiles, err := effectiveServiceLogLimits(serviceLogConfig{})
	if err != nil {
		t.Fatalf("default limits error: %v", err)
	}
	if maxBytes != defaultServiceLogMaxBytes || maxFiles != defaultServiceLogMaxFiles {
		t.Fatalf("default limits = (%d,%d), want (%d,%d)", maxBytes, maxFiles, defaultServiceLogMaxBytes, defaultServiceLogMaxFiles)
	}

	if _, _, err := effectiveServiceLogLimits(serviceLogConfig{MaxBytes: maxServiceLogMaxBytes + 1}); err == nil {
		t.Fatal("expected max bytes validation error")
	}
	if _, _, err := effectiveServiceLogLimits(serviceLogConfig{MaxFiles: maxServiceLogMaxFiles + 1}); err == nil {
		t.Fatal("expected max files validation error")
	}
}
