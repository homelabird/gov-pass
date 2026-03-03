//go:build windows

package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/sys/windows/svc"
)

const defaultServiceName = "gov-pass"

func main() {
	serviceName := flag.String("service-name", defaultServiceName, "Windows service name to control")
	action := flag.String("action", "", "action mode: start|stop|restart|reload|enable|disable|toggle|status (runs and exits)")
	flag.Parse()

	name := strings.TrimSpace(*serviceName)
	if name == "" {
		name = defaultServiceName
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
		fmt.Println("=== gov-pass TUI (Windows) ===")
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
	state, stateErr := queryServiceState(serviceName)
	active, activeErr := isServiceActive(serviceName)
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

	if stateErr != nil || activeErr != nil || enabledErr != nil {
		lines = append(lines, "Warning: some status checks failed.")
	}
	return strings.Join(lines, "\n")
}

func runAction(serviceName string, action string) error {
	normalized, err := normalizeAction(action)
	if err != nil {
		return err
	}

	switch normalized {
	case "start":
		return startService(serviceName)
	case "stop":
		return stopService(serviceName)
	case "restart", "reload":
		return restartService(serviceName)
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
	out, err := runSC("start", serviceName)
	if err != nil {
		if isAlreadyRunningError(nonEmpty(out, err.Error())) {
			return nil
		}
		return err
	}
	return waitForState(serviceName, "running", 15*time.Second)
}

func stopService(serviceName string) error {
	out, err := runSC("stop", serviceName)
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

func setServiceStartMode(serviceName, mode string) error {
	_, err := runSC("config", serviceName, "start=", mode)
	return err
}

func isServiceActive(serviceName string) (bool, error) {
	state, err := queryServiceState(serviceName)
	if err != nil {
		return false, err
	}
	return strings.EqualFold(state, "running"), nil
}

func isServiceEnabled(serviceName string) (bool, error) {
	out, err := runSC("qc", serviceName)
	if err != nil {
		return false, err
	}
	upper := strings.ToUpper(out)
	if strings.Contains(upper, "AUTO_START") {
		return true, nil
	}
	if strings.Contains(upper, "DISABLED") {
		return false, nil
	}
	return false, nil
}

func queryServiceState(serviceName string) (string, error) {
	out, err := runSC("query", serviceName)
	if err != nil {
		return "unknown", err
	}
	state := parseSCState(out)
	if state == "" {
		return "unknown", nil
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
	cmd := exec.Command("sc", args...)
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

func clearTerminalScreen() {
	fmt.Print("\033[H\033[2J")
}

func nonEmpty(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return strings.TrimSpace(primary)
	}
	return strings.TrimSpace(fallback)
}

func quoteArgs(args []string) string {
	var b strings.Builder
	for i := range args {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(quoteArg(args[i]))
	}
	return b.String()
}

func quoteArg(s string) string {
	if s == "" {
		return `""`
	}
	if !strings.ContainsAny(s, " \t\"") {
		return s
	}
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

func stateString(st svc.State) string {
	switch st {
	case svc.Stopped:
		return "stopped"
	case svc.StartPending:
		return "start-pending"
	case svc.StopPending:
		return "stop-pending"
	case svc.Running:
		return "running"
	case svc.ContinuePending:
		return "continue-pending"
	case svc.PausePending:
		return "pause-pending"
	case svc.Paused:
		return "paused"
	default:
		return fmt.Sprintf("unknown(%d)", st)
	}
}
