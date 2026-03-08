//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var trustedLinuxTUICommandDirs = []string{
	"/usr/local/sbin",
	"/usr/local/bin",
	"/usr/sbin",
	"/usr/bin",
	"/sbin",
	"/bin",
	"/run/current-system/sw/bin",
	"/nix/var/nix/profiles/default/bin",
}

var linuxTUICommandLookPath = lookTrustedLinuxTUICommand

func lookTrustedLinuxTUICommand(name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", false
	}
	if filepath.IsAbs(name) {
		if isTrustedLinuxTUICommandPath(name) {
			return filepath.Clean(name), true
		}
		return "", false
	}
	for _, dir := range trustedLinuxTUICommandDirs {
		candidate := filepath.Join(dir, name)
		if isExecutableLinuxTUICommandFile(candidate) {
			return candidate, true
		}
	}
	path, err := exec.LookPath(name)
	if err == nil && isTrustedLinuxTUICommandPath(path) {
		return filepath.Clean(path), true
	}
	return "", false
}

func resolveTrustedLinuxTUICommand(name string) (string, error) {
	if path, ok := linuxTUICommandLookPath(name); ok {
		return path, nil
	}
	return "", fmt.Errorf("%s not found in trusted command directories", strings.TrimSpace(name))
}

func isTrustedLinuxTUICommandPath(path string) bool {
	clean := filepath.Clean(strings.TrimSpace(path))
	if !filepath.IsAbs(clean) {
		return false
	}
	for _, dir := range trustedLinuxTUICommandDirs {
		if filepath.Dir(clean) == filepath.Clean(dir) && isExecutableLinuxTUICommandFile(clean) {
			return true
		}
	}
	return false
}

func isExecutableLinuxTUICommandFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode()&0o111 != 0
}
