//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

var (
	windowsCommandStat              = os.Stat
	windowsCommandGetFileAttributes = windows.GetFileAttributes
	windowsCommandDirsProvider      = defaultWindowsCommandDirs
)

func defaultWindowsCommandDirs() []string {
	sysDir, err := windows.GetSystemDirectory()
	if err != nil {
		return nil
	}
	sysDir = filepath.Clean(strings.TrimSpace(sysDir))
	if sysDir == "" || sysDir == "." {
		return nil
	}
	dirs := []string{sysDir}
	root := filepath.Dir(sysDir)
	if root != "" && root != "." {
		dirs = append(dirs, filepath.Clean(filepath.Join(root, "Sysnative")))
	}
	return dirs
}

func trustedWindowsCommandDirs() []string {
	return windowsCommandDirsProvider()
}

func resolveTrustedWindowsCommand(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("command name is empty")
	}
	if filepath.IsAbs(name) {
		if isTrustedWindowsCommandPath(name) {
			return filepath.Clean(name), nil
		}
		return "", fmt.Errorf("%s is outside trusted command directories", name)
	}
	if !isBareTrustedCommandName(name) {
		return "", fmt.Errorf("command name must be a bare filename: %s", name)
	}
	for _, dir := range trustedWindowsCommandDirs() {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, name)
		if ok, err := isSafeExecutableWindowsCommandFile(candidate); err != nil {
			return "", err
		} else if ok {
			return candidate, nil
		}
		if filepath.Ext(candidate) == "" {
			exeCandidate := candidate + ".exe"
			if ok, err := isSafeExecutableWindowsCommandFile(exeCandidate); err != nil {
				return "", err
			} else if ok {
				return exeCandidate, nil
			}
		}
	}
	return "", fmt.Errorf("%s not found in trusted command directories", name)
}

func isTrustedWindowsCommandPath(path string) bool {
	clean := filepath.Clean(strings.TrimSpace(path))
	if !filepath.IsAbs(clean) {
		return false
	}
	dir := filepath.Clean(filepath.Dir(clean))
	for _, trustedDir := range trustedWindowsCommandDirs() {
		if trustedDir == "" {
			continue
		}
		if strings.EqualFold(dir, filepath.Clean(trustedDir)) && isExecutableWindowsCommandFile(clean) {
			return true
		}
	}
	return false
}

func isExecutableWindowsCommandFile(path string) bool {
	ok, _ := isSafeExecutableWindowsCommandFile(path)
	return ok
}

func isSafeExecutableWindowsCommandFile(path string) (bool, error) {
	unsafe, err := windowsCommandPathHasReparsePoint(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if unsafe {
		return false, fmt.Errorf("Windows command must not be a reparse point: %s", path)
	}
	info, err := windowsCommandStat(path)
	if err != nil || info.IsDir() {
		return false, nil
	}
	return true, nil
}

func windowsCommandPathHasReparsePoint(path string) (bool, error) {
	ptr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false, err
	}
	attrs, err := windowsCommandGetFileAttributes(ptr)
	if err != nil {
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
			return false, os.ErrNotExist
		}
		return false, err
	}
	return attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0, nil
}
