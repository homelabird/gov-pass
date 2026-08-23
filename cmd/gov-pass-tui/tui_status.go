package main

import (
	"fmt"
	"strings"
)

type tuiStatus struct {
	Platform       string
	ServiceName    string
	RawState       string
	Active         bool
	ActiveKnown    bool
	WarningSummary string
}

type statusIssue struct {
	Label string
	Err   error
}

type tuiStatusInput struct {
	Platform    string
	ServiceName string
	RawState    string
	Active      bool
	ActiveKnown bool
}

func newTUIStatus(input tuiStatusInput, issues ...statusIssue) tuiStatus {
	return tuiStatus{
		Platform:       strings.TrimSpace(input.Platform),
		ServiceName:    strings.TrimSpace(input.ServiceName),
		RawState:       normalizeStatusState(input.RawState),
		Active:         input.Active,
		ActiveKnown:    input.ActiveKnown,
		WarningSummary: buildStatusWarning(issues...),
	}
}

func buildStatusWarning(issues ...statusIssue) string {
	count := 0
	label := "Status check"
	for _, issue := range issues {
		if issue.Err == nil {
			continue
		}
		count++
		if count == 1 {
			label = normalizeIssueLabel(issue.Label)
		}
	}
	if count == 0 {
		return ""
	}
	if count == 1 {
		return label + " needs attention."
	}
	return fmt.Sprintf("%d checks need attention.", count)
}

func normalizeIssueLabel(label string) string {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "service":
		return "Service check"
	case "":
		return "Status check"
	default:
		return strings.TrimSpace(label) + " check"
	}
}

func normalizeStatusState(state string) string {
	trimmed := strings.ToLower(strings.TrimSpace(state))
	if trimmed == "" {
		return "unknown"
	}
	return trimmed
}
