//go:build linux

package main

import (
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
	for _, action := range []string{"start", "stop", "restart", "toggle", "status"} {
		err := runAction("nonexistent-test-service-gov-pass", action)
		if err != nil && err.Error() == "unknown action: "+action {
			t.Errorf("action %q should be recognized", action)
		}
	}
}
