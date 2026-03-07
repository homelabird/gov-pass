//go:build !windows

package driver

import (
	"context"
	"errors"
)

var ErrNotSupported = errors.New("WinDivert driver is only supported on Windows")

func Ensure(ctx context.Context, cfg Config) (func() error, error) {
	_, cleanup, err := EnsureWithReport(ctx, cfg)
	return cleanup, err
}

func EnsureWithReport(ctx context.Context, cfg Config) (Report, func() error, error) {
	return Report{}, nil, ErrNotSupported
}

func Inspect(ctx context.Context, cfg Config) (Report, error) {
	return Report{}, ErrNotSupported
}
