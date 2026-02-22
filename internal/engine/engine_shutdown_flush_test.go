package engine

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"fk-gov/internal/packet"
)

type flushBlockingAdapter struct {
	flushCalled       atomic.Bool
	flushHasDeadline  atomic.Bool
	closeCalled       atomic.Bool
	closeBeforeFlush  atomic.Bool
	recvErr           error
	sendCount         atomic.Int64
	flushWaitForCtx   bool
	closeWaitForFlush bool
}

func (a *flushBlockingAdapter) Recv(ctx context.Context) (*packet.Packet, error) {
	if a.recvErr != nil {
		return nil, a.recvErr
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func (a *flushBlockingAdapter) Send(ctx context.Context, pkt *packet.Packet) error {
	a.sendCount.Add(1)
	return nil
}

func (a *flushBlockingAdapter) Drop(ctx context.Context, pkt *packet.Packet) error {
	return nil
}

func (a *flushBlockingAdapter) CalcChecksums(pkt *packet.Packet) error {
	return nil
}

func (a *flushBlockingAdapter) Flush(ctx context.Context) error {
	a.flushCalled.Store(true)
	if _, ok := ctx.Deadline(); ok {
		a.flushHasDeadline.Store(true)
	}
	if a.flushWaitForCtx {
		<-ctx.Done()
		return ctx.Err()
	}
	return nil
}

func (a *flushBlockingAdapter) Close() error {
	if !a.flushCalled.Load() {
		a.closeBeforeFlush.Store(true)
	}
	a.closeCalled.Store(true)
	if a.closeWaitForFlush {
		// Best-effort small wait so Close doesn't race Flush assertions in tests
		time.Sleep(5 * time.Millisecond)
	}
	return nil
}

func TestEngineRun_IgnoresAdapterFlushDeadlineOnCancel(t *testing.T) {
	ad := &flushBlockingAdapter{flushWaitForCtx: true}
	cfg := DefaultConfig()
	cfg.WorkerCount = 1
	cfg.AdapterFlushTimeout = 20 * time.Millisecond

	eng := New(cfg, ad)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	err := eng.Run(ctx)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("engine stop took too long: %s", d)
	}
	if !ad.flushCalled.Load() {
		t.Fatalf("expected adapter.Flush to be called")
	}
	if !ad.flushHasDeadline.Load() {
		t.Fatalf("expected adapter.Flush ctx to have a deadline")
	}
	if !ad.closeCalled.Load() {
		t.Fatalf("expected adapter.Close to be called")
	}
	if ad.closeBeforeFlush.Load() {
		t.Fatalf("expected Flush to be called before Close")
	}
}

func TestEngineRun_ReturnsFlushErrorOnNonCancelStop(t *testing.T) {
	errBoom := errors.New("boom")
	ad := &flushBlockingAdapter{recvErr: errBoom, flushWaitForCtx: true}
	cfg := DefaultConfig()
	cfg.WorkerCount = 1
	cfg.AdapterFlushTimeout = 10 * time.Millisecond

	eng := New(cfg, ad)

	err := eng.Run(context.Background())
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, errBoom) {
		t.Fatalf("expected error to include recv error %v, got %v", errBoom, err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected error to include flush deadline exceeded, got %v", err)
	}
}

