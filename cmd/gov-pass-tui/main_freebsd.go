//go:build freebsd

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
	serviceName := flag.String("service-name", defaultServiceName, "FreeBSD service name to control")
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
	scanner := bufio.NewScanner(os.Stdin)
	lastNote := "Ready."

	for {
		clearTerminalScreen()
		fmt.Println("=== gov-pass TUI (FreeBSD) ===")
		fmt.Println(buildStatusSummary(serviceName))
		fmt.Println()
		fmt.Printf("1. Start/Stop service (%s)\n", serviceName)
		fmt.Println("2. Restart service")
		fmt.Printf("3. Enable/Disable boot start (%s)\n", serviceName)
		fmt.Println("q. Quit")
		fmt.Println()
		fmt.Println("Message:")
		fmt.Println(lastNote)
		fmt.Println()
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
		if strings.TrimSpace(msg) == "" {
			lastNote = "Done."
		} else {
			lastNote = msg
		}
	}
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
		if err := runAction(serviceName, "restart"); err != nil {
			return "", err
		}
		return fmt.Sprintf("Service restarted: %s", serviceName), nil
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
	default:
		return "", fmt.Errorf("unknown selection: %s", choice)
	}
}

func buildStatusSummary(serviceName string) string {
	active, activeErr := isServiceActive(serviceName)
	state, stateErr := serviceStatusText(serviceName)
	enabled, enabledErr := isServiceEnabled(serviceName)

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
	case "start":
		return runPrivileged("service", serviceName, "onestart")
	case "stop":
		return runPrivileged("service", serviceName, "onestop")
	case "restart":
		return runPrivileged("service", serviceName, "onerestart")
	case "reload":
		return runPrivileged("service", serviceName, "onereload")
	case "enable":
		return runPrivileged("sysrc", fmt.Sprintf("%s_enable=YES", serviceName))
	case "disable":
		return runPrivileged("sysrc", fmt.Sprintf("%s_enable=NO", serviceName))
	case "toggle":
		active, err := isServiceActive(serviceName)
		if err != nil {
			return err
		}
		if active {
			return runPrivileged("service", serviceName, "onestop")
		}
		return runPrivileged("service", serviceName, "onestart")
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
	cmd := exec.Command("service", serviceName, "onestatus")
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
	key := fmt.Sprintf("%s_enable", serviceName)
	cmd := exec.Command("sysrc", "-n", key)
	out, err := cmd.CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return false, nil
		}
		return false, err
	}
	v := strings.ToLower(strings.TrimSpace(string(out)))
	return v == "yes" || v == "true" || v == "1" || v == "on", nil
}

func serviceStatusText(serviceName string) (string, error) {
	active, err := isServiceActive(serviceName)
	if err != nil {
		return "unknown", err
	}
	if active {
		return "active", nil
	}
	return "inactive", nil
}

func runPrivileged(name string, args ...string) error {
	if _, err := runCommand(name, args...); err == nil {
		return nil
	} else if !requiresPrivilegedRetry(err.Error()) || os.Geteuid() == 0 {
		return err
	} else {
		primaryErr := err

		if _, sudoErr := runCommand("sudo", append([]string{"-n", name}, args...)...); sudoErr == nil {
			return nil
		} else if hasCommand("doas") {
			if _, doasErr := runCommand("doas", append([]string{name}, args...)...); doasErr == nil {
				return nil
			} else {
				return fmt.Errorf("%v | doas: %v", primaryErr, doasErr)
			}
		} else {
			return fmt.Errorf("%v | sudo: %v", primaryErr, sudoErr)
		}
	}
}

func runCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		return text, fmt.Errorf("%s %s failed: %s", name, strings.Join(args, " "), nonEmpty(text, err.Error()))
	}
	return text, nil
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
		"operation not permitted",
		"must be root",
		"authentication required",
	}
	for _, s := range signals {
		if strings.Contains(text, s) {
			return true
		}
	}
	return false
}

func clearTerminalScreen() {
	fmt.Print("\033[H\033[2J")
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
