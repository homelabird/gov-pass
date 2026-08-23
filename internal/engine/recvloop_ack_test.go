package engine

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"fk-gov/internal/flow"
	"fk-gov/internal/packet"
)

type recvSequenceAdapter struct {
	mu      sync.Mutex
	packets []*packet.Packet
	sends   []*packet.Packet
}

func (a *recvSequenceAdapter) Recv(ctx context.Context) (*packet.Packet, error) {
	a.mu.Lock()
	if len(a.packets) > 0 {
		pkt := a.packets[0]
		a.packets = a.packets[1:]
		a.mu.Unlock()
		return pkt, nil
	}
	a.mu.Unlock()
	<-ctx.Done()
	return nil, ctx.Err()
}

func (a *recvSequenceAdapter) Send(ctx context.Context, pkt *packet.Packet) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sends = append(a.sends, pkt)
	return nil
}

func (a *recvSequenceAdapter) Drop(ctx context.Context, pkt *packet.Packet) error {
	return nil
}

func (a *recvSequenceAdapter) CalcChecksums(pkt *packet.Packet) error {
	return nil
}

func (a *recvSequenceAdapter) Flush(ctx context.Context) error {
	return nil
}

func (a *recvSequenceAdapter) Close() error {
	return nil
}

func (a *recvSequenceAdapter) sendCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.sends)
}

func TestRecvLoop_ACKOnlyFastPathSendsImmediately(t *testing.T) {
	cfg := DefaultConfig()
	cfg.WorkerCount = 1

	pkt := buildTestIPv4Packet(t, 12345, 0x01020304, packet.TCPFlagACK, nil)
	ad := &recvSequenceAdapter{packets: []*packet.Packet{pkt}}
	eng := New(cfg, ad)

	key := flow.KeyFromMeta(pkt.Meta)
	eng.workers[0].flows.GetOrCreate(key, time.Now())

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- eng.recvLoop(ctx)
	}()

	waitForCondition(t, func() bool {
		return ad.sendCount() == 1
	})
	cancel()

	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("recvLoop error = %v, want context.Canceled", err)
	}
	if got := len(eng.workers[0].in); got != 0 {
		t.Fatalf("worker queue len = %d, want 0", got)
	}
}

func TestRecvLoop_ACKOnlyDoesNotBlockWhenTouchIsFull(t *testing.T) {
	cfg := DefaultConfig()
	cfg.WorkerCount = 1

	pkt := buildTestIPv4Packet(t, 12345, 0x01020304, packet.TCPFlagACK, nil)
	ad := &recvSequenceAdapter{packets: []*packet.Packet{pkt}}
	eng := New(cfg, ad)

	key := flow.KeyFromMeta(pkt.Meta)
	eng.workers[0].flows.GetOrCreate(key, time.Now())
	for i := 0; i < cap(eng.workers[0].touch); i++ {
		eng.workers[0].touch <- key
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- eng.recvLoop(ctx)
	}()

	waitForCondition(t, func() bool { return ad.sendCount() == 1 })
	cancel()

	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("recvLoop error = %v, want context.Canceled", err)
	}
	if got := len(eng.workers[0].in); got != 0 {
		t.Fatalf("worker queue len = %d, want 0", got)
	}
}

func waitForCondition(t *testing.T, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}
