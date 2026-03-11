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

type windowsManagedPath struct {
	cleanAbs         string
	existingAbs      string
	existingResolved string
	missingParts     []string
}

var (
	windowsPathLstat         = os.Lstat
	windowsPathEvalSymlinks  = filepath.EvalSymlinks
	windowsGetFileAttributes = func(path *uint16) (uint32, error) { return windows.GetFileAttributes(path) }
)

func validateManagedWindowsPath(path string, allowedRoots ...string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("path is empty")
	}
	if !filepath.IsAbs(path) {
		return errors.New("path must be absolute")
	}

	inspectedPath, err := inspectManagedWindowsPath(path)
	if err != nil {
		return err
	}
	for _, root := range allowedRoots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		inspectedRoot, err := inspectManagedWindowsPath(root)
		if err != nil {
			continue
		}
		if managedWindowsPathWithinRoot(inspectedPath, inspectedRoot) {
			return nil
		}
	}
	return fmt.Errorf("path must stay under one of: %s", strings.Join(allowedRoots, ", "))
}

func inspectManagedWindowsPath(path string) (windowsManagedPath, error) {
	cleanAbs, err := filepath.Abs(filepath.Clean(strings.TrimSpace(path)))
	if err != nil {
		return windowsManagedPath{}, err
	}
	volume := filepath.VolumeName(cleanAbs)
	if volume == "" {
		return windowsManagedPath{}, fmt.Errorf("path must include a drive root: %s", path)
	}

	root := volume + string(os.PathSeparator)
	trimmed := strings.TrimPrefix(cleanAbs, root)
	parts := splitManagedWindowsPath(trimmed)

	info := windowsManagedPath{
		cleanAbs:         cleanAbs,
		existingAbs:      root,
		existingResolved: root,
	}
	current := root
	for idx, part := range parts {
		current = filepath.Join(current, part)

		if unsafe, err := windowsPathHasReparsePoint(current); err != nil && !errors.Is(err, os.ErrNotExist) {
			return windowsManagedPath{}, err
		} else if unsafe {
			return windowsManagedPath{}, fmt.Errorf("managed path contains reparse point: %s", current)
		}

		if _, err := windowsPathLstat(current); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				info.missingParts = append([]string(nil), parts[idx:]...)
				return info, nil
			}
			return windowsManagedPath{}, err
		}

		resolved, err := windowsPathEvalSymlinks(current)
		if err != nil {
			return windowsManagedPath{}, err
		}
		info.existingAbs = current
		info.existingResolved = filepath.Clean(resolved)
	}

	return info, nil
}

func managedWindowsPathWithinRoot(pathInfo windowsManagedPath, rootInfo windowsManagedPath) bool {
	if rootInfo.cleanAbs == "" || pathInfo.cleanAbs == "" {
		return false
	}
	if len(rootInfo.missingParts) == 0 {
		return sameOrChildWindowsPath(pathInfo.existingResolved, rootInfo.existingResolved)
	}
	if !sameWindowsPath(pathInfo.existingResolved, rootInfo.existingResolved) {
		return false
	}
	return windowsPartsHasPrefix(pathInfo.missingParts, rootInfo.missingParts)
}

func sameOrChildWindowsPath(path string, root string) bool {
	path = filepath.Clean(strings.TrimSpace(path))
	root = filepath.Clean(strings.TrimSpace(root))
	if sameWindowsPath(path, root) {
		return true
	}
	if !strings.HasSuffix(root, string(os.PathSeparator)) {
		root += string(os.PathSeparator)
	}
	return strings.HasPrefix(strings.ToLower(path), strings.ToLower(root))
}

func sameWindowsPath(a string, b string) bool {
	return strings.EqualFold(filepath.Clean(strings.TrimSpace(a)), filepath.Clean(strings.TrimSpace(b)))
}

func windowsPartsHasPrefix(parts []string, prefix []string) bool {
	if len(prefix) > len(parts) {
		return false
	}
	for i := range prefix {
		if !strings.EqualFold(parts[i], prefix[i]) {
			return false
		}
	}
	return true
}

func splitManagedWindowsPath(path string) []string {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	raw := strings.Split(path, string(os.PathSeparator))
	out := make([]string, 0, len(raw))
	for _, part := range raw {
		part = strings.TrimSpace(part)
		if part == "" || part == "." {
			continue
		}
		out = append(out, part)
	}
	return out
}

func windowsPathHasReparsePoint(path string) (bool, error) {
	ptr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false, err
	}
	attrs, err := windowsGetFileAttributes(ptr)
	if err != nil {
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
			return false, os.ErrNotExist
		}
		return false, err
	}
	return attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0, nil
}
