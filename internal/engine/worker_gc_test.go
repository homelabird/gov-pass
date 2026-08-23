package engine

import (
	"context"
	"testing"
	"time"

	"fk-gov/internal/packet"
)

func TestWorkerRun_EnforcesCollectTimeoutBeforeIdleGC(t *testing.T) {
	cfg := DefaultConfig()
	cfg.CollectTimeout = 20 * time.Millisecond
	cfg.GCInterval = time.Hour

	ad := &recordingAdapter{}
	w := newWorker(0, cfg, ad, newStats())
	pkt := buildTestIPv4Packet(t, 12345, 1, packet.TCPFlagPSH|packet.TCPFlagACK, []byte{0x16})
	if err := w.handlePacket(context.Background(), pkt); err != nil {
		t.Fatalf("handlePacket failed: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- w.run(ctx) }()

	deadline := time.Now().Add(500 * time.Millisecond)
	for w.stats.snapshot().FailOpen[string(failOpenReasonCollectTimeout)] == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	if err := <-errCh; err != context.Canceled {
		t.Fatalf("worker run error = %v, want context canceled", err)
	}
	if got := w.stats.snapshot().FailOpen[string(failOpenReasonCollectTimeout)]; got != 1 {
		t.Fatalf("collect timeout count = %d, want 1", got)
	}
	if got := len(ad.sends); got != 1 || ad.sends[0] != pkt {
		t.Fatalf("fail-open sends = %#v, want original packet", ad.sends)
	}
}
