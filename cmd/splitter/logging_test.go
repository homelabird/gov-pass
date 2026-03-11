package main

import (
	"errors"
	"testing"
)

func TestFormatLogEvent(t *testing.T) {
	got := formatLogEvent(
		"info",
		"engine_started",
		"engine started (workers=4)",
		"workers", 4,
		"split_mode", "tls-hello",
		"note", "two words",
	)
	want := `level=info event=engine_started msg="engine started (workers=4)" workers=4 split_mode=tls-hello note="two words"`
	if got != want {
		t.Fatalf("unexpected log event: got %q want %q", got, want)
	}
}

func TestFormatLogEvent_SanitizesKeysAndErrors(t *testing.T) {
	got := formatLogEvent(
		"error",
		"reload_failed",
		"reload failed",
		"windivert dir", `C:\Program Files\gov-pass`,
		"err", errors.New("permission denied"),
	)
	want := `level=error event=reload_failed msg="reload failed" windivert_dir="C:\\Program Files\\gov-pass" err="permission denied"`
	if got != want {
		t.Fatalf("unexpected sanitized log event: got %q want %q", got, want)
	}
}
