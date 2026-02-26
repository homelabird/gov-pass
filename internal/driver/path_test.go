package driver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrependPathAddsAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", "/usr/bin:/bin")

	if err := PrependPath(dir); err != nil {
		t.Fatalf("first prepend failed: %v", err)
	}
	path1 := os.Getenv("PATH")
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("abs failed: %v", err)
	}
	if !strings.HasPrefix(path1, abs+string(os.PathListSeparator)) {
		t.Fatalf("PATH prefix mismatch: %q", path1)
	}

	if err := PrependPath(dir); err != nil {
		t.Fatalf("second prepend failed: %v", err)
	}
	path2 := os.Getenv("PATH")
	if path1 != path2 {
		t.Fatalf("expected idempotent prepend, got %q then %q", path1, path2)
	}
}

func TestPrependPathEmptyNoop(t *testing.T) {
	t.Setenv("PATH", "/usr/bin")
	before := os.Getenv("PATH")
	if err := PrependPath(""); err != nil {
		t.Fatalf("empty path should not fail: %v", err)
	}
	if after := os.Getenv("PATH"); after != before {
		t.Fatalf("PATH changed on empty prepend: before=%q after=%q", before, after)
	}
}

func TestPrependPathMissingDir(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	if err := PrependPath(missing); err == nil {
		t.Fatalf("expected error for missing path")
	}
}
