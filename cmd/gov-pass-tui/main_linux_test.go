//go:build linux

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func writeLinuxTUITestScript(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fakesvc")
	script := `#!/bin/sh
set -eu
mode="${1:-}"
shift || true
case "$mode" in
  systemctl)
	    case "${1:-}" in
	      is-active)
	        if [ "${2:-}" = "--quiet" ]; then
	          exit 1
	        fi
	        printf '%s\n' "inactive"
	        exit 0
	        ;;
	      is-enabled)
	        exit 1
	        ;;
	      show)
	        printf '%s\n' "${FAKE_SYSTEMCTL_CAN_RELOAD_OUTPUT:-yes}"
	        exit 0
	        ;;
	      start|stop|restart|reload|enable|disable)
	        exit 0
	        ;;
	    esac
    ;;
  sudo-systemctl|pkexec-systemctl)
    exit 0
    ;;
esac
printf '%s\n' "unexpected invocation: $mode $*" >&2
exit 64
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake command: %v", err)
	}
	return path
}

func stubLinuxTUICommands(t *testing.T) {
	t.Helper()
	script := writeLinuxTUITestScript(t)

	origLookPath := linuxLookPath
	origSystemctl := linuxSystemctlCmd
	origSudo := linuxSudoSystemctlCmd
	origPkexec := linuxPkexecSystemctlCmd
	t.Cleanup(func() {
		linuxLookPath = origLookPath
		linuxSystemctlCmd = origSystemctl
		linuxSudoSystemctlCmd = origSudo
		linuxPkexecSystemctlCmd = origPkexec
	})

	linuxLookPath = func(name string) (string, bool) {
		switch name {
		case "whiptail", "systemctl", "sudo", "pkexec":
			return script, true
		default:
			return "", false
		}
	}
	linuxSystemctlCmd = func(args ...string) (*exec.Cmd, error) {
		return exec.Command(script, append([]string{"systemctl"}, args...)...), nil
	}
	linuxSudoSystemctlCmd = func(action, serviceName string) (*exec.Cmd, error) {
		return exec.Command(script, "sudo-systemctl", action, serviceName), nil
	}
	linuxPkexecSystemctlCmd = func(action, serviceName string) (*exec.Cmd, error) {
		return exec.Command(script, "pkexec-systemctl", action, serviceName), nil
	}
}

func TestRunAction_UnknownAction(t *testing.T) {
	stubLinuxTUICommands(t)
	err := runAction("gov-pass", "unknown-action")
	if err == nil {
		t.Fatal("expected error for unknown action")
	}
	if got := err.Error(); got != "unknown action: unknown-action" {
		t.Fatalf("unexpected error message: %s", got)
	}
}

func TestRunAction_ValidActions(t *testing.T) {
	stubLinuxTUICommands(t)
	for _, action := range []string{"start", "stop", "restart", "reload", "enable", "disable", "toggle", "status"} {
		err := runAction("nonexistent-test-service-gov-pass", action)
		if err != nil && err.Error() == "unknown action: "+action {
			t.Errorf("action %q should be recognized", action)
		}
	}
}

func TestRunAction_ReloadRejectedWhenUnsupported(t *testing.T) {
	stubLinuxTUICommands(t)
	t.Setenv("FAKE_SYSTEMCTL_CAN_RELOAD_OUTPUT", "no")

	err := runAction("gov-pass", "reload")
	if err == nil {
		t.Fatal("expected reload to be rejected when CanReload=no")
	}
	if got := err.Error(); got != "reload is not available for gov-pass" {
		t.Fatalf("unexpected error message: %s", got)
	}
}

func TestRunAction_InvalidServiceName(t *testing.T) {
	stubLinuxTUICommands(t)
	if err := runAction("gov pass", "status"); err == nil {
		t.Fatal("expected invalid service name error")
	}
}
