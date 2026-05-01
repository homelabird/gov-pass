//go:build windows

package driver

import (
	"errors"
	"os"
	"strings"

	"golang.org/x/sys/windows"
)

var winDivertGetFileAttributes = windows.GetFileAttributes

func winDivertPathHasReparsePoint(path string) (bool, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return false, errors.New("path is empty")
	}
	ptr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false, err
	}
	attrs, err := winDivertGetFileAttributes(ptr)
	if err != nil {
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
			return false, os.ErrNotExist
		}
		return false, err
	}
	return attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0, nil
}
