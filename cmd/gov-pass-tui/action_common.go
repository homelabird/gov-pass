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
	if !isASCIIAlphaNum(normalized[0]) {
		return "", errors.New("service name must start with a letter or digit")
	}
	if !isASCIIAlphaNum(normalized[len(normalized)-1]) {
		return "", errors.New("service name must end with a letter or digit")
	}
	for i := 0; i < len(normalized); i++ {
		c := normalized[i]
		switch {
		case isASCIIAlphaNum(c):
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
	if len(normalized) > 63 {
		return "", errors.New("service name is too long")
	}
	if !isASCIIAlphaNum(normalized[0]) {
		return "", errors.New("service name must start with a letter or digit")
	}
	for i := 0; i < len(normalized); i++ {
		c := normalized[i]
		switch {
		case isASCIIAlphaNum(c):
		case c == '-' || c == '_' || c == ' ':
		default:
			return "", fmt.Errorf("service name contains unsupported character %q", c)
		}
	}
	return normalized, nil
}

func isASCIIAlphaNum(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}
