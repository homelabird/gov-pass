//go:build freebsd

package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunFreeBSDPreflight_JSONReportsPFState(t *testing.T) {
	restore := stubFreeBSDPreflightDeps(t, freeBSDPreflightStubs{
		lookPath: func(name string) (string, error) {
			return filepath.Join("/usr/sbin", name), nil
		},
		stat: func(path string) (os.FileInfo, error) {
			return fakeFreeBSDFileInfo{name: filepath.Base(path)}, nil
		},
		cmdOutput: func(name string, args ...string) ([]byte, error) {
			switch strings.Join(args, " ") {
			case "-s info":
				return []byte("Status: Enabled for 0 days 00:01:00\n"), nil
			case "-a gov-pass -s rules":
				return []byte("pass out quick inet proto tcp to any port = https divert-to 127.0.0.1 port 10000\n"), nil
			default:
				return nil, errors.New("unexpected command")
			}
		},
	})
	defer restore()

	out, err := captureFreeBSDStdout(t, func() error {
		return runFreeBSDPreflight(true)
	})

	report := decodeFreeBSDPreflightReport(t, out)
	if os.Geteuid() == 0 {
		if err != nil || !report.OK {
			t.Fatalf("root preflight should succeed: err=%v report=%+v", err, report)
		}
	} else {
		if err == nil || report.OK {
			t.Fatalf("non-root preflight should fail only root check: err=%v report=%+v", err, report)
		}
	}
	assertFreeBSDCheck(t, report, "pfctl", true)
	assertFreeBSDCheck(t, report, "service", true)
	assertFreeBSDCheck(t, report, "sysrc", true)
	assertFreeBSDCheck(t, report, "pf_conf", true)
	assertFreeBSDCheck(t, report, "pf_anchor_source", true)
	assertFreeBSDCheck(t, report, "pf_anchor_installed", true)
	assertFreeBSDCheck(t, report, "pf_status", true)
	assertFreeBSDCheck(t, report, "pf_anchor_rules", true)
}

func TestRunFreeBSDPreflight_JSONReportsMissingAnchorRules(t *testing.T) {
	restore := stubFreeBSDPreflightDeps(t, freeBSDPreflightStubs{
		lookPath: func(name string) (string, error) {
			return filepath.Join("/usr/sbin", name), nil
		},
		stat: func(path string) (os.FileInfo, error) {
			return fakeFreeBSDFileInfo{name: filepath.Base(path)}, nil
		},
		cmdOutput: func(name string, args ...string) ([]byte, error) {
			switch strings.Join(args, " ") {
			case "-s info":
				return []byte("Status: Enabled\n"), nil
			case "-a gov-pass -s rules":
				return []byte("\n"), nil
			default:
				return nil, errors.New("unexpected command")
			}
		},
	})
	defer restore()

	out, err := captureFreeBSDStdout(t, func() error {
		return runFreeBSDPreflight(true)
	})
	if err == nil {
		t.Fatal("expected preflight failure when live anchor has no rules")
	}

	report := decodeFreeBSDPreflightReport(t, out)
	check := findFreeBSDCheck(report, "pf_anchor_rules")
	if check == nil {
		t.Fatalf("pf_anchor_rules check missing: %+v", report.Checks)
	}
	if check.OK || !strings.Contains(check.Detail, "live anchor has no rules") {
		t.Fatalf("unexpected pf_anchor_rules check: %+v", *check)
	}
}

type freeBSDPreflightStubs struct {
	lookPath  func(string) (string, error)
	stat      func(string) (os.FileInfo, error)
	cmdOutput func(string, ...string) ([]byte, error)
}

func stubFreeBSDPreflightDeps(t *testing.T, stubs freeBSDPreflightStubs) func() {
	t.Helper()
	origLookPath := freebsdLookPath
	origStat := freebsdStat
	origCmdOutput := freebsdCmdOutput
	if stubs.lookPath != nil {
		freebsdLookPath = stubs.lookPath
	}
	if stubs.stat != nil {
		freebsdStat = stubs.stat
	}
	if stubs.cmdOutput != nil {
		freebsdCmdOutput = stubs.cmdOutput
	}
	return func() {
		freebsdLookPath = origLookPath
		freebsdStat = origStat
		freebsdCmdOutput = origCmdOutput
	}
}

func captureFreeBSDStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	runErr := fn()
	_ = w.Close()
	os.Stdout = orig
	out, _ := io.ReadAll(r)
	_ = r.Close()
	return string(out), runErr
}

func decodeFreeBSDPreflightReport(t *testing.T, out string) freebsdPreflightReport {
	t.Helper()
	var report freebsdPreflightReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("json parse failed: %v, out=%q", err, out)
	}
	return report
}

func assertFreeBSDCheck(t *testing.T, report freebsdPreflightReport, name string, ok bool) {
	t.Helper()
	check := findFreeBSDCheck(report, name)
	if check == nil {
		t.Fatalf("%s check missing: %+v", name, report.Checks)
	}
	if check.OK != ok {
		t.Fatalf("%s check OK=%t, want %t: %+v", name, check.OK, ok, *check)
	}
}

func findFreeBSDCheck(report freebsdPreflightReport, name string) *freebsdPreflightCheck {
	for i := range report.Checks {
		if report.Checks[i].Name == name {
			return &report.Checks[i]
		}
	}
	return nil
}

type fakeFreeBSDFileInfo struct {
	name string
}

func (f fakeFreeBSDFileInfo) Name() string       { return f.name }
func (f fakeFreeBSDFileInfo) Size() int64        { return 1 }
func (f fakeFreeBSDFileInfo) Mode() os.FileMode  { return 0o644 }
func (f fakeFreeBSDFileInfo) ModTime() time.Time { return time.Unix(0, 0) }
func (f fakeFreeBSDFileInfo) IsDir() bool        { return false }
func (f fakeFreeBSDFileInfo) Sys() any           { return nil }

var _ os.FileInfo = fakeFreeBSDFileInfo{}
