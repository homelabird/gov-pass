package engine

import (
	"context"
	"testing"
	"time"

	"fk-gov/internal/flow"
	"fk-gov/internal/packet"
)

func TestWorkerGC_EnforcesCollectTimeout(t *testing.T) {
	cfg := DefaultConfig()
	cfg.CollectTimeout = 20 * time.Millisecond
	cfg.FlowIdleTimeout = time.Hour

	ad := &recordingAdapter{}
	w := newWorker(0, cfg, ad, newStats())

	key := flow.Key{SrcPort: 1234, DstPort: 443, Proto: 6}
	st := w.flows.GetOrCreate(key, time.Now())
	pkt := &packet.Packet{Data: []byte{1, 2, 3}}
	st.State = flow.StateCollecting
	st.CollectStart = time.Now().Add(-200 * time.Millisecond)
	st.LastActive = time.Now()
	st.Template = pkt
	st.HeldPackets = []*packet.Packet{pkt}
	w.heldBytes = int64(len(pkt.Data))

	if err := w.gc(context.Background()); err != nil {
		t.Fatalf("gc returned error: %v", err)
	}
	if got, want := len(ad.sends), 1; got != want {
		t.Fatalf("send count = %d, want %d", got, want)
	}
	if ad.sends[0] != pkt {
		t.Fatalf("gc sent %p, want %p", ad.sends[0], pkt)
	}
	if st.State != flow.StatePassThrough {
		t.Fatalf("state = %v, want pass-through", st.State)
	}
	if len(st.HeldPackets) != 0 || st.Template != nil || st.Reassembler != nil {
		t.Fatalf("expected collecting state cleared, got %+v", st)
	}
	snap := w.stats.snapshot()
	if got := snap.FailOpen["collect_timeout"]; got != 1 {
		t.Fatalf("collect_timeout fail-open count = %d, want 1", got)
	}
}
