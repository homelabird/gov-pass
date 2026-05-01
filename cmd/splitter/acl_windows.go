//go:build windows

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

var windowsMkdirAll = os.MkdirAll

const (
	sidSystem         = "*S-1-5-18"
	sidAdministrators = "*S-1-5-32-544"
	sidUsers          = "*S-1-5-32-545"
)

func ensureSecureWindowsDir(dir string) error {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return fmt.Errorf("dir is empty")
	}
	if err := rejectWindowsReparsePath(dir); err != nil {
		return err
	}
	if err := windowsMkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := rejectWindowsReparsePath(dir); err != nil {
		return err
	}
	return hardenWindowsDirACL(dir)
}

func hardenWindowsDirACL(dir string) error {
	if err := rejectWindowsReparsePath(dir); err != nil {
		return err
	}
	icacls, err := resolveSystem32WindowsCommand("icacls.exe")
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Reset first to remove any explicit ACEs from older installs that could
	// grant write access to unprivileged users.
	if out, err := exec.CommandContext(ctx, icacls, dir, "/reset").CombinedOutput(); err != nil {
		return fmt.Errorf("icacls reset failed (%s): %w: %s", dir, err, strings.TrimSpace(string(out)))
	}

	args := []string{
		dir,
		"/inheritance:r",
		"/grant:r",
		sidSystem + ":(OI)(CI)F",
		sidAdministrators + ":(OI)(CI)F",
		sidUsers + ":(OI)(CI)RX",
	}
	if out, err := exec.CommandContext(ctx, icacls, args...).CombinedOutput(); err != nil {
		return fmt.Errorf("icacls harden dir failed (%s): %w: %s", dir, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func hardenWindowsFileACL(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("path is empty")
	}
	if err := rejectWindowsReparsePath(path); err != nil {
		return err
	}
	icacls, err := resolveSystem32WindowsCommand("icacls.exe")
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if out, err := exec.CommandContext(ctx, icacls, path, "/reset").CombinedOutput(); err != nil {
		return fmt.Errorf("icacls reset failed (%s): %w: %s", path, err, strings.TrimSpace(string(out)))
	}

	args := []string{
		path,
		"/inheritance:r",
		"/grant:r",
		sidSystem + ":F",
		sidAdministrators + ":F",
		sidUsers + ":R",
	}
	if out, err := exec.CommandContext(ctx, icacls, args...).CombinedOutput(); err != nil {
		return fmt.Errorf("icacls harden file failed (%s): %w: %s", path, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func resolveSystem32WindowsCommand(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("command name is empty")
	}
	if filepath.Base(name) != name || filepath.VolumeName(name) != "" {
		return "", fmt.Errorf("command name must be a bare filename: %s", name)
	}
	sysDir, err := windows.GetSystemDirectory()
	if err != nil {
		return "", fmt.Errorf("resolve System32 failed: %w", err)
	}
	if strings.TrimSpace(sysDir) == "" {
		return "", fmt.Errorf("resolve System32 failed: empty path")
	}
	path := filepath.Join(strings.TrimSpace(sysDir), name)
	if err := rejectWindowsReparsePath(path); err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("System32 command is a directory: %s", path)
	}
	return path, nil
}

func rejectWindowsReparsePath(path string) error {
	if _, err := inspectManagedWindowsPath(path); err != nil {
		return fmt.Errorf("refusing reparse point path %s: %w", path, err)
	}
	return nil
}
