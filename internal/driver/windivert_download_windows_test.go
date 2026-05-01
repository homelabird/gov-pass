//go:build windows

package driver

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestVerifyPinnedWinDivertX64FilesRejectsMissingDir(t *testing.T) {
	err := VerifyPinnedWinDivertX64Files("")
	if err == nil || !strings.Contains(err.Error(), "dir is empty") {
		t.Fatalf("expected empty dir error, got %v", err)
	}
}

func TestVerifyPinnedWinDivertX64FilesRejectsHashMismatch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "WinDivert.dll"), []byte("bad dll"), 0o644); err != nil {
		t.Fatalf("write dll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "WinDivert64.sys"), []byte("bad sys"), 0o644); err != nil {
		t.Fatalf("write sys: %v", err)
	}

	err := VerifyPinnedWinDivertX64Files(dir)
	if err == nil || !strings.Contains(err.Error(), "SHA256 mismatch") {
		t.Fatalf("expected hash mismatch, got %v", err)
	}
}

func TestHasWinDivertFilesRejectsSymlinkedFiles(t *testing.T) {
	dir := t.TempDir()
	dllTarget := filepath.Join(dir, "target.dll")
	sysTarget := filepath.Join(dir, "target.sys")
	if err := os.WriteFile(dllTarget, []byte("dll"), 0o644); err != nil {
		t.Fatalf("write dll target: %v", err)
	}
	if err := os.WriteFile(sysTarget, []byte("sys"), 0o644); err != nil {
		t.Fatalf("write sys target: %v", err)
	}
	if err := os.Symlink(dllTarget, filepath.Join(dir, "WinDivert.dll")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := os.Symlink(sysTarget, filepath.Join(dir, "WinDivert64.sys")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	if HasWinDivertFiles(dir, "") {
		t.Fatal("expected symlinked WinDivert files to be rejected")
	}
}

func TestHasWinDivertFilesRejectsSymlinkedDirectory(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "WinDivert.dll"), []byte("dll"), 0o644); err != nil {
		t.Fatalf("write dll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "WinDivert64.sys"), []byte("sys"), 0o644); err != nil {
		t.Fatalf("write sys: %v", err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	if HasWinDivertFiles(link, "") {
		t.Fatal("expected symlinked WinDivert directory to be rejected")
	}
}

func TestVerifyPinnedWinDivertX64FilesRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.dll")
	if err := os.WriteFile(target, []byte("dll"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "WinDivert.dll")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "WinDivert64.sys"), []byte("bad sys"), 0o644); err != nil {
		t.Fatalf("write sys: %v", err)
	}

	err := VerifyPinnedWinDivertX64Files(dir)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink rejection, got %v", err)
	}
}

func TestExtractZipFileRejectsOversizedHeader(t *testing.T) {
	f := &zip.File{
		FileHeader: zip.FileHeader{
			Name:               "WinDivert-2.2.2-A/x64/WinDivert.dll",
			UncompressedSize64: maxWinDivertExtractFileBytes + 1,
		},
	}
	err := extractZipFile(f, filepath.Join(t.TempDir(), "WinDivert.dll"))
	if err == nil || !strings.Contains(err.Error(), "oversized zip entry") {
		t.Fatalf("expected oversized entry error, got %v", err)
	}
}

func TestExtractZipFileRejectsDirectoryEntry(t *testing.T) {
	f := &zip.File{
		FileHeader: zip.FileHeader{
			Name: "WinDivert-2.2.2-A/x64/WinDivert.dll/",
		},
	}
	f.SetMode(os.ModeDir | 0o755)
	err := extractZipFile(f, filepath.Join(t.TempDir(), "WinDivert.dll"))
	if err == nil || !strings.Contains(err.Error(), "directory entry") {
		t.Fatalf("expected directory entry error, got %v", err)
	}
}

func TestExtractZipFileRejectsSymlinkDestination(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("WinDivert-2.2.2-A/x64/WinDivert.dll")
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	if _, err := w.Write([]byte("dll")); err != nil {
		t.Fatalf("write zip entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target.dll")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	dest := filepath.Join(dir, "WinDivert.dll")
	if err := os.Symlink(target, dest); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	err = extractZipFile(zr.File[0], dest)
	if err == nil || !strings.Contains(err.Error(), "symlink WinDivert destination") {
		t.Fatalf("expected symlink destination rejection, got %v", err)
	}
}

func installWinDivertPathTestStubs(t *testing.T, existing map[string]bool, reparse map[string]bool) {
	t.Helper()
	origAttrs := winDivertGetFileAttributes
	t.Cleanup(func() { winDivertGetFileAttributes = origAttrs })

	normalize := func(path string) string {
		return filepath.Clean(path)
	}
	winDivertGetFileAttributes = func(path *uint16) (uint32, error) {
		name := normalize(windows.UTF16PtrToString(path))
		if reparse[name] {
			return windows.FILE_ATTRIBUTE_REPARSE_POINT, nil
		}
		if existing[name] {
			return windows.FILE_ATTRIBUTE_DIRECTORY, nil
		}
		return 0, windows.ERROR_PATH_NOT_FOUND
	}
}

func TestPrepareWinDivertDirectoryRejectsReparseBeforeMkdir(t *testing.T) {
	installWinDivertPathTestStubs(t,
		map[string]bool{
			filepath.Clean(`C:\`): true,
		},
		map[string]bool{
			filepath.Clean(`C:\Temp`): true,
		},
	)

	origMkdirAll := winDivertMkdirAll
	mkdirCalled := false
	winDivertMkdirAll = func(string, os.FileMode) error {
		mkdirCalled = true
		return nil
	}
	t.Cleanup(func() { winDivertMkdirAll = origMkdirAll })

	err := prepareWinDivertDirectory(`C:\Temp\windivert`)
	if err == nil || !strings.Contains(err.Error(), "reparse") {
		t.Fatalf("expected reparse rejection, got %v", err)
	}
	if mkdirCalled {
		t.Fatal("MkdirAll was called after pre-existing reparse point")
	}
}

func TestPrepareWinDivertDirectoryRejectsReparseAfterMkdir(t *testing.T) {
	existing := map[string]bool{
		filepath.Clean(`C:\`):     true,
		filepath.Clean(`C:\Temp`): true,
	}
	reparse := map[string]bool{}
	installWinDivertPathTestStubs(t, existing, reparse)

	origMkdirAll := winDivertMkdirAll
	mkdirCalled := false
	winDivertMkdirAll = func(path string, _ os.FileMode) error {
		mkdirCalled = true
		clean := filepath.Clean(path)
		existing[clean] = true
		reparse[clean] = true
		return nil
	}
	t.Cleanup(func() { winDivertMkdirAll = origMkdirAll })

	err := prepareWinDivertDirectory(`C:\Temp\windivert`)
	if err == nil || !strings.Contains(err.Error(), "reparse") {
		t.Fatalf("expected post-create reparse rejection, got %v", err)
	}
	if !mkdirCalled {
		t.Fatal("MkdirAll was not called")
	}
}

func TestExtractZipFileCopiesSmallFile(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("WinDivert-2.2.2-A/x64/WinDivert.dll")
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	if _, err := w.Write([]byte("dll")); err != nil {
		t.Fatalf("write zip entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	dest := filepath.Join(t.TempDir(), "WinDivert.dll")
	if err := extractZipFile(zr.File[0], dest); err != nil {
		t.Fatalf("extract zip file: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read extracted file: %v", err)
	}
	if string(got) != "dll" {
		t.Fatalf("extracted file = %q", string(got))
	}
}
