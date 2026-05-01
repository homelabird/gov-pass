package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var trustedFreeBSDSplitterCommandDirs = []string{
	"/usr/local/sbin",
	"/usr/local/bin",
	"/usr/sbin",
	"/usr/bin",
	"/sbin",
	"/bin",
}

func sanitizedFreeBSDSplitterCommandEnv() []string {
	return []string{
		"PATH=" + strings.Join(trustedFreeBSDSplitterCommandDirs, string(os.PathListSeparator)),
		"HOME=/root",
		"LANG=C",
		"LC_ALL=C",
	}
}

func lookTrustedFreeBSDSplitterCommand(name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", false
	}
	if filepath.IsAbs(name) {
		if path, ok := canonicalTrustedFreeBSDSplitterCommandPath(name); ok {
			return path, true
		}
		return "", false
	}
	if !isBareTrustedCommandName(name) {
		return "", false
	}
	for _, dir := range trustedFreeBSDSplitterCommandDirs {
		candidate := filepath.Join(dir, name)
		if path, ok := canonicalTrustedFreeBSDSplitterCommandPath(candidate); ok {
			return path, true
		}
	}
	return "", false
}

func resolveTrustedFreeBSDSplitterCommand(name string) (string, error) {
	if path, ok := lookTrustedFreeBSDSplitterCommand(name); ok {
		return path, nil
	}
	return "", fmt.Errorf("%s not found in trusted command directories", strings.TrimSpace(name))
}

func isTrustedFreeBSDSplitterCommandPath(path string) bool {
	_, ok := canonicalTrustedFreeBSDSplitterCommandPath(path)
	return ok
}

func canonicalTrustedFreeBSDSplitterCommandPath(path string) (string, bool) {
	clean := filepath.Clean(strings.TrimSpace(path))
	if !filepath.IsAbs(clean) {
		return "", false
	}

	resolved, err := filepath.EvalSymlinks(clean)
	if err != nil {
		return "", false
	}
	resolved = filepath.Clean(resolved)
	if !filepath.IsAbs(resolved) || !isExecutableFreeBSDSplitterCommandFile(resolved) {
		return "", false
	}

	for _, dir := range trustedFreeBSDSplitterCommandDirs {
		trustedDir := filepath.Clean(strings.TrimSpace(dir))
		if trustedDir == "" {
			continue
		}
		if canonicalDir, err := filepath.EvalSymlinks(trustedDir); err == nil {
			trustedDir = filepath.Clean(canonicalDir)
		}
		if filepath.Dir(resolved) == trustedDir {
			return resolved, true
		}
	}
	return "", false
}

func isExecutableFreeBSDSplitterCommandFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode()&0o111 != 0
}

func freeBSDPFStatusEnabled(output string) bool {
	for _, line := range strings.Split(output, "\n") {
		line = strings.ToLower(strings.TrimSpace(line))
		if strings.HasPrefix(line, "status:") {
			return strings.Contains(line, "enabled")
		}
	}
	return false
}

func freeBSDPFAnchorRulesLoaded(output string) bool {
	return strings.TrimSpace(output) != ""
}
