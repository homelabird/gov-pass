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
		if path, ok := canonicalTrustedCommandPath(name, trustedLinuxTUICommandDirs, isExecutableLinuxTUICommandFile); ok {
			return path, true
		}
		return "", false
	}
	for _, dir := range trustedLinuxTUICommandDirs {
		candidate := filepath.Join(dir, name)
		if path, ok := canonicalTrustedCommandPath(candidate, trustedLinuxTUICommandDirs, isExecutableLinuxTUICommandFile); ok {
			return path, true
		}
	}
	path, err := exec.LookPath(name)
	if err == nil {
		if trustedPath, ok := canonicalTrustedCommandPath(path, trustedLinuxTUICommandDirs, isExecutableLinuxTUICommandFile); ok {
			return trustedPath, true
		}
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
	_, ok := canonicalTrustedCommandPath(path, trustedLinuxTUICommandDirs, isExecutableLinuxTUICommandFile)
	return ok
}

func isExecutableLinuxTUICommandFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode()&0o111 != 0
}
