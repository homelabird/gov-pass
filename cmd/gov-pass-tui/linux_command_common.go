//go:build linux

package main

import (
	"os"
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

func sanitizedLinuxTUICommandEnv() []string {
	term := safeLinuxTUITerm(os.Getenv("TERM"))
	return []string{
		"PATH=" + strings.Join(trustedLinuxTUICommandDirs, string(os.PathListSeparator)),
		"LANG=C",
		"LC_ALL=C",
		"TERM=" + term,
	}
}

func safeLinuxTUITerm(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 64 {
		return "xterm-256color"
	}
	for _, r := range value {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-' || r == '.' || r == '+':
		default:
			return "xterm-256color"
		}
	}
	return value
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
	if !isBareTrustedCommandName(name) {
		return "", false
	}
	for _, dir := range trustedLinuxTUICommandDirs {
		candidate := filepath.Join(dir, name)
		if path, ok := canonicalTrustedCommandPath(candidate, trustedLinuxTUICommandDirs, isExecutableLinuxTUICommandFile); ok {
			return path, true
		}
	}
	return "", false
}

func isExecutableLinuxTUICommandFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode()&0o111 != 0
}
