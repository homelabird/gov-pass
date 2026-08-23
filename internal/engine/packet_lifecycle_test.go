package engine

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"fk-gov/internal/adapter"
	"fk-gov/internal/flow"
	"fk-gov/internal/packet"
)

type failOnSendAdapter struct {
	sendCount int
	failAt    int
}

var _ adapter.Adapter = (*failOnSendAdapter)(nil)

func (a *failOnSendAdapter) Recv(ctx context.Context) (*packet.Packet, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, context.Canceled
}

func (a *failOnSendAdapter) Send(ctx context.Context, pkt *packet.Packet) error {
	a.sendCount++
	if a.failAt > 0 && a.sendCount == a.failAt {
		return errors.New("send failed")
	}
	return nil
}

func (a *failOnSendAdapter) Drop(ctx context.Context, pkt *packet.Packet) error {
	return nil
}

func (a *failOnSendAdapter) CalcChecksums(pkt *packet.Packet) error {
	return nil
}

func (a *failOnSendAdapter) Flush(ctx context.Context) error {
	return nil
}

func (a *failOnSendAdapter) Close() error {
	return nil
}

func TestFailOpen_RemovesReleasedPacketsFromFlowState(t *testing.T) {
	cfg := DefaultConfig()
	ad := &failOnSendAdapter{failAt: 2}
	w := newWorker(0, cfg, ad, newStats())

	p1 := pooledCapturedPacket([]byte{1, 2, 3})
	p2 := pooledCapturedPacket([]byte{4, 5, 6})

	st := &flow.FlowState{
		State:       flow.StateCollecting,
		LastActive:  time.Now(),
		Template:    p1,
		HeldPackets: []*packet.Packet{p1, p2},
	}
	w.heldBytes = packetMemoryBytes(p1) + packetMemoryBytes(p2)

	err := w.failOpen(context.Background(), flow.Key{}, st)
	if err == nil || err.Error() != "send failed" {
		t.Fatalf("failOpen error = %v, want send failed", err)
	}
	if got, want := ad.sendCount, 2; got != want {
		t.Fatalf("send count = %d, want %d", got, want)
	}
	if len(st.HeldPackets) != 1 || st.HeldPackets[0] != p2 {
		t.Fatalf("remaining held packets = %#v, want only second packet", st.HeldPackets)
	}
	if st.Template != p2 {
		t.Fatalf("template = %p, want %p", st.Template, p2)
	}
	if got, want := w.heldBytes, packetMemoryBytes(p2); got != want {
		t.Fatalf("heldBytes = %d, want %d", got, want)
	}
	if p1.Data == nil {
		t.Fatal("released packet data should remain readable for accounting")
	}
}

func pooledCapturedPacket(data []byte) *packet.Packet {
	backing := append([]byte(nil), data...)
	pool := &sync.Pool{}
	pkt := &packet.Packet{
		Data:   backing[:len(data)],
		Source: packet.SourceCaptured,
	}
	pkt.SetDataPool(pool, backing)
	return pkt
}
