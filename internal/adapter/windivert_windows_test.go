//go:build windows

package adapter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigureWinDivertDLLRejectsNonRegularPath(t *testing.T) {
	err := ConfigureWinDivertDLL(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("expected regular file error, got %v", err)
	}
}

func TestConfigureWinDivertDLLRejectsUnexpectedBasename(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-windivert.dll")
	if err := os.WriteFile(path, []byte("not a dll"), 0o644); err != nil {
		t.Fatalf("write temp dll: %v", err)
	}

	err := ConfigureWinDivertDLL(path)
	if err == nil || !strings.Contains(err.Error(), "must end with WinDivert.dll") {
		t.Fatalf("expected basename error, got %v", err)
	}
}

func TestConfigureWinDivertDLLRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.dll")
	if err := os.WriteFile(target, []byte("not a dll"), 0o644); err != nil {
		t.Fatalf("write temp dll: %v", err)
	}
	link := filepath.Join(dir, "WinDivert.dll")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	err := ConfigureWinDivertDLL(link)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink error, got %v", err)
	}
}
