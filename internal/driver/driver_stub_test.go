//go:build !windows

package driver

import (
	"context"
	"errors"
	"testing"
)

func TestEnsureNotSupportedOnNonWindows(t *testing.T) {
	cleanup, err := Ensure(context.Background(), Config{})
	if cleanup != nil {
		t.Fatalf("cleanup must be nil on unsupported platform")
	}
	if !errors.Is(err, ErrNotSupported) {
		t.Fatalf("unexpected error: %v", err)
	}
}
