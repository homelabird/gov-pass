//go:build freebsd

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFreeBSDFlowCommands(t *testing.T) (dir, logPath string) {
	t.Helper()
	dir = t.TempDir()
	logPath = filepath.Join(dir, "invocations.log")

	serviceScript := `#!/bin/sh
set -eu
log_file="${FAKE_FREEBSD_LOG:?}"
service_name="${1:-}"
action="${2:-}"
case "$action" in
  onestatus)
    printf '%s\n' "${FAKE_FREEBSD_ONESTATUS_OUTPUT:-gov-pass is not running as a static service}"
    exit "${FAKE_FREEBSD_ONESTATUS_EXIT:-1}"
    ;;
  onestart|onestop|onerestart)
    printf 'service %s %s\n' "$service_name" "$action" >> "$log_file"
    exit 0
    ;;
esac
printf 'unexpected service invocation: %s %s\n' "$service_name" "$action" >&2
exit 64
`
	sysrcScript := `#!/bin/sh
set -eu
log_file="${FAKE_FREEBSD_LOG:?}"
if [ "${1:-}" = "-n" ]; then
  printf '%s\n' "${FAKE_FREEBSD_SYSRC_OUTPUT:-NO}"
  exit "${FAKE_FREEBSD_SYSRC_EXIT:-0}"
fi
printf 'sysrc %s\n' "$*" >> "$log_file"
exit 0
`
	sudoScript := `#!/bin/sh
set -eu
shift
exec "$@"
`

	files := map[string]string{
		"service": serviceScript,
		"sysrc":   sysrcScript,
		"sudo":    sudoScript,
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir, logPath
}

func stubFreeBSDFlowCommands(t *testing.T) string {
	t.Helper()
	dir, logPath := writeFreeBSDFlowCommands(t)
	origDirs := trustedFreeBSDCommandDirs
	trustedFreeBSDCommandDirs = []string{dir}
	t.Cleanup(func() {
		trustedFreeBSDCommandDirs = origDirs
	})
	t.Setenv("FAKE_FREEBSD_LOG", logPath)
	return logPath
}

func TestExecuteMenuChoice_StartsInactiveService(t *testing.T) {
	logPath := stubFreeBSDFlowCommands(t)
	t.Setenv("FAKE_FREEBSD_ONESTATUS_OUTPUT", "gov-pass is not running as a static service")
	t.Setenv("FAKE_FREEBSD_ONESTATUS_EXIT", "1")
	t.Setenv("FAKE_FREEBSD_SYSRC_OUTPUT", "NO")
	t.Setenv("FAKE_FREEBSD_SYSRC_EXIT", "0")

	status := collectTUIStatus("gov-pass")
	feedback, err := executeMenuChoice("gov-pass", status, "toggle")
	if err != nil {
		t.Fatalf("executeMenuChoice unexpected error: %v", err)
	}
	if feedback.Summary != "Service start requested." {
		t.Fatalf("executeMenuChoice summary = %q", feedback.Summary)
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(logBytes), "service gov-pass onestart") {
		t.Fatalf("expected onestart invocation, got %q", string(logBytes))
	}
}

func TestRunAction_DisablesBoot(t *testing.T) {
	logPath := stubFreeBSDFlowCommands(t)

	if err := runAction("gov-pass", "disable"); err != nil {
		t.Fatalf("runAction unexpected error: %v", err)
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(logBytes), "sysrc gov_pass_enable=NO") {
		t.Fatalf("expected sysrc disable invocation, got %q", string(logBytes))
	}
}

func TestServiceStatusText_WarnsOnUnknownStatus(t *testing.T) {
	stubFreeBSDFlowCommands(t)
	t.Setenv("FAKE_FREEBSD_ONESTATUS_OUTPUT", "service gov-pass does not exist")
	t.Setenv("FAKE_FREEBSD_ONESTATUS_EXIT", "1")

	state, err := serviceStatusText("gov-pass")
	if err == nil {
		t.Fatal("serviceStatusText expected error")
	}
	if state != "unknown" {
		t.Fatalf("serviceStatusText = %q, want unknown", state)
	}
}
