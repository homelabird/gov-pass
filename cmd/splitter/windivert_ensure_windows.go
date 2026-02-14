//go:build windows

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"fk-gov/internal/driver"
)

func ensureWinDivertFiles(ctx context.Context, wc windowsRunConfig, exeDir string) (windowsRunConfig, string, error) {
	exeDir = strings.TrimSpace(exeDir)
	requested := strings.TrimSpace(wc.WinDivertDir)

	driverDir := requested
	if driverDir == "" {
		driverDir = exeDir
	}
	driverDir = strings.TrimSpace(driverDir)
	if driverDir == "" {
		return wc, "", fmt.Errorf("could not resolve WinDivert directory (exeDir=%q)", exeDir)
	}

	if driver.HasWinDivertFiles(driverDir, wc.WinDivertSys) {
		return wc, driverDir, nil
	}

	if !wc.AutoDownloadFiles {
		return wc, driverDir, fmt.Errorf("WinDivert files not found in %s (expected WinDivert.dll and WinDivert64.sys); install them or set --auto-download-windivert=true", driverDir)
	}

	log.Printf("WinDivert files missing in %s; downloading pinned WinDivert zip", driverDir)
	if err := driver.DownloadWinDivertX64(ctx, driverDir); err == nil {
		return wc, driverDir, nil
	} else if requested == "" && os.IsPermission(err) {
		// If we didn't explicitly choose a directory and exeDir isn't writable
		// (eg, Program Files), fall back to ProgramData.
		programDataRoot := filepath.Join(defaultProgramDataDir(), "gov-pass")
		fallback := filepath.Join(programDataRoot, "windivert")
		if err := ensureSecureWindowsDir(programDataRoot); err != nil {
			return wc, fallback, fmt.Errorf("secure ProgramData dir failed: %w", err)
		}
		if err := ensureSecureWindowsDir(fallback); err != nil {
			return wc, fallback, fmt.Errorf("secure windivert dir failed: %w", err)
		}
		log.Printf("WinDivert download to %s failed (permission); retrying in %s", driverDir, fallback)
		if err2 := driver.DownloadWinDivertX64(ctx, fallback); err2 != nil {
			return wc, fallback, err2
		}
		wc.WinDivertDir = fallback
		return wc, fallback, nil
	} else {
		return wc, driverDir, err
	}
}
