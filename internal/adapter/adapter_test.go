package adapter

import (
	"context"
	"errors"
	"testing"

	"fk-gov/internal/packet"
)

var _ Adapter = (*StubAdapter)(nil)

func TestStubAdapterMethods(t *testing.T) {
	s := NewStub()
	ctx := context.Background()

	if _, err := s.Recv(ctx); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("Recv error mismatch: %v", err)
	}
	if err := s.Send(ctx, &packet.Packet{}); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("Send error mismatch: %v", err)
	}
	if err := s.Drop(ctx, &packet.Packet{}); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("Drop error mismatch: %v", err)
	}
	if err := s.CalcChecksums(&packet.Packet{}); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("CalcChecksums error mismatch: %v", err)
	}
	if err := s.Flush(ctx); err != nil {
		t.Fatalf("Flush should succeed, got: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close should succeed, got: %v", err)
	}
}
