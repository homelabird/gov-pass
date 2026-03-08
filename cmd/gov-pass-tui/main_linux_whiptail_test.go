//go:build linux

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func writeLinuxWhiptailScript(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fake-whiptail")
	script := `#!/bin/sh
set -eu
choice_file="${FAKE_WHIPTAIL_CHOICES:?}"
log_file="${FAKE_WHIPTAIL_LOG:?}"
mode=""
msg=""
prev=""
for arg in "$@"; do
  if [ "$prev" = "--msgbox" ]; then
    msg="$arg"
  fi
  if [ "$arg" = "--menu" ]; then
    mode="menu"
  elif [ "$arg" = "--msgbox" ]; then
    mode="msgbox"
  fi
  prev="$arg"
done
case "$mode" in
  menu)
    if [ ! -s "$choice_file" ]; then
      exit 1
    fi
    IFS= read -r choice < "$choice_file" || exit 1
    awk 'NR > 1 { print }' "$choice_file" > "${choice_file}.tmp"
    mv "${choice_file}.tmp" "$choice_file"
    if [ "$choice" = "__CANCEL__" ]; then
      exit 1
    fi
    printf '%s' "$choice"
    exit 0
    ;;
  msgbox)
    printf '%s\n' "$msg" >> "$log_file"
    exit 0
    ;;
esac
printf 'unexpected whiptail invocation: %s\n' "$*" >&2
exit 64
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake whiptail: %v", err)
	}
	return path
}

func writeLinuxSystemctlActionScript(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fake-systemctl")
	script := `#!/bin/sh
set -eu
cmd="${1:-}"
shift || true
case "$cmd" in
  is-active)
    printf '%s\n' "inactive"
    exit 3
    ;;
  is-enabled)
    printf '%s\n' "disabled"
    exit 1
    ;;
  restart|start|stop|reload|enable|disable)
    exit 0
    ;;
esac
printf 'unexpected systemctl invocation: %s %s\n' "$cmd" "$*" >&2
exit 64
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake systemctl: %v", err)
	}
	return path
}

func stubLinuxWhiptailFlow(t *testing.T) (choicesPath, logPath string) {
	t.Helper()
	whiptailScript := writeLinuxWhiptailScript(t)
	systemctlScript := writeLinuxSystemctlActionScript(t)
	choicesPath = filepath.Join(t.TempDir(), "choices.txt")
	logPath = filepath.Join(t.TempDir(), "msgbox.log")

	origLookPath := linuxLookPath
	origSystemctl := linuxSystemctlCmd
	t.Cleanup(func() {
		linuxLookPath = origLookPath
		linuxSystemctlCmd = origSystemctl
	})

	linuxLookPath = func(name string) (string, bool) {
		switch name {
		case "whiptail":
			return whiptailScript, true
		default:
			return "", false
		}
	}
	linuxSystemctlCmd = func(args ...string) (*exec.Cmd, error) {
		cmd := exec.Command(systemctlScript, args...)
		cmd.Env = append(os.Environ(), "FAKE_WHIPTAIL_CHOICES="+choicesPath, "FAKE_WHIPTAIL_LOG="+logPath)
		return cmd, nil
	}

	t.Setenv("FAKE_WHIPTAIL_CHOICES", choicesPath)
	t.Setenv("FAKE_WHIPTAIL_LOG", logPath)
	return choicesPath, logPath
}

func TestRunWhiptailTUI_CancelReturnsNil(t *testing.T) {
	choicesPath, _ := stubLinuxWhiptailFlow(t)
	if err := os.WriteFile(choicesPath, []byte("__CANCEL__\n"), 0o644); err != nil {
		t.Fatalf("write choices: %v", err)
	}

	if err := runWhiptailTUI("gov-pass"); err != nil {
		t.Fatalf("runWhiptailTUI unexpected error: %v", err)
	}
}

func TestRunWhiptailTUI_ActionDisplaysMessage(t *testing.T) {
	choicesPath, logPath := stubLinuxWhiptailFlow(t)
	if err := os.WriteFile(choicesPath, []byte("2\n__CANCEL__\n"), 0o644); err != nil {
		t.Fatalf("write choices: %v", err)
	}

	if err := runWhiptailTUI("gov-pass"); err != nil {
		t.Fatalf("runWhiptailTUI unexpected error: %v", err)
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read msgbox log: %v", err)
	}
	if !strings.Contains(string(logBytes), "Service restart requested.") {
		t.Fatalf("msgbox log missing restart confirmation: %q", string(logBytes))
	}
}
