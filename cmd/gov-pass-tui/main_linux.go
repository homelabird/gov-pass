//go:build linux

package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const defaultServiceName = "gov-pass"

func main() {
	serviceName := flag.String("service-name", defaultServiceName, "systemd service name to control")
	action := flag.String("action", "", "action mode: start|stop|restart|reload|enable|disable|toggle|status (runs and exits)")
	flag.Parse()

	name := strings.TrimSpace(*serviceName)
	if name == "" {
		name = defaultServiceName
	}
	name, err := normalizeServiceName(name)
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
	if hasCommand("whiptail") {
		return runWhiptailTUI(serviceName)
	}
	return runPlainTUI(serviceName)
}

func runWhiptailTUI(serviceName string) error {
	for {
		choice, canceled, err := whiptailMenu(serviceName, buildStatusSummary(serviceName))
		if err != nil {
			return err
		}
		if canceled || choice == "" || choice == "q" {
			return nil
		}

		msg, err := executeMenuChoice(serviceName, choice)
		if err != nil {
			_ = whiptailMessage("gov-pass error", err.Error())
			continue
		}
		if strings.TrimSpace(msg) != "" {
			_ = whiptailMessage("gov-pass", msg)
		}
	}
}

func whiptailMenu(serviceName, summary string) (choice string, canceled bool, err error) {
	args := []string{
		"--title", "gov-pass control",
		"--menu", summary,
		"18", "88", "9",
		"1", fmt.Sprintf("Start/Stop service (%s)", serviceName),
		"2", "Restart service",
		"3", fmt.Sprintf("Enable/Disable boot start (%s)", serviceName),
		"q", "Quit",
		"--output-fd", "1",
	}
	cmd := whiptailCommand(args...)
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
	cmd := whiptailCommand("--title", title, "--msgbox", text, "16", "90")
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func whiptailShowText(title, text string) error {
	cmd := whiptailCommand("--title", title, "--scrolltext", "--msgbox", text, "28", "110")
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runPlainTUI(serviceName string) error {
	scanner := bufio.NewScanner(os.Stdin)
	lastNote := "Ready."

	for {
		renderPlainTUI(serviceName, lastNote)
		fmt.Print("Select> ")

		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return err
			}
			return nil
		}
		choice := strings.ToLower(strings.TrimSpace(scanner.Text()))
		if choice == "" {
			lastNote = "No selection."
			continue
		}
		if choice == "q" || choice == "quit" || choice == "exit" {
			return nil
		}

		msg, err := executeMenuChoice(serviceName, choice)
		if err != nil {
			lastNote = "Error: " + err.Error()
			continue
		}
		if strings.TrimSpace(msg) != "" {
			lastNote = msg
		} else {
			lastNote = "Done."
		}
	}
}

func renderPlainTUI(serviceName, note string) {
	clearTerminalScreen()
	fmt.Println("=== gov-pass TUI ===")
	fmt.Println(buildStatusSummary(serviceName))
	fmt.Println()
	fmt.Printf("1. Start/Stop service (%s)\n", serviceName)
	fmt.Println("2. Restart service")
	fmt.Printf("3. Enable/Disable boot start (%s)\n", serviceName)
	fmt.Println("q. Quit")
	if strings.TrimSpace(note) != "" {
		fmt.Println()
		fmt.Println("Message:")
		fmt.Println(note)
	}
	fmt.Println()
}

func clearTerminalScreen() {
	// ANSI clear screen + move cursor home.
	fmt.Print("\033[H\033[2J")
}

func executeMenuChoice(serviceName, choice string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(choice)) {
	case "1", "switch", "start-stop":
		active, err := isServiceActive(serviceName)
		if err != nil {
			return "", err
		}
		if active {
			if err := runAction(serviceName, "stop"); err != nil {
				return "", err
			}
			return fmt.Sprintf("Service stopped: %s", serviceName), nil
		}
		if err := runAction(serviceName, "start"); err != nil {
			return "", err
		}
		return fmt.Sprintf("Service started: %s", serviceName), nil
	case "2", "restart":
		return "Service restart requested.", runAction(serviceName, "restart")
	case "3", "boot":
		enabled, err := isServiceEnabled(serviceName)
		if err != nil {
			return "", err
		}
		if enabled {
			if err := runAction(serviceName, "disable"); err != nil {
				return "", err
			}
			return fmt.Sprintf("Boot start disabled: %s", serviceName), nil
		}
		if err := runAction(serviceName, "enable"); err != nil {
			return "", err
		}
		return fmt.Sprintf("Boot start enabled: %s", serviceName), nil
	case "r", "refresh":
		return "", nil
	default:
		return "", fmt.Errorf("unknown selection: %s", choice)
	}
}

func buildStatusSummary(serviceName string) string {
	active, activeErr := isServiceActive(serviceName)
	stateText, stateErr := serviceStatusText(serviceName)
	enabled, enabledErr := isServiceEnabled(serviceName)

	state := "unknown"
	if stateText != "" {
		state = stateText
	}
	activeStr := "no"
	if active {
		activeStr = "yes"
	}
	enabledStr := "no"
	if enabled {
		enabledStr = "yes"
	}

	lines := []string{
		fmt.Sprintf("Service: %s", serviceName),
		fmt.Sprintf("Status: %s (active=%s)", state, activeStr),
		fmt.Sprintf("Enabled at boot: %s", enabledStr),
	}

	if activeErr != nil || stateErr != nil || enabledErr != nil {
		lines = append(lines, "Warning: some status checks failed.")
	}

	return strings.Join(lines, "\n")
}

func runAction(serviceName string, action string) error {
	serviceName, err := normalizeServiceName(serviceName)
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
	cmd := systemctlCommand("is-active", "--quiet", serviceName)
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, nil
	}
	return false, err
}

func isServiceEnabled(serviceName string) (bool, error) {
	cmd := systemctlCommand("is-enabled", "--quiet", serviceName)
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, nil
	}
	return false, err
}

func serviceStatusText(serviceName string) (string, error) {
	cmd := systemctlCommand("is-active", serviceName)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if text == "" {
		if err == nil {
			return "unknown", nil
		}
		return "", err
	}
	return text, nil
}

func serviceStatusDetail(serviceName string) (string, error) {
	cmd := systemctlCommand("status", "--no-pager", serviceName)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if text == "" && err != nil {
		return "", err
	}
	return text, nil
}

func serviceRecentLogs(serviceName string, lines int) (string, error) {
	lineArg := fmt.Sprintf("%d", lines)
	cmd := journalctlCommand("-u", serviceName, "-n", lineArg, "--no-pager")
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if text == "" && err != nil {
		return "", err
	}
	return text, nil
}

func elevatedSystemctl(action, serviceName string) error {
	cmd := systemctlCommand(action, serviceName)
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

	sudo := sudoSystemctlCommand(action, serviceName)
	sudoOut, sudoErr := sudo.CombinedOutput()
	if sudoErr == nil {
		return nil
	}

	pkexec := pkexecSystemctlCommand(action, serviceName)
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
	_, err := exec.LookPath(name)
	return err == nil
}

func whiptailCommand(args ...string) *exec.Cmd {
	// #nosec G204 -- runs a fixed binary without a shell; title/text arguments stay positional.
	return exec.Command("whiptail", args...)
}

func systemctlCommand(args ...string) *exec.Cmd {
	// #nosec G204 -- action and service name are normalized through allowlists before invocation.
	return exec.Command("systemctl", args...)
}

func journalctlCommand(args ...string) *exec.Cmd {
	// #nosec G204 -- service name and line count are normalized before invocation.
	return exec.Command("journalctl", args...)
}

func sudoSystemctlCommand(action, serviceName string) *exec.Cmd {
	// #nosec G204 -- action and service name are normalized through allowlists before invocation.
	return exec.Command("sudo", "-n", "systemctl", action, serviceName)
}

func pkexecSystemctlCommand(action, serviceName string) *exec.Cmd {
	// #nosec G204 -- action and service name are normalized through allowlists before invocation.
	return exec.Command("pkexec", "systemctl", action, serviceName)
}
