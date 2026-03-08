package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var trustedFreeBSDCommandDirs = []string{
	"/usr/local/sbin",
	"/usr/local/bin",
	"/usr/sbin",
	"/usr/bin",
	"/sbin",
	"/bin",
}

var freeBSDCommandLookPath = lookTrustedFreeBSDCommand

func freeBSDRcVarName(serviceName string) string {
	serviceName = strings.TrimSpace(serviceName)
	if serviceName == "" {
		return "_enable"
	}

	var b strings.Builder
	b.Grow(len(serviceName) + len("_enable"))
	for _, r := range serviceName {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	b.WriteString("_enable")
	return b.String()
}

func lookTrustedFreeBSDCommand(name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", false
	}
	if filepath.IsAbs(name) {
		if isTrustedFreeBSDCommandPath(name) {
			return filepath.Clean(name), true
		}
		return "", false
	}
	for _, dir := range trustedFreeBSDCommandDirs {
		candidate := filepath.Join(dir, name)
		if isExecutableFreeBSDCommandFile(candidate) {
			return candidate, true
		}
	}
	path, err := exec.LookPath(name)
	if err == nil && isTrustedFreeBSDCommandPath(path) {
		return filepath.Clean(path), true
	}
	return "", false
}

func resolveTrustedFreeBSDCommand(name string) (string, error) {
	if path, ok := freeBSDCommandLookPath(name); ok {
		return path, nil
	}
	return "", fmt.Errorf("%s not found in trusted command directories", strings.TrimSpace(name))
}

func isTrustedFreeBSDCommandPath(path string) bool {
	clean := filepath.Clean(strings.TrimSpace(path))
	if !filepath.IsAbs(clean) {
		return false
	}
	for _, dir := range trustedFreeBSDCommandDirs {
		if filepath.Dir(clean) == filepath.Clean(dir) && isExecutableFreeBSDCommandFile(clean) {
			return true
		}
	}
	return false
}

func isExecutableFreeBSDCommandFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode()&0o111 != 0
}
