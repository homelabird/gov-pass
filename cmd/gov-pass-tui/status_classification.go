package main

import "strings"

func firstStatusLine(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	line := text
	if idx := strings.IndexAny(line, "\r\n"); idx >= 0 {
		line = line[:idx]
	}
	return strings.ToLower(strings.TrimSpace(line))
}

func classifySystemctlActiveStatus(text string) (string, bool, bool) {
	switch firstStatusLine(text) {
	case "active":
		return "active", true, true
	case "inactive":
		return "inactive", false, true
	case "failed":
		return "failed", false, true
	case "activating":
		return "activating", false, true
	case "deactivating":
		return "deactivating", false, true
	case "reloading":
		return "reloading", false, true
	case "maintenance":
		return "maintenance", false, true
	case "refreshing":
		return "refreshing", false, true
	default:
		return "unknown", false, false
	}
}

func classifyFreeBSDServiceStatusOutput(text string, success bool) (string, bool, bool) {
	if success {
		return "active", true, true
	}

	normalized := strings.ToLower(strings.TrimSpace(text))
	switch {
	case strings.Contains(normalized, "is not running"),
		strings.Contains(normalized, "not running as"),
		strings.Contains(normalized, "is stopped"),
		strings.Contains(normalized, "not started"):
		return "inactive", false, true
	case strings.Contains(normalized, "is running"),
		strings.Contains(normalized, "running as pid"),
		strings.Contains(normalized, "running for"):
		return "active", true, true
	default:
		return "unknown", false, false
	}
}
