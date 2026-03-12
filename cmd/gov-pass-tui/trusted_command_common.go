package main

import (
	"path/filepath"
	"strings"
)

func canonicalTrustedCommandPath(path string, trustedDirs []string, isExecutable func(string) bool) (string, bool) {
	clean := filepath.Clean(strings.TrimSpace(path))
	if !filepath.IsAbs(clean) {
		return "", false
	}

	resolved, err := filepath.EvalSymlinks(clean)
	if err != nil {
		return "", false
	}
	resolved = filepath.Clean(resolved)
	if !filepath.IsAbs(resolved) || !isExecutable(resolved) {
		return "", false
	}

	for _, dir := range trustedDirs {
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
