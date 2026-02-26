//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeExecScript(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fakecmd")
	script := "#!/usr/bin/env sh\nset -eu\n" + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	return path
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	s := strings.TrimSpace(string(data))
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func assertLineContains(t *testing.T, lines []string, want string) {
	t.Helper()
	for _, line := range lines {
		if strings.Contains(line, want) {
			return
		}
	}
	t.Fatalf("expected line containing %q, got: %v", want, lines)
}

func TestRunCommandAndEnv(t *testing.T) {
	cmd := writeExecScript(t, `
if [ "${1:-}" = "fail" ]; then
  echo "boom" >&2
  exit 7
fi
if [ "${1:-}" = "printenv" ]; then
  echo "X_VAR=${X_VAR:-}"
  exit 0
fi
echo "ok:$*"
`)

	out, err := runCommand(cmd, "a", "b")
	if err != nil {
		t.Fatalf("runCommand success unexpected error: %v", err)
	}
	if !strings.Contains(out, "ok:a b") {
		t.Fatalf("unexpected output: %q", out)
	}

	if _, err := runCommand(cmd, "fail"); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("runCommand failure error mismatch: %v", err)
	}

	out, err = runCommandEnv([]string{"X_VAR=42"}, cmd, "printenv")
	if err != nil {
		t.Fatalf("runCommandEnv success unexpected error: %v", err)
	}
	if !strings.Contains(out, "X_VAR=42") {
		t.Fatalf("runCommandEnv env propagation failed: %q", out)
	}

	if _, err := runCommandEnv(nil, cmd, "fail"); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("runCommandEnv failure error mismatch: %v", err)
	}
}

func TestInstallLinuxPackagesDispatch(t *testing.T) {
	type tc struct {
		mgr       string
		wantParts []string
	}
	tests := []tc{
		{mgr: "apt-get", wantParts: []string{"update", "install -y --no-install-recommends pkg1 pkg2"}},
		{mgr: "dnf", wantParts: []string{"install -y pkg1 pkg2"}},
		{mgr: "yum", wantParts: []string{"install -y pkg1 pkg2"}},
		{mgr: "pacman", wantParts: []string{"-Sy --noconfirm --needed pkg1 pkg2"}},
		{mgr: "apk", wantParts: []string{"add --no-cache pkg1 pkg2"}},
		{mgr: "zypper", wantParts: []string{"--non-interactive install -y pkg1 pkg2"}},
	}

	for _, tt := range tests {
		t.Run(tt.mgr, func(t *testing.T) {
			logFile := filepath.Join(t.TempDir(), "log.txt")
			t.Setenv("FAKE_LOG_FILE", logFile)
			cmd := writeExecScript(t, `
echo "$*" >> "$FAKE_LOG_FILE"
`)

			if err := installLinuxPackages(tt.mgr, cmd, []string{"pkg1", "pkg2"}); err != nil {
				t.Fatalf("installLinuxPackages(%s) error: %v", tt.mgr, err)
			}

			lines := readLines(t, logFile)
			for _, want := range tt.wantParts {
				assertLineContains(t, lines, want)
			}
		})
	}

	if err := installLinuxPackages("unsupported", "/bin/true", []string{"x"}); err == nil {
		t.Fatalf("expected error for unsupported manager")
	}
}

func TestEnsureIptablesRule(t *testing.T) {
	check := []string{"-t", "mangle", "-C", "OUTPUT", "-j", "GOVPASS_OUTPUT"}
	add := []string{"-t", "mangle", "-I", "OUTPUT", "1", "-j", "GOVPASS_OUTPUT"}
	cmd := writeExecScript(t, `
if [ "${3:-}" = "-C" ]; then
  if [ "${CHECK_OK:-0}" = "1" ]; then
    exit 0
  fi
  echo "missing" >&2
  exit 1
fi
if [ "${3:-}" = "-I" ]; then
  if [ "${ADD_OK:-0}" = "1" ]; then
    exit 0
  fi
  echo "add failed" >&2
  exit 1
fi
exit 0
`)

	t.Setenv("CHECK_OK", "1")
	t.Setenv("ADD_OK", "0")
	if err := ensureIptablesRule(cmd, check, add); err != nil {
		t.Fatalf("check-success path should pass: %v", err)
	}

	t.Setenv("CHECK_OK", "0")
	t.Setenv("ADD_OK", "1")
	if err := ensureIptablesRule(cmd, check, add); err != nil {
		t.Fatalf("add-after-check-fail path should pass: %v", err)
	}

	t.Setenv("CHECK_OK", "0")
	t.Setenv("ADD_OK", "0")
	if err := ensureIptablesRule(cmd, check, add); err == nil {
		t.Fatalf("expected error when both check and add fail")
	}
}

func TestEnsureIptablesChain(t *testing.T) {
	cmd := writeExecScript(t, `
if [ "${3:-}" = "-N" ]; then
  case "${CHAIN_MODE:-ok}" in
    ok) exit 0 ;;
    exists)
      echo "Chain already exists." >&2
      exit 1
      ;;
    fail)
      echo "fatal" >&2
      exit 1
      ;;
  esac
fi
exit 0
`)

	t.Setenv("CHAIN_MODE", "ok")
	if err := ensureIptablesChain(cmd, "mangle", "GOVPASS_OUTPUT"); err != nil {
		t.Fatalf("ok mode unexpected error: %v", err)
	}

	t.Setenv("CHAIN_MODE", "exists")
	if err := ensureIptablesChain(cmd, "mangle", "GOVPASS_OUTPUT"); err != nil {
		t.Fatalf("exists mode should be treated as success: %v", err)
	}

	t.Setenv("CHAIN_MODE", "fail")
	if err := ensureIptablesChain(cmd, "mangle", "GOVPASS_OUTPUT"); err == nil {
		t.Fatalf("fail mode should return error")
	}
}

func TestInstallAndUninstallIptablesRules(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "iptables.log")
	stateFile := filepath.Join(t.TempDir(), "state.txt")
	t.Setenv("FAKE_LOG_FILE", logFile)
	t.Setenv("STATE_FILE", stateFile)

	cmd := writeExecScript(t, `
echo "$*" >> "$FAKE_LOG_FILE"
if [ "${3:-}" = "-D" ] && [ "${4:-}" = "OUTPUT" ]; then
  c=0
  if [ -f "$STATE_FILE" ]; then
    c=$(cat "$STATE_FILE")
  fi
  if [ "$c" -eq 0 ]; then
    echo 1 > "$STATE_FILE"
    exit 0
  fi
  echo "not found" >&2
  exit 1
fi
if [ "${3:-}" = "-C" ] && [ "${4:-}" = "OUTPUT" ]; then
  echo "missing jump" >&2
  exit 1
fi
exit 0
`)

	opts := ruleOptions{QueueNum: 100, Mark: 1, ExcludeLoopback: true}
	if err := installIptablesRules(cmd, opts); err != nil {
		t.Fatalf("installIptablesRules error: %v", err)
	}
	if err := uninstallIptablesRules(cmd, opts); err != nil {
		t.Fatalf("uninstallIptablesRules error: %v", err)
	}

	lines := readLines(t, logFile)
	assertLineContains(t, lines, "-t mangle -N GOVPASS_OUTPUT")
	assertLineContains(t, lines, "-t mangle -F GOVPASS_OUTPUT")
	assertLineContains(t, lines, "-t mangle -I OUTPUT 1 -j GOVPASS_OUTPUT")
	assertLineContains(t, lines, "-t mangle -A GOVPASS_OUTPUT -m mark --mark 1/1 -j RETURN")
	assertLineContains(t, lines, "-t mangle -A GOVPASS_OUTPUT -o lo -j RETURN")
	assertLineContains(t, lines, "--queue-num 100 --queue-bypass")
	assertLineContains(t, lines, "-t mangle -X GOVPASS_OUTPUT")
}

func TestInstallAndDeleteNftRules(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "nft.log")
	t.Setenv("FAKE_LOG_FILE", logFile)
	t.Setenv("NFT_FAIL_LIST_TABLE", "1")
	t.Setenv("NFT_FAIL_LIST_CHAIN", "1")
	t.Setenv("NFT_LIST_CHAIN_OUTPUT", "tcp dport 443 queue num 100 bypass comment \"gov-pass\" # handle 11\nmeta mark & 1 == 1 return comment \"gov-pass\" # handle 15\n")

	cmd := writeExecScript(t, `
echo "$*" >> "$FAKE_LOG_FILE"
if [ "${1:-}" = "list" ] && [ "${2:-}" = "table" ] && [ "${3:-}" = "inet" ]; then
  if [ "${NFT_FAIL_LIST_TABLE:-0}" = "1" ]; then
    echo "No such file or directory" >&2
    exit 1
  fi
fi
if [ "${1:-}" = "list" ] && [ "${2:-}" = "chain" ] && [ "${3:-}" = "inet" ]; then
  if [ "${NFT_FAIL_LIST_CHAIN:-0}" = "1" ]; then
    echo "No such file or directory" >&2
    exit 1
  fi
fi
if [ "${1:-}" = "-a" ] && [ "${2:-}" = "list" ] && [ "${3:-}" = "chain" ]; then
  printf "%b" "${NFT_LIST_CHAIN_OUTPUT:-}"
  exit 0
fi
if [ "${1:-}" = "delete" ] && [ "${2:-}" = "rule" ] && [ "${8:-}" = "11" ] && [ "${NFT_FAIL_DELETE_11:-0}" = "1" ]; then
  echo "No such file or directory" >&2
  exit 1
fi
exit 0
`)

	opts := ruleOptions{QueueNum: 100, Mark: 1, ExcludeLoopback: true}
	if err := installNftRules(cmd, opts); err != nil {
		t.Fatalf("installNftRules error: %v", err)
	}

	// After installation, simulate a clean environment where list calls succeed.
	t.Setenv("NFT_FAIL_LIST_TABLE", "0")
	t.Setenv("NFT_FAIL_LIST_CHAIN", "0")
	if err := deleteTaggedNftRules(cmd, "gov_pass", "output", "gov-pass"); err != nil {
		t.Fatalf("deleteTaggedNftRules error: %v", err)
	}
	if err := uninstallNftRules(cmd); err != nil {
		t.Fatalf("uninstallNftRules error: %v", err)
	}

	lines := readLines(t, logFile)
	assertLineContains(t, lines, "add table inet gov_pass")
	assertLineContains(t, lines, "add chain inet gov_pass output")
	assertLineContains(t, lines, "meta mark & 1 == 1 return comment gov-pass")
	assertLineContains(t, lines, "oifname lo return comment gov-pass")
	assertLineContains(t, lines, "meta nfproto ipv4 tcp dport 443 queue num 100 bypass comment gov-pass")
	assertLineContains(t, lines, "delete rule inet gov_pass output handle 11")
	assertLineContains(t, lines, "delete rule inet gov_pass output handle 15")
}
