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
)

// HasWinDivertFiles returns true if the directory looks usable for a 64-bit
// WinDivert app, i.e. it contains WinDivert.dll and a .sys driver file.
//
// Note: this is a best-effort check; it does not validate digital signatures.
func HasWinDivertFiles(dir string, sysName string) bool {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return false
	}
	if _, err := os.Stat(filepath.Join(dir, "WinDivert.dll")); err != nil {
		return false
	}
	if sysName != "" {
		if _, err := os.Stat(filepath.Join(dir, sysName)); err != nil {
			return false
		}
		return true
	}
	// Match driver.resolveSysPath behavior.
	if _, err := os.Stat(filepath.Join(dir, "WinDivert64.sys")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(dir, "WinDivert.sys")); err == nil {
		return true
	}
	return false
}

// DownloadWinDivertX64 downloads the pinned WinDivert distribution zip and
// extracts the x64 WinDivert.dll and WinDivert64.sys into destDir.
func DownloadWinDivertX64(ctx context.Context, destDir string) error {
	destDir = strings.TrimSpace(destDir)
	if destDir == "" {
		return errors.New("destDir is empty")
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create dest dir failed: %w", err)
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
	if _, err := io.Copy(io.MultiWriter(tmp, h), resp.Body); err != nil {
		return fmt.Errorf("download failed: %w", err)
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

	return nil
}

func extractZipFile(f *zip.File, destPath string) error {
	if f == nil {
		return errors.New("zip file is nil")
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
	if err := os.MkdirAll(dir, 0o755); err != nil {
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

	if _, err := io.Copy(tmp, rc); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	// Replace existing file (if any).
	_ = os.Remove(destPath)
	if err := os.Rename(tmpPath, destPath); err != nil {
		return err
	}
	return nil
}

