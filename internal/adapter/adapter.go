package adapter

import (
	"context"
	"errors"

	"fk-gov/internal/packet"
)

// Adapter abstracts WinDivert recv/send.
type Adapter interface {
	Recv(ctx context.Context) (*packet.Packet, error)
	Send(ctx context.Context, pkt *packet.Packet) error
	CalcChecksums(pkt *packet.Packet) error
	Close() error
}

var ErrNotImplemented = errors.New("adapter not implemented")

// WinDivertOptions holds optional queue parameters.
type WinDivertOptions struct {
	QueueLen  uint64
	QueueTime uint64
	QueueSize uint64
}

// NFQueueOptions holds NFQUEUE parameters for Linux.
type NFQueueOptions struct {
	QueueNum    uint16
	QueueMaxLen uint32
	CopyRange   uint32
	Mark        uint32
}

// StubAdapter is a placeholder until WinDivert integration lands.
type StubAdapter struct{}

func NewStub() *StubAdapter {
	return &StubAdapter{}
}

func (s *StubAdapter) Recv(ctx context.Context) (*packet.Packet, error) {
	return nil, ErrNotImplemented
}

func (s *StubAdapter) Send(ctx context.Context, pkt *packet.Packet) error {
	return ErrNotImplemented
}

func (s *StubAdapter) CalcChecksums(pkt *packet.Packet) error {
	return ErrNotImplemented
}

func (s *StubAdapter) Close() error {
	return nil
}
