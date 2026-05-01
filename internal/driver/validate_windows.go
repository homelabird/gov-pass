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
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	if stem == "" || stem == "." || stem == ".." {
		return fmt.Errorf("driver sys name must include a filename before .sys")
	}
	if strings.TrimRight(stem, " .") != stem {
		return fmt.Errorf("driver sys name must not end with space or dot before .sys")
	}
	if isReservedWindowsDeviceFileName(stem) {
		return fmt.Errorf("driver sys name must not use reserved Windows device name %q", stem)
	}
	return nil
}

func isReservedWindowsDeviceFileName(stem string) bool {
	stem = strings.ToUpper(strings.TrimRight(strings.TrimSpace(stem), " ."))
	switch stem {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$":
		return true
	}
	if len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) {
		return stem[3] >= '1' && stem[3] <= '9'
	}
	return false
}
