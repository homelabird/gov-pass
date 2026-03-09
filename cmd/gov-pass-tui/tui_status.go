package main

import (
	"fmt"
	"strings"
	"time"
)

type tuiHealth string

const (
	tuiHealthHealthy  tuiHealth = "healthy"
	tuiHealthStopped  tuiHealth = "stopped"
	tuiHealthBusy     tuiHealth = "busy"
	tuiHealthDegraded tuiHealth = "degraded"
	tuiHealthUnknown  tuiHealth = "unknown"
)

type tuiCapabilities struct {
	Reload bool
}

type tuiStatus struct {
	Platform       string
	ServiceName    string
	Health         tuiHealth
	Summary        string
	RawState       string
	StateLabel     string
	Active         bool
	ActiveKnown    bool
	Enabled        bool
	EnabledKnown   bool
	BootLabel      string
	WarningSummary string
	WarningDetails []string
	Capabilities   tuiCapabilities
	UpdatedAt      time.Time
}

type statusIssue struct {
	Label string
	Err   error
}

type tuiStatusInput struct {
	Platform     string
	ServiceName  string
	RawState     string
	Active       bool
	ActiveKnown  bool
	Enabled      bool
	EnabledKnown bool
	Capabilities tuiCapabilities
	UpdatedAt    time.Time
}

func newTUIStatus(input tuiStatusInput, issues ...statusIssue) tuiStatus {
	rawState := normalizeStatusState(input.RawState)
	warningSummary, warningDetails := buildStatusWarning(issues...)

	if input.UpdatedAt.IsZero() {
		input.UpdatedAt = time.Now()
	}

	status := tuiStatus{
		Platform:       strings.TrimSpace(input.Platform),
		ServiceName:    strings.TrimSpace(input.ServiceName),
		RawState:       rawState,
		StateLabel:     humanizeStateLabel(rawState),
		Active:         input.Active,
		ActiveKnown:    input.ActiveKnown,
		Enabled:        input.Enabled,
		EnabledKnown:   input.EnabledKnown,
		BootLabel:      bootSettingLabel(input.Enabled, input.EnabledKnown),
		WarningSummary: warningSummary,
		WarningDetails: warningDetails,
		Capabilities:   input.Capabilities,
		UpdatedAt:      input.UpdatedAt,
	}

	status.Health = deriveStatusHealth(status)
	status.Summary = deriveStatusSummary(status)
	return status
}

func buildStatusWarning(issues ...statusIssue) (string, []string) {
	details := make([]string, 0, len(issues))
	for _, issue := range issues {
		if issue.Err == nil {
			continue
		}

		label := normalizeIssueLabel(issue.Label)
		text := strings.TrimSpace(issue.Err.Error())
		if text == "" {
			text = "unknown error"
		}
		details = append(details, fmt.Sprintf("%s: %s", label, text))
	}

	switch len(details) {
	case 0:
		return "", nil
	case 1:
		label := details[0]
		if idx := strings.Index(label, ":"); idx > 0 {
			label = label[:idx]
		}
		return label + " needs attention.", details
	default:
		return fmt.Sprintf("%d checks need attention.", len(details)), details
	}
}

func normalizeIssueLabel(label string) string {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "service":
		return "Service check"
	case "boot":
		return "Boot check"
	case "":
		return "Status check"
	default:
		return humanizeStateLabel(label) + " check"
	}
}

func normalizeStatusState(state string) string {
	trimmed := strings.ToLower(strings.TrimSpace(state))
	if trimmed == "" {
		return "unknown"
	}
	return trimmed
}

func humanizeStateLabel(state string) string {
	switch normalizeStatusState(state) {
	case "active", "running":
		return "Running"
	case "inactive", "stopped":
		return "Stopped"
	case "failed":
		return "Failed"
	case "activating", "start-pending":
		return "Starting"
	case "deactivating", "stop-pending":
		return "Stopping"
	case "reloading":
		return "Reloading"
	case "refreshing":
		return "Refreshing"
	case "continue-pending":
		return "Resuming"
	case "pause-pending":
		return "Pausing"
	case "paused":
		return "Paused"
	case "maintenance":
		return "Maintenance"
	case "unknown":
		return "Unknown"
	default:
		return titleCaseWords(strings.NewReplacer("-", " ", "_", " ").Replace(state))
	}
}

func titleCaseWords(text string) string {
	parts := strings.Fields(strings.TrimSpace(text))
	for i, part := range parts {
		runes := []rune(strings.ToLower(part))
		if len(runes) == 0 {
			continue
		}
		runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
		parts[i] = string(runes)
	}
	return strings.Join(parts, " ")
}

func bootSettingLabel(enabled, known bool) string {
	if !known {
		return "Unknown"
	}
	if enabled {
		return "Enabled"
	}
	return "Disabled"
}

func deriveStatusHealth(status tuiStatus) tuiHealth {
	hasWarnings := strings.TrimSpace(status.WarningSummary) != ""

	switch status.RawState {
	case "active", "running":
		if hasWarnings {
			return tuiHealthDegraded
		}
		return tuiHealthHealthy
	case "inactive", "stopped":
		if hasWarnings {
			return tuiHealthDegraded
		}
		return tuiHealthStopped
	case "activating", "deactivating", "reloading", "refreshing", "start-pending", "stop-pending", "continue-pending", "pause-pending":
		return tuiHealthBusy
	case "failed", "paused", "maintenance":
		return tuiHealthDegraded
	default:
		if hasWarnings {
			return tuiHealthDegraded
		}
		return tuiHealthUnknown
	}
}

func deriveStatusSummary(status tuiStatus) string {
	var summary string

	switch status.Health {
	case tuiHealthHealthy:
		summary = "Service is running normally."
	case tuiHealthStopped:
		summary = "Service is stopped."
	case tuiHealthBusy:
		summary = fmt.Sprintf("Service is %s.", strings.ToLower(status.StateLabel))
	case tuiHealthDegraded:
		if status.StateLabel == "Failed" {
			summary = "Service failed and needs attention."
		} else if status.StateLabel == "Unknown" {
			summary = "Service state could not be verified."
		} else {
			summary = fmt.Sprintf("Service is %s and needs attention.", strings.ToLower(status.StateLabel))
		}
	default:
		summary = "Service state is unknown."
	}

	if status.EnabledKnown {
		summary += " Boot is " + strings.ToLower(status.BootLabel) + "."
	}

	return summary
}

func healthLabel(health tuiHealth) string {
	switch health {
	case tuiHealthHealthy:
		return "HEALTHY"
	case tuiHealthStopped:
		return "STOPPED"
	case tuiHealthBusy:
		return "BUSY"
	case tuiHealthDegraded:
		return "DEGRADED"
	default:
		return "UNKNOWN"
	}
}
