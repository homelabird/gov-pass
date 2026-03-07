//go:build linux

package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

func TestRunAction_UnknownAction(t *testing.T) {
	err := runAction("gov-pass", "unknown-action")
	if err == nil {
		t.Fatal("expected error for unknown action")
	}
	if got := err.Error(); got != "unknown action: unknown-action" {
		t.Fatalf("unexpected error message: %s", got)
	}
}

func TestRunAction_ValidActions(t *testing.T) {
	// These actions are recognized by runAction but will fail because systemd
	// is not managing our test service. We just verify that they don't return
	// the "unknown action" error.
	for _, action := range []string{"start", "stop", "restart", "reload", "enable", "disable", "toggle", "status"} {
		err := runAction("nonexistent-test-service-gov-pass", action)
		if err != nil && err.Error() == "unknown action: "+action {
			t.Errorf("action %q should be recognized", action)
		}
	}
}

func TestRunAction_InvalidServiceName(t *testing.T) {
	if err := runAction("gov pass", "status"); err == nil {
		t.Fatal("expected invalid service name error")
	}
}

func TestRenderPlainTUI_UsesContextualLabels(t *testing.T) {
	origActive := linuxIsServiceActive
	origEnabled := linuxIsServiceEnabled
	origStatusText := linuxServiceStatusText
	t.Cleanup(func() {
		linuxIsServiceActive = origActive
		linuxIsServiceEnabled = origEnabled
		linuxServiceStatusText = origStatusText
	})

	linuxIsServiceActive = func(serviceName string) (bool, error) { return true, nil }
	linuxIsServiceEnabled = func(serviceName string) (bool, error) { return false, nil }
	linuxServiceStatusText = func(serviceName string) (string, error) { return "active", nil }

	out := captureStdout(t, func() {
		renderPlainTUI("gov-pass", "Ready.")
	})

	for _, want := range []string{
		"1. Stop service (gov-pass)",
		"3. Enable boot start (gov-pass)",
		"4. Show detailed status",
		"5. Show recent logs",
		"r. Refresh",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected output to contain %q\nfull output:\n%s", want, out)
		}
	}
}

func TestExecuteMenuChoice_StatusDetail(t *testing.T) {
	origDetail := linuxServiceStatusView
	t.Cleanup(func() {
		linuxServiceStatusView = origDetail
	})

	linuxServiceStatusView = func(serviceName string) (string, error) {
		if serviceName != "gov-pass" {
			t.Fatalf("unexpected service name: %s", serviceName)
		}
		return "Loaded: loaded (/etc/systemd/system/gov-pass.service)", nil
	}

	msg, err := executeMenuChoice("gov-pass", "4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(msg, "Loaded: loaded") {
		t.Fatalf("unexpected detail text: %s", msg)
	}
}

func TestExecuteMenuChoice_RecentLogs(t *testing.T) {
	origLogs := linuxServiceRecentLogs
	t.Cleanup(func() {
		linuxServiceRecentLogs = origLogs
	})

	linuxServiceRecentLogs = func(serviceName string, lines int) (string, error) {
		if serviceName != "gov-pass" {
			t.Fatalf("unexpected service name: %s", serviceName)
		}
		if lines != 40 {
			t.Fatalf("unexpected line count: %d", lines)
		}
		return "Mar 07 19:00:00 gov-pass started", nil
	}

	msg, err := executeMenuChoice("gov-pass", "5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(msg, "gov-pass started") {
		t.Fatalf("unexpected log text: %s", msg)
	}
}

func TestExecuteMenuChoice_RefreshAlias(t *testing.T) {
	msg, err := executeMenuChoice("gov-pass", "refresh")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg != "" {
		t.Fatalf("expected empty refresh message, got %q", msg)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}

	os.Stdout = w
	defer func() {
		os.Stdout = origStdout
	}()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("close reader: %v", err)
	}
	return string(out)
}
