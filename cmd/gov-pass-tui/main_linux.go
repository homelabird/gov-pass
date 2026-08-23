//go:build linux

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
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

func collectTUIStatus(serviceName string) tuiStatus {
	stateText, stateErr := serviceStatusText(serviceName)
	return newTUIStatus(tuiStatusInput{
		Platform:    "Linux",
		ServiceName: serviceName,
		RawState:    stateText,
		Active:      strings.EqualFold(stateText, "active"),
		ActiveKnown: stateErr == nil && !strings.EqualFold(strings.TrimSpace(stateText), "unknown"),
	},
		statusIssue{Label: "service", Err: stateErr},
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
		if normalized == "reload" {
			canReload, err := serviceCanReload(serviceName)
			if err != nil {
				return err
			}
			if !canReload {
				return fmt.Errorf("reload is not available for %s", serviceName)
			}
		}
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

func serviceCanReload(serviceName string) (bool, error) {
	cmd, err := linuxSystemctlCmd("show", "--property=CanReload", "--value", serviceName)
	if err != nil {
		return false, err
	}
	out, err := cmd.CombinedOutput()
	text := firstStatusLine(string(out))
	switch text {
	case "yes", "true", "1":
		return true, nil
	case "no", "false", "0":
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("systemctl show %s -p CanReload failed: %s", serviceName, nonEmpty(strings.TrimSpace(string(out)), err.Error()))
	}
	if text == "" {
		return false, fmt.Errorf("systemctl show %s -p CanReload returned empty output", serviceName)
	}
	return false, fmt.Errorf("systemctl show %s -p CanReload returned unrecognized state: %s", serviceName, text)
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

func systemctlCommand(args ...string) (*exec.Cmd, error) {
	path, ok := linuxLookPath("systemctl")
	if !ok {
		return nil, fmt.Errorf("systemctl not found in trusted command directories")
	}
	// #nosec G204 -- action and service name are normalized through allowlists before invocation.
	return newLinuxTUICommand(path, args...), nil
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
	return newLinuxTUICommand(sudoPath, "-n", systemctlPath, action, serviceName), nil
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
	return newLinuxTUICommand(pkexecPath, systemctlPath, action, serviceName), nil
}

func newLinuxTUICommand(path string, args ...string) *exec.Cmd {
	cmd := exec.Command(path, args...)
	cmd.Env = sanitizedLinuxTUICommandEnv()
	return cmd
}
