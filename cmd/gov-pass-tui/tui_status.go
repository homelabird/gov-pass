package main

import "strings"

type statusIssue struct {
	Label string
	Err   error
}

func newTUIStatus(platform, serviceName, state string, active, enabled bool, issues ...statusIssue) tuiStatus {
	state = strings.TrimSpace(state)
	if state == "" {
		state = "unknown"
	}
	return tuiStatus{
		Platform:    strings.TrimSpace(platform),
		ServiceName: strings.TrimSpace(serviceName),
		State:       state,
		Active:      active,
		Enabled:     enabled,
		Warning:     buildStatusWarning(issues...),
	}
}

func buildStatusWarning(issues ...statusIssue) string {
	lines := make([]string, 0, len(issues))
	for _, issue := range issues {
		if issue.Err == nil {
			continue
		}
		label := strings.TrimSpace(issue.Label)
		if label == "" {
			label = "status"
		}
		text := strings.TrimSpace(issue.Err.Error())
		if text == "" {
			text = "unknown error"
		}
		lines = append(lines, label+": "+text)
	}
	return strings.Join(lines, "\n")
}
