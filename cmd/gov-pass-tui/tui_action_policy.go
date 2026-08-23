package main

import (
	"fmt"
	"strings"
)

func executeMenuChoice(serviceName string, status tuiStatus, choice string) (tuiFeedback, error) {
	if strings.ToLower(strings.TrimSpace(choice)) != "toggle" {
		return tuiFeedback{}, fmt.Errorf("unknown selection: %s", strings.TrimSpace(choice))
	}

	command, err := resolveServiceToggleAction(serviceName, status)
	if err != nil {
		return tuiFeedback{}, err
	}
	if err := runAction(serviceName, command); err != nil {
		return tuiFeedback{}, err
	}
	if command == "stop" {
		return successTUIFeedback("Service stop requested.", "Target service: "+strings.TrimSpace(serviceName)), nil
	}
	return successTUIFeedback("Service start requested.", "Target service: "+strings.TrimSpace(serviceName)), nil
}

func resolveServiceToggleAction(serviceName string, status tuiStatus) (string, error) {
	if serviceStateBusy(status.RawState) {
		return "", fmt.Errorf("service is %s; refresh and retry", normalizeStatusState(status.RawState))
	}
	if action, ok := knownServiceToggleAction(status.RawState); ok {
		return action, nil
	}
	if status.ActiveKnown {
		if status.Active {
			return "stop", nil
		}
		return "start", nil
	}

	active, err := isServiceActive(serviceName)
	if err != nil {
		return "", err
	}
	if active {
		return "stop", nil
	}
	return "start", nil
}

func knownServiceToggleAction(rawState string) (string, bool) {
	switch normalizeStatusState(rawState) {
	case "active", "running":
		return "stop", true
	case "inactive", "stopped", "failed":
		return "start", true
	default:
		return "", false
	}
}

func serviceStateBusy(rawState string) bool {
	switch normalizeStatusState(rawState) {
	case "activating", "deactivating", "reloading", "refreshing", "start-pending", "stop-pending", "continue-pending", "pause-pending", "paused", "maintenance":
		return true
	default:
		return false
	}
}
