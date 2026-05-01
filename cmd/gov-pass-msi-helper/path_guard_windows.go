//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

var helperGetFileAttributes = func(path *uint16) (uint32, error) { return windows.GetFileAttributes(path) }

func windowsPathHasReparsePoint(path string) (bool, error) {
	ptr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false, err
	}
	attrs, err := helperGetFileAttributes(ptr)
	if err != nil {
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
			return false, os.ErrNotExist
		}
		return false, err
	}
	return attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0, nil
}

func safeRemoveAllWindows(path string) error {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" || path == "." {
		return errors.New("refusing to remove empty path")
	}
	if unsafe, err := windowsPathHasReparsePoint(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	} else if unsafe {
		return fmt.Errorf("refusing to remove reparse point: %s", path)
	}

	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return removeCheckedWindowsPath(path, false)
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		child := filepath.Join(path, entry.Name())
		if err := safeRemoveAllWindows(child); err != nil {
			return err
		}
	}
	return removeCheckedWindowsPath(path, true)
}

func removeCheckedWindowsPath(path string, wantDir bool) error {
	if unsafe, err := windowsPathHasReparsePoint(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	} else if unsafe {
		return fmt.Errorf("refusing to remove reparse point: %s", path)
	}
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if wantDir && !info.IsDir() {
		return fmt.Errorf("refusing to remove replaced non-directory: %s", path)
	}
	if !wantDir && info.IsDir() {
		return fmt.Errorf("refusing to remove replaced directory as file: %s", path)
	}
	return os.Remove(path)
}
