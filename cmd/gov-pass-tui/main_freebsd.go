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
	action := flag.String("action", "", "action mode: start|stop|restart|enable|disable|toggle|status (runs and exits); reload is not supported")
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
	scanner := bufio.NewScanner(os.Stdin)
	lastNote := "Ready."

	for {
		clearTerminalScreen()
		fmt.Print(renderPlainTUIView(collectTUIStatus(serviceName), lastNote))
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
	case "r", "refresh":
		return "", nil
	default:
		return "", fmt.Errorf("unknown selection: %s", choice)
	}
}

func collectTUIStatus(serviceName string) tuiStatus {
	state, activeErr := serviceStatusText(serviceName)
	enabled, enabledErr := isServiceEnabled(serviceName)
	return newTUIStatus(
		"FreeBSD",
		serviceName,
		state,
		strings.EqualFold(state, "active"),
		enabled,
		statusIssue{Label: "service", Err: activeErr},
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
	case "start":
		return runPrivileged("service", serviceName, "onestart")
	case "stop":
		return runPrivileged("service", serviceName, "onestop")
	case "restart":
		return runPrivileged("service", serviceName, "onerestart")
	case "reload":
		return errors.New("reload is not supported on FreeBSD; use restart")
	case "enable":
		return runPrivileged("sysrc", fmt.Sprintf("%s=YES", freeBSDRcVarName(serviceName)))
	case "disable":
		return runPrivileged("sysrc", fmt.Sprintf("%s=NO", freeBSDRcVarName(serviceName)))
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
	state, err := serviceStatusText(serviceName)
	return strings.EqualFold(state, "active"), err
}

func serviceStatusText(serviceName string) (string, error) {
	servicePath, err := resolveTrustedFreeBSDCommand("service")
	if err != nil {
		return "", err
	}
	cmd := exec.Command(servicePath, serviceName, "onestatus")
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if state, _, ok := classifyFreeBSDServiceStatusOutput(text, err == nil); ok {
		return state, nil
	}
	if err != nil {
		return "unknown", fmt.Errorf("service %s onestatus failed: %s", serviceName, nonEmpty(text, err.Error()))
	}
	return "unknown", fmt.Errorf("service %s onestatus returned unrecognized status", serviceName)
}

func isServiceEnabled(serviceName string) (bool, error) {
	key := freeBSDRcVarName(serviceName)
	sysrcPath, err := resolveTrustedFreeBSDCommand("sysrc")
	if err != nil {
		return false, err
	}
	cmd := exec.Command(sysrcPath, "-n", key)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err == nil {
		if enabled, ok := classifyFreeBSDBootSetting(text); ok {
			return enabled, nil
		}
		return false, fmt.Errorf("sysrc -n %s returned unrecognized value: %s", key, text)
	}
	return false, fmt.Errorf("sysrc -n %s failed: %s", key, nonEmpty(text, err.Error()))
}

func runPrivileged(name string, args ...string) error {
	targetPath, err := resolveTrustedFreeBSDCommand(name)
	if err != nil {
		return err
	}
	if _, err := runCommandPath(targetPath, args...); err == nil {
		return nil
	} else if !requiresPrivilegedRetry(err.Error()) || os.Geteuid() == 0 {
		return err
	} else {
		primaryErr := err

		if _, sudoErr := runCommand("sudo", append([]string{"-n", targetPath}, args...)...); sudoErr == nil {
			return nil
		} else if hasCommand("doas") {
			if _, doasErr := runCommand("doas", append([]string{targetPath}, args...)...); doasErr == nil {
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
	path, err := resolveTrustedFreeBSDCommand(name)
	if err != nil {
		return "", err
	}
	return runCommandPath(path, args...)
}

func runCommandPath(path string, args ...string) (string, error) {
	cmd := exec.Command(path, args...)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		return text, fmt.Errorf("%s %s failed: %s", path, strings.Join(args, " "), nonEmpty(text, err.Error()))
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
	_, ok := freeBSDCommandLookPath(name)
	return ok
}
