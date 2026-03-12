package main

import (
	"fmt"
	"strings"
)

type tuiActionID string

const (
	tuiActionToggleService tuiActionID = "toggle-service"
	tuiActionRestart       tuiActionID = "restart"
	tuiActionToggleBoot    tuiActionID = "toggle-boot"
	tuiActionReload        tuiActionID = "reload"
	tuiActionRefresh       tuiActionID = "refresh"
	tuiActionQuit          tuiActionID = "quit"
)

type tuiAction struct {
	ID      tuiActionID
	Key     string
	Aliases []string
	Label   string
	Detail  string
}

func tuiActionsForStatus(status tuiStatus) []tuiAction {
	serviceLabel, serviceDetail := serviceTogglePresentation(status)

	bootLabel := "Toggle boot"
	bootDetail := "Query the current boot setting and enable or disable startup."
	if status.EnabledKnown {
		bootLabel = "Enable boot"
		bootDetail = "Start the service automatically at boot."
		if status.Enabled {
			bootLabel = "Disable boot"
			bootDetail = "Keep the service from starting automatically at boot."
		}
	}

	actions := []tuiAction{
		{
			ID:      tuiActionToggleService,
			Key:     "1",
			Aliases: []string{"1", "switch", "start-stop", "toggle"},
			Label:   serviceLabel,
			Detail:  serviceDetail,
		},
		{
			ID:      tuiActionRestart,
			Key:     "2",
			Aliases: []string{"2", "restart"},
			Label:   "Restart service",
			Detail:  "Restart the runtime without changing boot mode.",
		},
		{
			ID:      tuiActionToggleBoot,
			Key:     "3",
			Aliases: []string{"3", "boot"},
			Label:   bootLabel,
			Detail:  bootDetail,
		},
	}

	if status.Capabilities.Reload {
		actions = append(actions, tuiAction{
			ID:      tuiActionReload,
			Key:     "4",
			Aliases: []string{"4", "reload"},
			Label:   "Reload service",
			Detail:  "Ask the running service to reload its configuration in place.",
		})
	}

	actions = append(actions,
		tuiAction{
			ID:      tuiActionRefresh,
			Key:     "r",
			Aliases: []string{"r", "refresh"},
			Label:   "Refresh",
			Detail:  "Re-read service state from the host.",
		},
		tuiAction{
			ID:      tuiActionQuit,
			Key:     "q",
			Aliases: []string{"q", "quit", "exit"},
			Label:   "Quit",
			Detail:  "Leave the operator panel.",
		},
	)

	return actions
}

func executeMenuChoice(serviceName string, status tuiStatus, choice string) (tuiFeedback, error) {
	action, err := resolveTUIAction(status, choice)
	if err != nil {
		return tuiFeedback{}, err
	}

	switch action.ID {
	case tuiActionToggleService:
		command, err := resolveServiceToggleAction(serviceName, status)
		if err != nil {
			return tuiFeedback{}, err
		}
		if err := runAction(serviceName, command); err != nil {
			return tuiFeedback{}, err
		}
		return actionFeedback(command, serviceName), nil
	case tuiActionRestart:
		if err := runAction(serviceName, "restart"); err != nil {
			return tuiFeedback{}, err
		}
		return actionFeedback("restart", serviceName), nil
	case tuiActionToggleBoot:
		command, err := resolveBootToggleAction(serviceName, status)
		if err != nil {
			return tuiFeedback{}, err
		}
		if err := runAction(serviceName, command); err != nil {
			return tuiFeedback{}, err
		}
		return actionFeedback(command, serviceName), nil
	case tuiActionReload:
		if !status.Capabilities.Reload {
			return tuiFeedback{}, fmt.Errorf("reload is not available on %s", nonEmptyString(status.Platform, "this platform"))
		}
		if err := runAction(serviceName, "reload"); err != nil {
			return tuiFeedback{}, err
		}
		return actionFeedback("reload", serviceName), nil
	case tuiActionRefresh:
		return infoTUIFeedback("Status refreshed.", ""), nil
	default:
		return tuiFeedback{}, fmt.Errorf("unknown selection: %s", strings.TrimSpace(choice))
	}
}

func resolveTUIAction(status tuiStatus, choice string) (tuiAction, error) {
	normalized := strings.ToLower(strings.TrimSpace(choice))
	if normalized == "" {
		return tuiAction{}, fmt.Errorf("unknown selection: %s", choice)
	}

	for _, action := range tuiActionsForStatus(status) {
		for _, alias := range action.Aliases {
			if normalized == alias {
				return action, nil
			}
		}
	}

	if !status.Capabilities.Reload && (normalized == "4" || normalized == "reload") {
		return tuiAction{}, fmt.Errorf("reload is not available on %s", nonEmptyString(status.Platform, "this platform"))
	}

	return tuiAction{}, fmt.Errorf("unknown selection: %s", choice)
}

func resolveServiceToggleAction(serviceName string, status tuiStatus) (string, error) {
	if serviceStateBusy(status.RawState) {
		return "", fmt.Errorf("service is %s; refresh and retry", strings.ToLower(humanizeStateLabel(status.RawState)))
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

func serviceTogglePresentation(status tuiStatus) (string, string) {
	if serviceStateBusy(status.RawState) {
		return "Service busy", "Wait for the current service transition to finish before toggling."
	}
	if action, ok := knownServiceToggleAction(status.RawState); ok {
		if action == "stop" {
			return "Stop service", "Stop the service without changing boot mode."
		}
		return "Start service", "Start the service without changing boot mode."
	}
	if status.ActiveKnown {
		if status.Active {
			return "Stop service", "Stop the service without changing boot mode."
		}
		return "Start service", "Start the service without changing boot mode."
	}
	return "Toggle service", "Query the current state and start or stop the service."
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

func resolveBootToggleAction(serviceName string, status tuiStatus) (string, error) {
	if status.EnabledKnown {
		if status.Enabled {
			return "disable", nil
		}
		return "enable", nil
	}

	enabled, err := isServiceEnabled(serviceName)
	if err != nil {
		return "", err
	}
	if enabled {
		return "disable", nil
	}
	return "enable", nil
}

func actionFeedback(command, serviceName string) tuiFeedback {
	serviceName = strings.TrimSpace(serviceName)

	switch command {
	case "start":
		return successTUIFeedback("Service start requested.", "Target service: "+serviceName, true)
	case "stop":
		return successTUIFeedback("Service stop requested.", "Target service: "+serviceName, true)
	case "restart":
		return successTUIFeedback("Service restart requested.", "Target service: "+serviceName, true)
	case "reload":
		return successTUIFeedback("Service reload requested.", "Target service: "+serviceName, true)
	case "enable":
		return successTUIFeedback("Boot start enabled.", "Target service: "+serviceName, true)
	case "disable":
		return successTUIFeedback("Boot start disabled.", "Target service: "+serviceName, true)
	default:
		return successTUIFeedback("Action requested.", "Target service: "+serviceName, true)
	}
}
