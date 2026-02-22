//go:build !windows

package driver

import (
	"context"
	"errors"
)

var ErrNotSupported = errors.New("WinDivert driver is only supported on Windows")

func Ensure(ctx context.Context, cfg Config) (func() error, error) {
	return nil, ErrNotSupported
}
