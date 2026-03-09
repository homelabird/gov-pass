//go:build linux

package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

const defaultServiceName = "gov-pass"

var (
	linuxLookPath     = linuxTUICommandLookPath
	linuxSystemctlCmd = func(args ...string) (*exec.Cmd, error) {
		return systemctlCommand(args...)
	}
	linuxSudoSystemctlCmd = func(action, serviceName string) (*exec.Cmd, error) {
		return sudoSystemctlCommand(action, serviceName)
	}
	linuxPkexecSystemctlCmd = func(action, serviceName string) (*exec.Cmd, error) {
		return pkexecSystemctlCommand(action, serviceName)
	}
)

func main() {
	serviceName := flag.String("service-name", defaultServiceName, "systemd service name to control")
	action := flag.String("action", "", "action mode: start|stop|restart|reload|enable|disable|toggle|status (runs and exits)")
	flag.Parse()

	name := strings.TrimSpace(*serviceName)
	if name == "" {
		name = defaultServiceName
	}
	name, err := normalizeUnixServiceName(name)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	act := strings.ToLower(strings.TrimSpace(*action))
	if act != "" {
		if err := runAction(name, act); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		return
	}

	if err := runTUI(name); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func runTUI(serviceName string) error {
	return runBubbleTUI(serviceName, collectTUIStatus)
}

func runWhiptailTUI(serviceName string) error {
	lastFeedback := readyTUIFeedback()

	for {
		status := collectTUIStatus(serviceName)
		choice, canceled, err := whiptailMenu(status, lastFeedback)
		if err != nil {
			return err
		}
		if canceled || choice == "" || choice == "q" {
			return nil
		}

		feedback, err := executeMenuChoice(serviceName, status, choice)
		if err != nil {
			lastFeedback = errorTUIFeedback(err)
			_ = whiptailMessage(lastFeedback.dialogTitle(), lastFeedback.text())
			continue
		}
		if !feedback.isZero() {
			lastFeedback = feedback
		}
		if feedback.Popup {
			_ = whiptailMessage(feedback.dialogTitle(), feedback.text())
		}
	}
}

func whiptailMenu(status tuiStatus, feedback tuiFeedback) (choice string, canceled bool, err error) {
	height, width, menuHeight := whiptailMenuDimensions(status, feedback)
	actions := tuiActionsForStatus(status)
	args := []string{
		"--title", "gov-pass operator panel",
		"--menu", buildCompactStatusSummary(status, feedback),
		strconv.Itoa(height), strconv.Itoa(width), strconv.Itoa(menuHeight),
		"--output-fd", "1",
	}
	for _, action := range actions {
		args = append(args, action.Key, fmt.Sprintf("%s: %s", action.Label, action.Detail))
	}
	cmd, err := whiptailCommand(args...)
	if err != nil {
		return "", false, err
	}
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	out, cmdErr := cmd.Output()
	if cmdErr != nil {
		var exitErr *exec.ExitError
		if errors.As(cmdErr, &exitErr) {
			// ESC/cancel returns non-zero in whiptail.
			if exitErr.ExitCode() == 1 || exitErr.ExitCode() == 255 {
				return "", true, nil
			}
		}
		return "", false, fmt.Errorf("whiptail failed: %v", cmdErr)
	}
	return strings.TrimSpace(string(out)), false, nil
}

func whiptailMessage(title, text string) error {
	height, width := whiptailMessageDimensions(text)
	cmd, err := whiptailCommand("--title", title, "--msgbox", text, strconv.Itoa(height), strconv.Itoa(width))
	if err != nil {
		return err
	}
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runPlainTUI(serviceName string) error {
	scanner := bufio.NewScanner(os.Stdin)
	lastFeedback := readyTUIFeedback()

	for {
		status := collectTUIStatus(serviceName)
		renderPlainTUI(status, lastFeedback)
		fmt.Print("Select> ")

		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return err
			}
			return nil
		}
		choice := strings.ToLower(strings.TrimSpace(scanner.Text()))
		if choice == "" {
			lastFeedback = infoTUIFeedback("No selection.", "Type one of the keys shown in brackets.")
			continue
		}
		if choice == "q" || choice == "quit" || choice == "exit" {
			return nil
		}

		feedback, err := executeMenuChoice(serviceName, status, choice)
		if err != nil {
			lastFeedback = errorTUIFeedback(err)
			continue
		}
		if !feedback.isZero() {
			lastFeedback = feedback
		}
	}
}

func renderPlainTUI(status tuiStatus, feedback tuiFeedback) {
	clearTerminalScreen()
	fmt.Print(renderPlainTUIView(status, feedback))
	fmt.Println()
}

func whiptailMenuDimensions(status tuiStatus, feedback tuiFeedback) (height, width, menuHeight int) {
	size := currentTerminalSize()
	maxWidth := maxInt(16, size.Cols-2)
	width = clampInt(size.Cols-4, minInt(60, maxWidth), maxWidth)
	summaryLines := countWrappedTextLines(buildCompactStatusSummary(status, feedback), width-8)
	actions := tuiActionsForStatus(status)
	maxHeight := maxInt(8, size.Rows-2)
	height = clampInt(summaryLines+len(actions)+9, minInt(16, maxHeight), maxHeight)
	menuHeight = clampInt(len(actions), minInt(5, maxInt(1, height-summaryLines-7)), maxInt(1, height-summaryLines-7))
	return height, width, menuHeight
}

func whiptailMessageDimensions(text string) (height, width int) {
	size := currentTerminalSize()
	maxWidth := maxInt(16, size.Cols-2)
	width = clampInt(size.Cols-6, minInt(56, maxWidth), maxWidth)
	maxHeight := maxInt(8, size.Rows-2)
	height = clampInt(countWrappedTextLines(text, width-8)+6, minInt(10, maxHeight), maxHeight)
	return height, width
}

func clearTerminalScreen() {
	// ANSI clear screen + move cursor home.
	fmt.Print("\033[H\033[2J")
}

func collectTUIStatus(serviceName string) tuiStatus {
	stateText, stateErr := serviceStatusText(serviceName)
	enabled, enabledErr := isServiceEnabled(serviceName)
	return newTUIStatus(tuiStatusInput{
		Platform:     "Linux",
		ServiceName:  serviceName,
		RawState:     stateText,
		Active:       strings.EqualFold(stateText, "active"),
		ActiveKnown:  stateErr == nil && !strings.EqualFold(strings.TrimSpace(stateText), "unknown"),
		Enabled:      enabled,
		EnabledKnown: enabledErr == nil,
		Capabilities: tuiCapabilities{Reload: true},
	},
		statusIssue{Label: "service", Err: stateErr},
		statusIssue{Label: "boot", Err: enabledErr},
	)
}

func runAction(serviceName string, action string) error {
	serviceName, err := normalizeUnixServiceName(serviceName)
	if err != nil {
		return err
	}
	normalized, err := normalizeAction(action)
	if err != nil {
		return err
	}

	switch normalized {
	case "start", "stop", "restart", "reload", "enable", "disable":
		return elevatedSystemctl(normalized, serviceName)
	case "toggle":
		active, err := isServiceActive(serviceName)
		if err != nil {
			return err
		}
		if active {
			return elevatedSystemctl("stop", serviceName)
		}
		return elevatedSystemctl("start", serviceName)
	case "status":
		active, err := isServiceActive(serviceName)
		if err != nil {
			return err
		}
		if active {
			fmt.Println("active")
		} else {
			fmt.Println("inactive")
		}
		return nil
	default:
		return fmt.Errorf("unknown action: %s", normalized)
	}
}

func isServiceActive(serviceName string) (bool, error) {
	state, err := serviceStatusText(serviceName)
	return strings.EqualFold(state, "active"), err
}

func isServiceEnabled(serviceName string) (bool, error) {
	cmd, err := linuxSystemctlCmd("is-enabled", serviceName)
	if err != nil {
		return false, err
	}
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if enabled, ok := classifySystemctlEnabledStatus(text); ok {
		return enabled, nil
	}
	if err != nil {
		return false, fmt.Errorf("systemctl is-enabled %s failed: %s", serviceName, nonEmpty(text, err.Error()))
	}
	if text == "" {
		return false, fmt.Errorf("systemctl is-enabled %s returned empty output", serviceName)
	}
	return false, fmt.Errorf("systemctl is-enabled %s returned unrecognized state: %s", serviceName, text)
}

func serviceStatusText(serviceName string) (string, error) {
	cmd, err := linuxSystemctlCmd("is-active", serviceName)
	if err != nil {
		return "", err
	}
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if state, _, ok := classifySystemctlActiveStatus(text); ok {
		return state, nil
	}
	if err != nil {
		return nonEmpty(firstStatusLine(text), "unknown"), fmt.Errorf("systemctl is-active %s failed: %s", serviceName, nonEmpty(text, err.Error()))
	}
	if text == "" {
		return "unknown", fmt.Errorf("systemctl is-active %s returned empty output", serviceName)
	}
	return nonEmpty(firstStatusLine(text), "unknown"), fmt.Errorf("systemctl is-active %s returned unrecognized state: %s", serviceName, text)
}

func elevatedSystemctl(action, serviceName string) error {
	cmd, err := linuxSystemctlCmd(action, serviceName)
	if err != nil {
		return err
	}
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}

	outText := strings.TrimSpace(string(out))
	if !requiresPrivilegedRetry(outText) {
		return fmt.Errorf("systemctl %s %s failed: %s", action, serviceName, nonEmpty(outText, err.Error()))
	}

	if os.Geteuid() == 0 {
		return fmt.Errorf("systemctl %s %s failed: %s", action, serviceName, nonEmpty(outText, err.Error()))
	}

	sudo, sudoResolveErr := linuxSudoSystemctlCmd(action, serviceName)
	if sudoResolveErr != nil {
		return fmt.Errorf("systemctl %s %s failed: %s | sudo: %s", action, serviceName, nonEmpty(outText, err.Error()), sudoResolveErr.Error())
	}
	sudoOut, sudoErr := sudo.CombinedOutput()
	if sudoErr == nil {
		return nil
	}

	pkexec, pkexecResolveErr := linuxPkexecSystemctlCmd(action, serviceName)
	if pkexecResolveErr != nil {
		return fmt.Errorf(
			"systemctl %s %s failed: %s | sudo: %s | pkexec: %s",
			action,
			serviceName,
			nonEmpty(outText, err.Error()),
			nonEmpty(strings.TrimSpace(string(sudoOut)), sudoErr.Error()),
			pkexecResolveErr.Error(),
		)
	}
	pkOut, pkErr := pkexec.CombinedOutput()
	if pkErr == nil {
		return nil
	}

	return fmt.Errorf(
		"systemctl %s %s failed: %s | sudo: %s | pkexec: %s",
		action,
		serviceName,
		nonEmpty(outText, err.Error()),
		nonEmpty(strings.TrimSpace(string(sudoOut)), sudoErr.Error()),
		nonEmpty(strings.TrimSpace(string(pkOut)), pkErr.Error()),
	)
}

func requiresPrivilegedRetry(output string) bool {
	text := strings.ToLower(strings.TrimSpace(output))
	if text == "" {
		return true
	}
	signals := []string{
		"access denied",
		"permission denied",
		"not permitted",
		"interactive authentication required",
		"authentication is required",
		"authorization failed",
	}
	for _, s := range signals {
		if strings.Contains(text, s) {
			return true
		}
	}
	return false
}

func nonEmpty(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return strings.TrimSpace(primary)
	}
	return strings.TrimSpace(fallback)
}

func hasCommand(name string) bool {
	_, ok := linuxLookPath(name)
	return ok
}

func whiptailCommand(args ...string) (*exec.Cmd, error) {
	path, ok := linuxLookPath("whiptail")
	if !ok {
		return nil, fmt.Errorf("whiptail not found in trusted command directories")
	}
	// #nosec G204 -- path is resolved from trusted command directories and arguments stay positional.
	return exec.Command(path, args...), nil
}

func systemctlCommand(args ...string) (*exec.Cmd, error) {
	path, ok := linuxLookPath("systemctl")
	if !ok {
		return nil, fmt.Errorf("systemctl not found in trusted command directories")
	}
	// #nosec G204 -- action and service name are normalized through allowlists before invocation.
	return exec.Command(path, args...), nil
}

func sudoSystemctlCommand(action, serviceName string) (*exec.Cmd, error) {
	sudoPath, ok := linuxLookPath("sudo")
	if !ok {
		return nil, fmt.Errorf("sudo not found in trusted command directories")
	}
	systemctlPath, ok := linuxLookPath("systemctl")
	if !ok {
		return nil, fmt.Errorf("systemctl not found in trusted command directories")
	}
	// #nosec G204 -- action and service name are normalized through allowlists before invocation.
	return exec.Command(sudoPath, "-n", systemctlPath, action, serviceName), nil
}

func pkexecSystemctlCommand(action, serviceName string) (*exec.Cmd, error) {
	pkexecPath, ok := linuxLookPath("pkexec")
	if !ok {
		return nil, fmt.Errorf("pkexec not found in trusted command directories")
	}
	systemctlPath, ok := linuxLookPath("systemctl")
	if !ok {
		return nil, fmt.Errorf("systemctl not found in trusted command directories")
	}
	// #nosec G204 -- action and service name are normalized through allowlists before invocation.
	return exec.Command(pkexecPath, systemctlPath, action, serviceName), nil
}
