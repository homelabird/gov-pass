//go:build linux || freebsd

package main

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

func openRegularConfigFile(path string) (*os.File, os.FileInfo, error) {
	// #nosec G304 -- config path is explicitly provided by the operator and
	// opened with O_NOFOLLOW before validating the opened file descriptor.
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		if errors.Is(err, syscall.ELOOP) {
			return nil, nil, fmt.Errorf("config path must not be a symlink: %s", path)
		}
		return nil, nil, err
	}

	f := os.NewFile(uintptr(fd), path)
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		_ = f.Close()
		return nil, nil, fmt.Errorf("config path must be a regular file: %s", path)
	}
	return f, info, nil
}
