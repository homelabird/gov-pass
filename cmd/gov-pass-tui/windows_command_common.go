//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var (
	windowsCommandLookPath    = exec.LookPath
	windowsCommandStat        = os.Stat
	windowsSystemRootProvider = defaultWindowsSystemRoot
)

func defaultWindowsSystemRoot() string {
	if root := strings.TrimSpace(os.Getenv("SystemRoot")); root != "" {
		return root
	}
	return strings.TrimSpace(os.Getenv("windir"))
}

func trustedWindowsCommandDirs() []string {
	root := windowsSystemRootProvider()
	if root == "" {
		return nil
	}
	return []string{
		filepath.Clean(filepath.Join(root, "System32")),
		filepath.Clean(filepath.Join(root, "Sysnative")),
	}
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
	for _, dir := range trustedWindowsCommandDirs() {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, name)
		if isExecutableWindowsCommandFile(candidate) {
			return candidate, nil
		}
		if filepath.Ext(candidate) == "" {
			exeCandidate := candidate + ".exe"
			if isExecutableWindowsCommandFile(exeCandidate) {
				return exeCandidate, nil
			}
		}
	}
	path, err := windowsCommandLookPath(name)
	if err == nil && isTrustedWindowsCommandPath(path) {
		return filepath.Clean(path), nil
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
	info, err := windowsCommandStat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return true
}
