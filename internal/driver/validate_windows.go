//go:build windows

package driver

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var serviceNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,62}$`)

// ValidateServiceName ensures the Windows service name is non-empty and limited
// to safe characters to avoid SCM command injection or unexpected path parsing.
func ValidateServiceName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("service name is empty")
	}
	if !serviceNamePattern.MatchString(name) {
		return fmt.Errorf("service name %q is invalid; use A-Z, a-z, 0-9, '-' or '_'", name)
	}
	return nil
}

// ValidateDriverFileName enforces that an override for the WinDivert driver
// file is a bare filename ending in .sys (no directories or drive prefixes).
func ValidateDriverFileName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	if strings.ContainsAny(name, `\/:`) {
		return fmt.Errorf("driver sys name must not include path separators or drive prefixes")
	}
	base := filepath.Base(name)
	if base != name {
		return fmt.Errorf("driver sys name must be a filename without directories")
	}
	if ext := strings.ToLower(filepath.Ext(name)); ext != ".sys" {
		return fmt.Errorf("driver sys name must end with .sys")
	}
	return nil
}
