//go:build !windows

package adapter

import (
	"context"

	"fk-gov/internal/packet"
)

type WinDivertAdapter struct{}

func NewWinDivert(filter string, opts WinDivertOptions) (*WinDivertAdapter, error) {
	return nil, ErrNotImplemented
}

func (w *WinDivertAdapter) Drop(ctx context.Context, pkt *packet.Packet) error {
	return ErrNotImplemented
}
