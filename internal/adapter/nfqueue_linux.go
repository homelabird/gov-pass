//go:build linux

package adapter

import (
	"context"

	"fk-gov/internal/packet"
)

// NFQueueAdapter is a Linux adapter skeleton for NFQUEUE.
type NFQueueAdapter struct {
	opts NFQueueOptions
}

func NewNFQueue(opts NFQueueOptions) (*NFQueueAdapter, error) {
	return nil, ErrNotImplemented
}

func (n *NFQueueAdapter) Recv(ctx context.Context) (*packet.Packet, error) {
	return nil, ErrNotImplemented
}

func (n *NFQueueAdapter) Send(ctx context.Context, pkt *packet.Packet) error {
	return ErrNotImplemented
}

func (n *NFQueueAdapter) CalcChecksums(pkt *packet.Packet) error {
	return ErrNotImplemented
}

func (n *NFQueueAdapter) Close() error {
	return nil
}
