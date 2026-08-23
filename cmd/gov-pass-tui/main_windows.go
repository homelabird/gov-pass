//go:build windows

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const defaultServiceName = "gov-pass"

var windowsRunSC = runSC

func main() {
	serviceName := flag.String("service-name", defaultServiceName, "Windows service name to control")
	action := flag.String("action", "", "action mode: start|stop|restart|reload|enable|disable|toggle|status (runs and exits)")
	flag.Parse()

	name := strings.TrimSpace(*serviceName)
	if name == "" {
		name = defaultServiceName
	}
	name, err := normalizeWindowsServiceName(name)
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
	state, stateErr := queryServiceState(serviceName)
	return newTUIStatus(tuiStatusInput{
		Platform:    "Windows",
		ServiceName: serviceName,
		RawState:    state,
		Active:      strings.EqualFold(state, "running"),
		ActiveKnown: stateErr == nil && !strings.EqualFold(strings.TrimSpace(state), "unknown"),
	},
		statusIssue{Label: "service", Err: stateErr},
	)
}

func runAction(serviceName string, action string) error {
	serviceName, err := normalizeWindowsServiceName(serviceName)
	if err != nil {
		return err
	}
	normalized, err := normalizeAction(action)
	if err != nil {
		return err
	}

	switch normalized {
	case "start":
		return startService(serviceName)
	case "stop":
		return stopService(serviceName)
	case "restart":
		return restartService(serviceName)
	case "reload":
		return reloadService(serviceName)
	case "enable":
		return setServiceStartMode(serviceName, "auto")
	case "disable":
		return setServiceStartMode(serviceName, "disabled")
	case "toggle":
		active, err := isServiceActive(serviceName)
		if err != nil {
			return err
		}
		if active {
			return stopService(serviceName)
		}
		return startService(serviceName)
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

func startService(serviceName string) error {
	out, err := windowsRunSC("start", serviceName)
	if err != nil {
		if isAlreadyRunningError(nonEmpty(out, err.Error())) {
			return nil
		}
		return err
	}
	return waitForState(serviceName, "running", 15*time.Second)
}

func stopService(serviceName string) error {
	out, err := windowsRunSC("stop", serviceName)
	if err != nil {
		if isAlreadyStoppedError(nonEmpty(out, err.Error())) {
			return nil
		}
		return err
	}
	return waitForState(serviceName, "stopped", 15*time.Second)
}

func restartService(serviceName string) error {
	if err := stopService(serviceName); err != nil && !isAlreadyStoppedError(err.Error()) {
		return err
	}
	return startService(serviceName)
}

func reloadService(serviceName string) error {
	_, err := windowsRunSC("control", serviceName, "paramchange")
	return err
}

func setServiceStartMode(serviceName, mode string) error {
	_, err := windowsRunSC("config", serviceName, "start=", mode)
	return err
}

func isServiceActive(serviceName string) (bool, error) {
	state, err := queryServiceState(serviceName)
	if err != nil {
		return false, err
	}
	return strings.EqualFold(state, "running"), nil
}

func queryServiceState(serviceName string) (string, error) {
	out, err := windowsRunSC("query", serviceName)
	if err != nil {
		return "unknown", err
	}
	state := parseSCState(out)
	if state == "" {
		return "unknown", fmt.Errorf("sc query %s returned unrecognized STATE output", serviceName)
	}
	return strings.ToLower(state), nil
}

func waitForState(serviceName, want string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		state, err := queryServiceState(serviceName)
		if err == nil && strings.EqualFold(state, want) {
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for service state: %s", want)
}

func parseSCState(output string) string {
	lines := strings.Split(output, "\n")
	for _, ln := range lines {
		line := strings.TrimSpace(ln)
		if !strings.Contains(strings.ToUpper(line), "STATE") {
			continue
		}
		parts := strings.Fields(line)
		for i := len(parts) - 1; i >= 0; i-- {
			p := strings.TrimSpace(parts[i])
			if p == "" || strings.Contains(p, ":") || strings.HasPrefix(p, "(") {
				continue
			}
			return strings.ToLower(p)
		}
	}
	return ""
}

func runSC(args ...string) (string, error) {
	path, err := resolveTrustedWindowsCommand("sc")
	if err != nil {
		return "", err
	}
	cmd := exec.Command(path, args...)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		return text, fmt.Errorf("sc %s failed: %s", strings.Join(args, " "), nonEmpty(text, err.Error()))
	}
	return text, nil
}

func isAlreadyStoppedError(msg string) bool {
	text := strings.ToLower(msg)
	return strings.Contains(text, "1062") || strings.Contains(text, "has not been started")
}

func isAlreadyRunningError(msg string) bool {
	text := strings.ToLower(msg)
	return strings.Contains(text, "1056") || strings.Contains(text, "already running")
}

func nonEmpty(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return strings.TrimSpace(primary)
	}
	return strings.TrimSpace(fallback)
}
