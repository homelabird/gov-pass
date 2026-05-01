//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

type fakeMSIHelperFileInfo struct {
	name string
	dir  bool
}

func (f fakeMSIHelperFileInfo) Name() string       { return f.name }
func (f fakeMSIHelperFileInfo) Size() int64        { return 0 }
func (f fakeMSIHelperFileInfo) Mode() os.FileMode  { return 0 }
func (f fakeMSIHelperFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeMSIHelperFileInfo) IsDir() bool        { return f.dir }
func (f fakeMSIHelperFileInfo) Sys() any           { return nil }

func TestDefaultMSIHelperProgramDataDirPrefersKnownFolder(t *testing.T) {
	orig := msiHelperKnownFolderPath
	t.Cleanup(func() { msiHelperKnownFolderPath = orig })

	msiHelperKnownFolderPath = func(folderID *windows.KNOWNFOLDERID, flags uint32) (string, error) {
		if folderID != windows.FOLDERID_ProgramData {
			return "", errors.New("unexpected folder")
		}
		return `D:\KnownProgramData`, nil
	}
	t.Setenv("ProgramData", `C:\PoisonedProgramData`)

	if got := defaultMSIHelperProgramDataDir(); got != filepath.Clean(`D:\KnownProgramData`) {
		t.Fatalf("defaultMSIHelperProgramDataDir = %q", got)
	}
}

func TestDefaultMSIHelperProgramDataDirFallsBackToSystemDefault(t *testing.T) {
	orig := msiHelperKnownFolderPath
	t.Cleanup(func() { msiHelperKnownFolderPath = orig })

	msiHelperKnownFolderPath = func(folderID *windows.KNOWNFOLDERID, flags uint32) (string, error) {
		return "", errors.New("known folder unavailable")
	}
	t.Setenv("ProgramData", `C:\PoisonedProgramData`)

	if got := defaultMSIHelperProgramDataDir(); got != `C:\ProgramData` {
		t.Fatalf("defaultMSIHelperProgramDataDir = %q", got)
	}
}

func TestResolveMSIHelperSystem32CommandRejectsReparsePoint(t *testing.T) {
	origSystemDirectory := msiHelperSystemDirectory
	origStat := msiHelperStat
	origAttrs := helperGetFileAttributes
	t.Cleanup(func() {
		msiHelperSystemDirectory = origSystemDirectory
		msiHelperStat = origStat
		helperGetFileAttributes = origAttrs
	})

	sysDir := filepath.Clean(`C:\Windows\System32`)
	commandPath := filepath.Join(sysDir, "taskkill.exe")
	msiHelperSystemDirectory = func() (string, error) { return sysDir, nil }
	msiHelperStat = func(path string) (os.FileInfo, error) {
		return fakeMSIHelperFileInfo{name: filepath.Base(path)}, nil
	}
	helperGetFileAttributes = func(path *uint16) (uint32, error) {
		if filepath.Clean(windows.UTF16PtrToString(path)) == commandPath {
			return windows.FILE_ATTRIBUTE_REPARSE_POINT, nil
		}
		return 0, nil
	}

	_, err := resolveMSIHelperSystem32Command("taskkill.exe")
	if err == nil || !strings.Contains(err.Error(), "reparse point") {
		t.Fatalf("expected reparse point rejection, got %v", err)
	}
}

func TestResolveMSIHelperSystem32CommandRejectsQualifiedName(t *testing.T) {
	if _, err := resolveMSIHelperSystem32Command(`C:\Windows\System32\taskkill.exe`); err == nil {
		t.Fatal("expected qualified System32 command name to be rejected")
	}
}
