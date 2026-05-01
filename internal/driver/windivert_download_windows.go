//go:build windows

package driver

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// Official WinDivert distribution zip. We pin the SHA256 of the whole zip
	// so the auto-download is deterministic and tamper-evident.
	//
	// Source: https://github.com/basil00/WinDivert/releases/tag/v2.2.2
	winDivertZipURL    = "https://github.com/basil00/WinDivert/releases/download/v2.2.2/WinDivert-2.2.2-A.zip"
	winDivertZipSHA256 = "63cb41763bb4b20f600b6de04e991a9c2be73279e317d4d82f237b150c5f3f15"

	winDivertDLLSHA256 = "c1e060ee19444a259b2162f8af0f3fe8c4428a1c6f694dce20de194ac8d7d9a2"
	winDivertSysSHA256 = "8da085332782708d8767bcace5327a6ec7283c17cfb85e40b03cd2323a90ddc2"

	maxWinDivertZipDownloadBytes = 50 * 1024 * 1024
	maxWinDivertExtractFileBytes = 16 * 1024 * 1024
)

var winDivertMkdirAll = os.MkdirAll

// HasWinDivertFiles returns true if the directory looks usable for a 64-bit
// WinDivert app, i.e. it contains WinDivert.dll and a .sys driver file.
//
// Note: this is only a presence check for already-provided files. The automatic
// download path verifies the pinned distribution zip and extracted x64 files.
func HasWinDivertFiles(dir string, sysName string) bool {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return false
	}
	if err := validateWinDivertDirectory(dir); err != nil {
		return false
	}
	if err := validateWinDivertRegularFile(filepath.Join(dir, "WinDivert.dll")); err != nil {
		return false
	}
	if sysName != "" {
		if err := validateWinDivertRegularFile(filepath.Join(dir, sysName)); err != nil {
			return false
		}
		return true
	}
	// Match driver.resolveSysPath behavior.
	if err := validateWinDivertRegularFile(filepath.Join(dir, "WinDivert64.sys")); err == nil {
		return true
	}
	if err := validateWinDivertRegularFile(filepath.Join(dir, "WinDivert.sys")); err == nil {
		return true
	}
	return false
}

// VerifyPinnedWinDivertX64Files verifies the known x64 files from the pinned
// WinDivert release used by the auto-download path.
func VerifyPinnedWinDivertX64Files(dir string) error {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return errors.New("dir is empty")
	}
	checks := map[string]string{
		"WinDivert.dll":   winDivertDLLSHA256,
		"WinDivert64.sys": winDivertSysSHA256,
	}
	for name, want := range checks {
		if err := verifyFileSHA256(filepath.Join(dir, name), want); err != nil {
			return err
		}
	}
	return nil
}

// DownloadWinDivertX64 downloads the pinned WinDivert distribution zip and
// extracts the x64 WinDivert.dll and WinDivert64.sys into destDir.
func DownloadWinDivertX64(ctx context.Context, destDir string) error {
	destDir = strings.TrimSpace(destDir)
	if destDir == "" {
		return errors.New("destDir is empty")
	}
	if err := prepareWinDivertDirectory(destDir); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(destDir, "windivert-*.zip")
	if err != nil {
		return fmt.Errorf("create temp file failed: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()

	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, winDivertZipURL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %s", resp.Status)
	}

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(resp.Body, maxWinDivertZipDownloadBytes+1))
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	if n > maxWinDivertZipDownloadBytes {
		return fmt.Errorf("download failed: WinDivert zip exceeds %d bytes", maxWinDivertZipDownloadBytes)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file failed: %w", err)
	}

	sum := hex.EncodeToString(h.Sum(nil))
	if strings.ToLower(sum) != winDivertZipSHA256 {
		return fmt.Errorf("windivert zip SHA256 mismatch: got %s, want %s", sum, winDivertZipSHA256)
	}

	zr, err := zip.OpenReader(tmpPath)
	if err != nil {
		return fmt.Errorf("open zip failed: %w", err)
	}
	defer func() {
		_ = zr.Close()
	}()

	need := map[string]string{
		"windivert.dll":   "WinDivert.dll",
		"windivert64.sys": "WinDivert64.sys",
	}
	found := make(map[string]bool, len(need))

	for _, f := range zr.File {
		name := strings.ToLower(strings.ReplaceAll(f.Name, "\\", "/"))
		if strings.Contains(name, "/x64/") && strings.HasSuffix(name, "/windivert.dll") {
			if err := extractZipFile(f, filepath.Join(destDir, need["windivert.dll"])); err != nil {
				return err
			}
			found["windivert.dll"] = true
			continue
		}
		if strings.Contains(name, "/x64/") && strings.HasSuffix(name, "/windivert64.sys") {
			if err := extractZipFile(f, filepath.Join(destDir, need["windivert64.sys"])); err != nil {
				return err
			}
			found["windivert64.sys"] = true
			continue
		}
	}

	for k := range need {
		if !found[k] {
			return fmt.Errorf("windivert zip missing expected x64 file: %s", need[k])
		}
	}

	if err := VerifyPinnedWinDivertX64Files(destDir); err != nil {
		return fmt.Errorf("verify extracted WinDivert files failed: %w", err)
	}
	return nil
}

func verifyFileSHA256(path string, want string) error {
	if err := validateWinDivertRegularFile(path); err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("%s SHA256 mismatch: got %s, want %s", filepath.Base(path), got, want)
	}
	return nil
}

func validateWinDivertRegularFile(path string) error {
	if err := rejectWinDivertPathReparseComponents(path); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s must not be a symlink", filepath.Base(path))
	}
	if unsafe, err := winDivertPathHasReparsePoint(path); err != nil {
		return err
	} else if unsafe {
		return fmt.Errorf("%s must not be a reparse point", filepath.Base(path))
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s must be a regular file", filepath.Base(path))
	}
	return nil
}

func validateWinDivertDirectory(dir string) error {
	if err := rejectWinDivertPathReparseComponents(dir); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("WinDivert directory must not be a symlink: %s", dir)
	}
	if unsafe, err := winDivertPathHasReparsePoint(dir); err != nil {
		return err
	} else if unsafe {
		return fmt.Errorf("WinDivert directory must not be a reparse point: %s", dir)
	}
	if !info.IsDir() {
		return fmt.Errorf("WinDivert directory must be a directory: %s", dir)
	}
	return nil
}

func prepareWinDivertDirectory(dir string) error {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return errors.New("WinDivert directory is empty")
	}
	if err := rejectWinDivertPathReparseComponents(dir); err != nil {
		return err
	}
	if err := winDivertMkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create WinDivert directory failed: %w", err)
	}
	if err := rejectWinDivertPathReparseComponents(dir); err != nil {
		return err
	}
	return validateWinDivertDirectory(dir)
}

func rejectWinDivertPathReparseComponents(path string) error {
	cleanAbs, err := filepath.Abs(filepath.Clean(strings.TrimSpace(path)))
	if err != nil {
		return err
	}
	volume := filepath.VolumeName(cleanAbs)
	if volume == "" {
		return fmt.Errorf("WinDivert path must include a drive root: %s", path)
	}

	root := volume + string(os.PathSeparator)
	trimmed := strings.TrimPrefix(cleanAbs, root)
	current := root
	for _, part := range strings.Split(trimmed, string(os.PathSeparator)) {
		part = strings.TrimSpace(part)
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		unsafe, err := winDivertPathHasReparsePoint(current)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if unsafe {
			return fmt.Errorf("WinDivert path contains reparse point: %s", current)
		}
	}
	return nil
}

func extractZipFile(f *zip.File, destPath string) error {
	if f == nil {
		return errors.New("zip file is nil")
	}
	if f.FileInfo().IsDir() {
		return fmt.Errorf("refusing to extract directory entry: %s", f.Name)
	}
	if f.UncompressedSize64 > maxWinDivertExtractFileBytes {
		return fmt.Errorf("refusing to extract oversized zip entry %s: %d bytes", f.Name, f.UncompressedSize64)
	}
	destPath = strings.TrimSpace(destPath)
	if destPath == "" {
		return errors.New("destPath is empty")
	}

	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer func() {
		_ = rc.Close()
	}()

	dir := filepath.Dir(destPath)
	if err := prepareWinDivertDirectory(dir); err != nil {
		return err
	}
	if err := rejectWinDivertExtractDestination(destPath); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(destPath)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()

	n, err := io.Copy(tmp, io.LimitReader(rc, maxWinDivertExtractFileBytes+1))
	if err != nil {
		return err
	}
	if n > maxWinDivertExtractFileBytes {
		return fmt.Errorf("refusing to extract oversized zip entry %s: exceeds %d bytes", f.Name, maxWinDivertExtractFileBytes)
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	// Replace existing file (if any).
	if err := rejectWinDivertExtractDestination(destPath); err != nil {
		return err
	}
	_ = os.Remove(destPath)
	if err := os.Rename(tmpPath, destPath); err != nil {
		return err
	}
	return nil
}

func rejectWinDivertExtractDestination(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to overwrite symlink WinDivert destination: %s", path)
	}
	if unsafe, err := winDivertPathHasReparsePoint(path); err != nil {
		return err
	} else if unsafe {
		return fmt.Errorf("refusing to overwrite reparse point WinDivert destination: %s", path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("refusing to overwrite non-regular WinDivert destination: %s", path)
	}
	return nil
}
