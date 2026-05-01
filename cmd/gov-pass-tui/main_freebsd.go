//go:build freebsd

package main

import (
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
	return runBubbleTUI(serviceName, collectTUIStatus)
}

func collectTUIStatus(serviceName string) tuiStatus {
	state, activeErr := serviceStatusText(serviceName)
	enabled, enabledErr := isServiceEnabled(serviceName)
	return newTUIStatus(tuiStatusInput{
		Platform:     "FreeBSD",
		ServiceName:  serviceName,
		RawState:     state,
		Active:       strings.EqualFold(state, "active"),
		ActiveKnown:  activeErr == nil && !strings.EqualFold(strings.TrimSpace(state), "unknown"),
		Enabled:      enabled,
		EnabledKnown: enabledErr == nil,
		Capabilities: tuiCapabilities{Reload: false},
	},
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
	cmd := newFreeBSDCommand(servicePath, serviceName, "onestatus")
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
	cmd := newFreeBSDCommand(sysrcPath, "-n", key)
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
	cmd := newFreeBSDCommand(path, args...)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		return text, fmt.Errorf("%s %s failed: %s", path, strings.Join(args, " "), nonEmpty(text, err.Error()))
	}
	return text, nil
}

func newFreeBSDCommand(path string, args ...string) *exec.Cmd {
	cmd := exec.Command(path, args...)
	cmd.Env = sanitizedFreeBSDCommandEnv()
	return cmd
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
