package main

import (
	"errors"
	"fmt"
	"strings"
)

var allowedTUIActions = map[string]struct{}{
	"start":   {},
	"stop":    {},
	"restart": {},
	"reload":  {},
	"enable":  {},
	"disable": {},
	"toggle":  {},
	"status":  {},
}

func normalizeAction(action string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(action))
	if _, ok := allowedTUIActions[normalized]; !ok {
		return "", fmt.Errorf("unknown action: %s", normalized)
	}
	return normalized, nil
}

func normalizeUnixServiceName(name string) (string, error) {
	normalized := strings.TrimSpace(name)
	if normalized == "" {
		return "", errors.New("service name is empty")
	}
	if len(normalized) > 128 {
		return "", errors.New("service name is too long")
	}
	if strings.HasPrefix(normalized, "-") {
		return "", errors.New("service name cannot start with '-'")
	}
	for i := 0; i < len(normalized); i++ {
		c := normalized[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '-', c == '_', c == '.', c == '@':
		default:
			return "", fmt.Errorf("service name contains unsupported character %q", c)
		}
	}
	return normalized, nil
}

func normalizeWindowsServiceName(name string) (string, error) {
	normalized := strings.TrimSpace(name)
	if normalized == "" {
		return "", errors.New("service name is empty")
	}
	if len(normalized) > 256 {
		return "", errors.New("service name is too long")
	}
	for _, r := range normalized {
		if r == 0 || r == '\r' || r == '\n' {
			return "", fmt.Errorf("service name contains unsupported character %q", r)
		}
	}
	return normalized, nil
}
