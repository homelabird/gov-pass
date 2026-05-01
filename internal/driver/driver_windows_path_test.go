//go:build windows

package driver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeServicePath_QuotedPathWithArgs(t *testing.T) {
	raw := `"C:\Program Files\gov-pass.sys dir\WinDivert64.sys" /flag`
	got := normalizeServicePath(raw)
	want := `C:\Program Files\gov-pass.sys dir\WinDivert64.sys`
	if got != want {
		t.Fatalf("normalizeServicePath(%q) = %q, want %q", raw, got, want)
	}
}

func TestNormalizeServicePath_UnquotedPathWithArgs(t *testing.T) {
	raw := `C:\Program Files\gov-pass.sys dir\WinDivert64.sys /flag`
	got := normalizeServicePath(raw)
	want := `C:\Program Files\gov-pass.sys dir\WinDivert64.sys`
	if got != want {
		t.Fatalf("normalizeServicePath(%q) = %q, want %q", raw, got, want)
	}
}

func TestServiceTakeoverRequired(t *testing.T) {
	report := Report{
		ServiceExists:                true,
		ServiceBinPath:               `C:\Other\WinDivert64.sys`,
		ServiceBinPathExists:         true,
		ServiceBinPathMatchesDesired: false,
	}
	if !serviceTakeoverRequired(report) {
		t.Fatal("expected takeover requirement for healthy mismatched service path")
	}
	report.ServiceBinPathExists = false
	if serviceTakeoverRequired(report) {
		t.Fatal("missing service path should be repairable without takeover")
	}
}

func TestResolveDirRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	_, err := resolveDir(link)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink directory rejection, got %v", err)
	}
}
