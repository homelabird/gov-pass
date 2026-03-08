//go:build linux

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func writeLinuxSystemctlStatusScript(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "systemctl-fake")
	script := `#!/bin/sh
set -eu
mode="${1:-}"
shift || true
case "$mode" in
  is-active)
    printf '%s\n' "${FAKE_SYSTEMCTL_IS_ACTIVE_OUTPUT:-inactive}"
    exit "${FAKE_SYSTEMCTL_IS_ACTIVE_EXIT:-3}"
    ;;
  is-enabled)
    printf '%s\n' "${FAKE_SYSTEMCTL_IS_ENABLED_OUTPUT:-disabled}"
    exit "${FAKE_SYSTEMCTL_IS_ENABLED_EXIT:-1}"
    ;;
esac
printf '%s\n' "unexpected invocation: $mode $*" >&2
exit 64
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake systemctl: %v", err)
	}
	return path
}

func stubLinuxSystemctlStatus(t *testing.T) {
	t.Helper()
	script := writeLinuxSystemctlStatusScript(t)

	origSystemctl := linuxSystemctlCmd
	t.Cleanup(func() {
		linuxSystemctlCmd = origSystemctl
	})

	linuxSystemctlCmd = func(args ...string) (*exec.Cmd, error) {
		return exec.Command(script, args...), nil
	}
}

func TestServiceStatusText_FailedStateDoesNotWarn(t *testing.T) {
	stubLinuxSystemctlStatus(t)
	t.Setenv("FAKE_SYSTEMCTL_IS_ACTIVE_OUTPUT", "failed")
	t.Setenv("FAKE_SYSTEMCTL_IS_ACTIVE_EXIT", "3")

	state, err := serviceStatusText("gov-pass")
	if err != nil {
		t.Fatalf("serviceStatusText unexpected error: %v", err)
	}
	if state != "failed" {
		t.Fatalf("serviceStatusText = %q, want failed", state)
	}

	active, err := isServiceActive("gov-pass")
	if err != nil {
		t.Fatalf("isServiceActive unexpected error: %v", err)
	}
	if active {
		t.Fatal("isServiceActive = true, want false")
	}
}

func TestServiceStatusText_UnknownStateWarns(t *testing.T) {
	stubLinuxSystemctlStatus(t)
	t.Setenv("FAKE_SYSTEMCTL_IS_ACTIVE_OUTPUT", "unknown")
	t.Setenv("FAKE_SYSTEMCTL_IS_ACTIVE_EXIT", "3")

	state, err := serviceStatusText("gov-pass")
	if err == nil {
		t.Fatal("serviceStatusText expected error for unknown state")
	}
	if state != "unknown" {
		t.Fatalf("serviceStatusText = %q, want unknown", state)
	}
}

func TestIsServiceEnabled_DisabledDoesNotWarn(t *testing.T) {
	stubLinuxSystemctlStatus(t)
	t.Setenv("FAKE_SYSTEMCTL_IS_ENABLED_OUTPUT", "disabled")
	t.Setenv("FAKE_SYSTEMCTL_IS_ENABLED_EXIT", "1")

	enabled, err := isServiceEnabled("gov-pass")
	if err != nil {
		t.Fatalf("isServiceEnabled unexpected error: %v", err)
	}
	if enabled {
		t.Fatal("isServiceEnabled = true, want false")
	}
}

func TestIsServiceEnabled_NotFoundWarns(t *testing.T) {
	stubLinuxSystemctlStatus(t)
	t.Setenv("FAKE_SYSTEMCTL_IS_ENABLED_OUTPUT", "not-found")
	t.Setenv("FAKE_SYSTEMCTL_IS_ENABLED_EXIT", "1")

	if _, err := isServiceEnabled("gov-pass"); err == nil {
		t.Fatal("isServiceEnabled expected error for not-found state")
	}
}
