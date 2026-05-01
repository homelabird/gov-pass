//go:build linux

package main

import (
	"fmt"
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

func assertLineCountContains(t *testing.T, lines []string, want string, wantCount int) {
	t.Helper()
	count := 0
	for _, line := range lines {
		if strings.Contains(line, want) {
			count++
		}
	}
	if count != wantCount {
		t.Fatalf("expected %d lines containing %q, got %d: %v", wantCount, want, count, lines)
	}
}

func TestRunCommandAndEnv(t *testing.T) {
	cmd := writeExecScript(t, `
if [ "${1:-}" = "fail" ]; then
  echo "boom" >&2
  exit 7
fi
if [ "${1:-}" = "printenv" ]; then
  echo "X_VAR=${X_VAR:-}"
  echo "LEAK_VAR=${LEAK_VAR:-}"
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

	t.Setenv("LEAK_VAR", "secret")
	out, err = runCommand(cmd, "printenv")
	if err != nil {
		t.Fatalf("runCommand sanitized env unexpected error: %v", err)
	}
	if strings.Contains(out, "LEAK_VAR=secret") {
		t.Fatalf("runCommand leaked caller environment: %q", out)
	}

	out, err = runCommandEnv([]string{"X_VAR=42"}, cmd, "printenv")
	if err != nil {
		t.Fatalf("runCommandEnv success unexpected error: %v", err)
	}
	if !strings.Contains(out, "X_VAR=42") {
		t.Fatalf("runCommandEnv env propagation failed: %q", out)
	}
	out, err = runCommandEnv(nil, cmd, "printenv")
	if err != nil {
		t.Fatalf("runCommandEnv sanitized env unexpected error: %v", err)
	}
	if strings.Contains(out, "LEAK_VAR=secret") {
		t.Fatalf("runCommandEnv leaked caller environment: %q", out)
	}

	if _, err := runCommandEnv(nil, cmd, "fail"); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("runCommandEnv failure error mismatch: %v", err)
	}
}

func TestLookPath_RejectsPoisonedPATHEntry(t *testing.T) {
	dir := t.TempDir()
	cmd := filepath.Join(dir, "nft")
	if err := os.WriteFile(cmd, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake command: %v", err)
	}
	t.Setenv("PATH", dir)
	if got, ok := lookPath("nft"); ok {
		if filepath.Clean(got) == filepath.Clean(cmd) {
			t.Fatalf("expected poisoned PATH entry to be rejected, got %q", got)
		}
	}
}

func TestLookPath_RejectsSymlinkOutsideTrustedDir(t *testing.T) {
	trustedDir := t.TempDir()
	outsideDir := t.TempDir()
	target := filepath.Join(outsideDir, "nft")
	if err := os.WriteFile(target, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write target command: %v", err)
	}
	link := filepath.Join(trustedDir, "nft")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	origDirs := trustedLinuxCommandDirs
	trustedLinuxCommandDirs = []string{trustedDir}
	defer func() {
		trustedLinuxCommandDirs = origDirs
	}()

	if got, ok := lookPath("nft"); ok {
		t.Fatalf("expected symlink target outside trusted dir to be rejected, got %q", got)
	}
}

func TestLookPath_AllowsSymlinkedTrustedDir(t *testing.T) {
	actualDir := t.TempDir()
	trustedParent := t.TempDir()
	trustedDir := filepath.Join(trustedParent, "trusted-bin")
	target := filepath.Join(actualDir, "nft")
	if err := os.WriteFile(target, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write target command: %v", err)
	}
	if err := os.Symlink(actualDir, trustedDir); err != nil {
		t.Fatalf("create trusted dir symlink: %v", err)
	}

	origDirs := trustedLinuxCommandDirs
	trustedLinuxCommandDirs = []string{trustedDir}
	defer func() {
		trustedLinuxCommandDirs = origDirs
	}()

	got, ok := lookPath("nft")
	if !ok {
		t.Fatal("expected trusted command lookup to succeed")
	}
	if filepath.Clean(got) != filepath.Clean(target) {
		t.Fatalf("lookPath returned %q, want %q", got, target)
	}
	if !isTrustedLinuxCommandPath(target) {
		t.Fatalf("isTrustedLinuxCommandPath(%q) = false, want true", target)
	}
}

func TestLookPath_RejectsRelativePathName(t *testing.T) {
	parent := t.TempDir()
	sbin := filepath.Join(parent, "sbin")
	bin := filepath.Join(parent, "bin")
	if err := os.MkdirAll(sbin, 0o755); err != nil {
		t.Fatalf("mkdir sbin: %v", err)
	}
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	cmd := filepath.Join(bin, "nft")
	if err := os.WriteFile(cmd, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write command: %v", err)
	}

	origDirs := trustedLinuxCommandDirs
	trustedLinuxCommandDirs = []string{sbin, bin}
	defer func() {
		trustedLinuxCommandDirs = origDirs
	}()

	if got, ok := lookPath(filepath.Join("..", "bin", "nft")); ok {
		t.Fatalf("expected relative path command name to be rejected, got %q", got)
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
			cmd := writeExecScript(t, fmt.Sprintf(`echo "$*" >> %q`, logFile))

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

func TestEnsureLinuxExternalToolsUsesInjectedPackageManagerDetector(t *testing.T) {
	toolsDir := t.TempDir()
	managerDir := t.TempDir()
	nftPath := filepath.Join(toolsDir, "nft")
	managerPath := filepath.Join(managerDir, "apt-get")
	managerScript := fmt.Sprintf(`#!/bin/sh
set -eu
nft_path=%q
case "${1:-}" in
  update)
    exit 0
    ;;
  install)
    printf '#!/bin/sh\nexit 0\n' > "$nft_path"
    /bin/chmod 755 "$nft_path"
    exit 0
    ;;
esac
exit 1
`, nftPath)
	if err := os.WriteFile(managerPath, []byte(managerScript), 0o755); err != nil {
		t.Fatalf("write package manager stub: %v", err)
	}

	origLookPath := linuxLookPath
	origDetect := linuxDetectPackageManager
	origDirs := trustedLinuxCommandDirs
	t.Cleanup(func() {
		linuxLookPath = origLookPath
		linuxDetectPackageManager = origDetect
		trustedLinuxCommandDirs = origDirs
	})

	trustedLinuxCommandDirs = []string{toolsDir}
	linuxLookPath = lookPath
	linuxDetectPackageManager = func() (string, string, bool) {
		return "apt-get", managerPath, true
	}

	if _, ok := linuxLookPath("nft"); ok {
		t.Fatal("test setup expected nft to be missing before auto-install")
	}
	if err := ensureLinuxExternalTools(true, linuxToolNeeds{AutoRules: true}); err != nil {
		t.Fatalf("ensureLinuxExternalTools failed: %v", err)
	}
	if got, ok := linuxLookPath("nft"); !ok || filepath.Clean(got) != filepath.Clean(nftPath) {
		t.Fatalf("nft lookup after auto-install = %q,%v; want %q,true", got, ok, nftPath)
	}
}

func TestEnsureLinuxExternalToolsInstallsPackagesInStableOrder(t *testing.T) {
	toolsDir := t.TempDir()
	managerDir := t.TempDir()
	logFile := filepath.Join(t.TempDir(), "apt.log")
	managerPath := filepath.Join(managerDir, "apt-get")
	managerScript := fmt.Sprintf(`#!/bin/sh
set -eu
log_file=%q
tools_dir=%q
echo "$*" >> "$log_file"
case "${1:-}" in
  update)
    exit 0
    ;;
  install)
    for tool in nft ethtool ip; do
      printf '#!/bin/sh\nexit 0\n' > "$tools_dir/$tool"
      /bin/chmod 755 "$tools_dir/$tool"
    done
    exit 0
    ;;
esac
exit 1
`, logFile, toolsDir)
	if err := os.WriteFile(managerPath, []byte(managerScript), 0o755); err != nil {
		t.Fatalf("write package manager stub: %v", err)
	}

	origLookPath := linuxLookPath
	origDetect := linuxDetectPackageManager
	origDirs := trustedLinuxCommandDirs
	t.Cleanup(func() {
		linuxLookPath = origLookPath
		linuxDetectPackageManager = origDetect
		trustedLinuxCommandDirs = origDirs
	})

	trustedLinuxCommandDirs = []string{toolsDir}
	linuxLookPath = lookPath
	linuxDetectPackageManager = func() (string, string, bool) {
		return "apt-get", managerPath, true
	}

	err := ensureLinuxExternalTools(true, linuxToolNeeds{
		AutoRules:   true,
		AutoOffload: true,
		NeedIP:      true,
	})
	if err != nil {
		t.Fatalf("ensureLinuxExternalTools failed: %v", err)
	}

	lines := readLines(t, logFile)
	assertLineContains(t, lines, "install -y --no-install-recommends ethtool iproute2 iptables nftables")
}

func TestEnsureIptablesRule(t *testing.T) {
	check := []string{"-t", "mangle", "-C", "OUTPUT", "-j", "GOVPASS_OUTPUT"}
	add := []string{"-t", "mangle", "-I", "OUTPUT", "1", "-j", "GOVPASS_OUTPUT"}
	modePath := filepath.Join(t.TempDir(), "mode")
	setMode := func(checkOK string, addOK string) {
		t.Helper()
		if err := os.WriteFile(modePath, []byte(checkOK+"\n"+addOK+"\n"), 0o644); err != nil {
			t.Fatalf("write mode: %v", err)
		}
	}
	cmd := writeExecScript(t, fmt.Sprintf(`
check_ok="$(sed -n '1p' %q)"
add_ok="$(sed -n '2p' %q)"
if [ "${3:-}" = "-C" ]; then
  if [ "$check_ok" = "1" ]; then
    exit 0
  fi
  echo "missing" >&2
  exit 1
fi
if [ "${3:-}" = "-I" ]; then
  if [ "$add_ok" = "1" ]; then
    exit 0
  fi
  echo "add failed" >&2
  exit 1
fi
exit 0
`, modePath, modePath))

	setMode("1", "0")
	if err := ensureIptablesRule(cmd, check, add); err != nil {
		t.Fatalf("check-success path should pass: %v", err)
	}

	setMode("0", "1")
	if err := ensureIptablesRule(cmd, check, add); err != nil {
		t.Fatalf("add-after-check-fail path should pass: %v", err)
	}

	setMode("0", "0")
	if err := ensureIptablesRule(cmd, check, add); err == nil {
		t.Fatalf("expected error when both check and add fail")
	}
}

func TestEnsureIptablesChain(t *testing.T) {
	modePath := filepath.Join(t.TempDir(), "mode")
	setMode := func(mode string) {
		t.Helper()
		if err := os.WriteFile(modePath, []byte(mode+"\n"), 0o644); err != nil {
			t.Fatalf("write mode: %v", err)
		}
	}
	cmd := writeExecScript(t, fmt.Sprintf(`
mode="$(cat %q 2>/dev/null || echo ok)"
if [ "${3:-}" = "-N" ]; then
  case "$mode" in
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
`, modePath))

	setMode("ok")
	if err := ensureIptablesChain(cmd, "mangle", "GOVPASS_OUTPUT"); err != nil {
		t.Fatalf("ok mode unexpected error: %v", err)
	}

	setMode("exists")
	if err := ensureIptablesChain(cmd, "mangle", "GOVPASS_OUTPUT"); err != nil {
		t.Fatalf("exists mode should be treated as success: %v", err)
	}

	setMode("fail")
	if err := ensureIptablesChain(cmd, "mangle", "GOVPASS_OUTPUT"); err == nil {
		t.Fatalf("fail mode should return error")
	}
}

func TestInstallAndUninstallIptablesRules(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "iptables.log")
	stateDir := filepath.Join(t.TempDir(), "state")

	cmd := writeExecScript(t, fmt.Sprintf(`
log_file=%q
state_dir=%q
echo "$*" >> "$log_file"
if [ "${3:-}" = "-D" ] && [ "${4:-}" = "OUTPUT" ]; then
  mkdir -p "$state_dir"
  state="$state_dir/${6:-unknown}"
  if [ ! -f "$state" ]; then
    echo 1 > "$state"
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
`, logFile, stateDir))

	opts := ruleOptions{QueueNum: 100, Mark: 1, ExcludeLoopback: true}
	if err := installIptablesRules(cmd, cmd, opts); err != nil {
		t.Fatalf("installIptablesRules error: %v", err)
	}
	if err := uninstallIptablesRules(cmd, cmd, opts); err != nil {
		t.Fatalf("uninstallIptablesRules error: %v", err)
	}

	lines := readLines(t, logFile)
	assertLineContains(t, lines, "-t mangle -N GOVPASS_OUTPUT")
	assertLineContains(t, lines, "-t mangle -N GOVPASS_OUTPUT6")
	assertLineContains(t, lines, "-t mangle -F GOVPASS_OUTPUT")
	assertLineContains(t, lines, "-t mangle -F GOVPASS_OUTPUT6")
	assertLineContains(t, lines, "-t mangle -I OUTPUT 1 -j GOVPASS_OUTPUT")
	assertLineContains(t, lines, "-t mangle -I OUTPUT 1 -j GOVPASS_OUTPUT6")
	assertLineContains(t, lines, "-t mangle -A GOVPASS_OUTPUT -m mark --mark 1/1 -j RETURN")
	assertLineContains(t, lines, "-t mangle -A GOVPASS_OUTPUT6 -m mark --mark 1/1 -j RETURN")
	assertLineContains(t, lines, "-t mangle -A GOVPASS_OUTPUT -o lo -j RETURN")
	assertLineContains(t, lines, "-t mangle -A GOVPASS_OUTPUT6 -o lo -j RETURN")
	assertLineCountContains(t, lines, "--queue-num 100 --queue-bypass", 2)
	assertLineContains(t, lines, "-t mangle -X GOVPASS_OUTPUT")
	assertLineContains(t, lines, "-t mangle -X GOVPASS_OUTPUT6")
}

func TestInstallAndDeleteNftRules(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "nft.log")
	stateDir := t.TempDir()
	failListTable := filepath.Join(stateDir, "fail-list-table")
	failListChain := filepath.Join(stateDir, "fail-list-chain")
	listChainOutput := filepath.Join(stateDir, "list-chain-output")
	if err := os.WriteFile(failListTable, []byte("1\n"), 0o644); err != nil {
		t.Fatalf("write nft state: %v", err)
	}
	if err := os.WriteFile(failListChain, []byte("1\n"), 0o644); err != nil {
		t.Fatalf("write nft state: %v", err)
	}
	if err := os.WriteFile(listChainOutput, []byte("meta nfproto ipv4 tcp dport 443 queue num 100 bypass comment \"gov-pass\" # handle 11\nmeta nfproto ipv6 tcp dport 443 queue num 100 bypass comment \"gov-pass\" # handle 13\nmeta mark & 1 == 1 return comment \"gov-pass\" # handle 15\noifname \"lo\" return comment \"gov-pass\" # handle 17\n"), 0o644); err != nil {
		t.Fatalf("write nft output: %v", err)
	}

	cmd := writeExecScript(t, fmt.Sprintf(`
log_file=%q
fail_list_table="$(cat %q 2>/dev/null || echo 0)"
fail_list_chain="$(cat %q 2>/dev/null || echo 0)"
list_chain_output=%q
echo "$*" >> "$log_file"
if [ "${1:-}" = "list" ] && [ "${2:-}" = "table" ] && [ "${3:-}" = "inet" ]; then
  if [ "$fail_list_table" = "1" ]; then
    echo "No such file or directory" >&2
    exit 1
  fi
fi
if [ "${1:-}" = "list" ] && [ "${2:-}" = "chain" ] && [ "${3:-}" = "inet" ]; then
  if [ "$fail_list_chain" = "1" ]; then
    echo "No such file or directory" >&2
    exit 1
  fi
fi
if [ "${1:-}" = "-a" ] && [ "${2:-}" = "list" ] && [ "${3:-}" = "chain" ]; then
  cat "$list_chain_output"
  exit 0
fi
exit 0
`, logFile, failListTable, failListChain, listChainOutput))

	opts := ruleOptions{QueueNum: 100, Mark: 1, ExcludeLoopback: true}
	if err := installNftRules(cmd, opts); err != nil {
		t.Fatalf("installNftRules error: %v", err)
	}

	// After installation, simulate a clean environment where list calls succeed.
	if err := os.WriteFile(failListTable, []byte("0\n"), 0o644); err != nil {
		t.Fatalf("write nft state: %v", err)
	}
	if err := os.WriteFile(failListChain, []byte("0\n"), 0o644); err != nil {
		t.Fatalf("write nft state: %v", err)
	}
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
	assertLineContains(t, lines, "meta nfproto ipv6 tcp dport 443 queue num 100 bypass comment gov-pass")
	assertLineContains(t, lines, "delete rule inet gov_pass output handle 11")
	assertLineContains(t, lines, "delete rule inet gov_pass output handle 13")
	assertLineContains(t, lines, "delete rule inet gov_pass output handle 15")
	assertLineContains(t, lines, "delete rule inet gov_pass output handle 17")
}
