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
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return hardenWindowsDirACL(dir)
}

func hardenWindowsDirACL(dir string) error {
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
	sysDir, err := windows.GetSystemDirectory()
	if err != nil {
		root := strings.TrimSpace(os.Getenv("SystemRoot"))
		if root == "" {
			root = `C:\Windows`
		}
		sysDir = filepath.Join(root, "System32")
	}
	path := filepath.Join(strings.TrimSpace(sysDir), name)
	if _, err := os.Stat(path); err != nil {
		return "", err
	}
	return path, nil
}
