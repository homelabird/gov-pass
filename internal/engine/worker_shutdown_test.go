package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"fk-gov/internal/adapter"
	"fk-gov/internal/flow"
	"fk-gov/internal/packet"
)

type recordingAdapter struct {
	sends []*packet.Packet
}

var _ adapter.Adapter = (*recordingAdapter)(nil)

func (a *recordingAdapter) Recv(ctx context.Context) (*packet.Packet, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, context.Canceled
}

func (a *recordingAdapter) Send(ctx context.Context, pkt *packet.Packet) error {
	a.sends = append(a.sends, pkt)
	return nil
}

func (a *recordingAdapter) Drop(ctx context.Context, pkt *packet.Packet) error {
	return nil
}

func (a *recordingAdapter) CalcChecksums(pkt *packet.Packet) error {
	return nil
}

func (a *recordingAdapter) Flush(ctx context.Context) error {
	return nil
}

func (a *recordingAdapter) Close() error {
	return nil
}

func TestWorkerShutdownFailOpen_OrderAndDrain(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ShutdownFailOpenMaxPackets = 10
	ad := &recordingAdapter{}
	w := newWorker(0, cfg, ad)

	key := flow.Key{SrcPort: 1234, DstPort: 443, Proto: 6}
	st := w.flows.GetOrCreate(key, time.Now())

	p1 := &packet.Packet{Data: []byte{1}}
	p2 := &packet.Packet{Data: []byte{2}}
	st.HeldPackets = []*packet.Packet{p1}
	w.in <- p2

	if err := w.shutdownFailOpen(context.Background()); err != nil {
		t.Fatalf("shutdownFailOpen returned error: %v", err)
	}
	if got, want := len(ad.sends), 2; got != want {
		t.Fatalf("send count: got %d, want %d", got, want)
	}
	if ad.sends[0] != p1 {
		t.Fatalf("first send: got %p, want %p", ad.sends[0], p1)
	}
	if ad.sends[1] != p2 {
		t.Fatalf("second send: got %p, want %p", ad.sends[1], p2)
	}
}

func TestWorkerShutdownFailOpen_Limit(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ShutdownFailOpenMaxPackets = 2
	ad := &recordingAdapter{}
	w := newWorker(0, cfg, ad)

	key := flow.Key{SrcPort: 1234, DstPort: 443, Proto: 6}
	st := w.flows.GetOrCreate(key, time.Now())

	p1 := &packet.Packet{Data: []byte{1}}
	p2 := &packet.Packet{Data: []byte{2}}
	p3 := &packet.Packet{Data: []byte{3}}
	st.HeldPackets = []*packet.Packet{p1, p2}
	w.in <- p3

	err := w.shutdownFailOpen(context.Background())
	if !errors.Is(err, ErrShutdownFailOpenLimitReached) {
		t.Fatalf("expected ErrShutdownFailOpenLimitReached, got %v", err)
	}
	if got, want := len(ad.sends), 2; got != want {
		t.Fatalf("send count: got %d, want %d", got, want)
	}
	if ad.sends[0] != p1 || ad.sends[1] != p2 {
		t.Fatalf("unexpected send order: got %p,%p want %p,%p", ad.sends[0], ad.sends[1], p1, p2)
	}
}

func TestWorkerShutdownFailOpen_CanceledContext(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ShutdownFailOpenMaxPackets = 10
	ad := &recordingAdapter{}
	w := newWorker(0, cfg, ad)

	key := flow.Key{SrcPort: 1234, DstPort: 443, Proto: 6}
	st := w.flows.GetOrCreate(key, time.Now())

	p1 := &packet.Packet{Data: []byte{1}}
	p2 := &packet.Packet{Data: []byte{2}}
	st.HeldPackets = []*packet.Packet{p1}
	w.in <- p2

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := w.shutdownFailOpen(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if got, want := len(ad.sends), 0; got != want {
		t.Fatalf("send count: got %d, want %d", got, want)
	}
}

