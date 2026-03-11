package engine

import (
	"context"
	"encoding/binary"
	"net/netip"
	"testing"
	"time"

	"fk-gov/internal/flow"
	"fk-gov/internal/packet"
	"fk-gov/internal/reassembly"
)

func TestWorkerStats_CountsSplitSuccess(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SplitMode = SplitModeImmediate
	cfg.SplitChunk = 3

	ad := &recordingAdapter{}
	st := newStats()
	w := newWorker(0, cfg, ad, st)

	pkt := buildTestIPv4Packet(t, 12345, 0x01020304, packet.TCPFlagPSH|packet.TCPFlagACK, []byte("abcdef"))
	state := &flow.FlowState{
		State:             flow.StateCollecting,
		BaseSeq:           pkt.Meta.Seq,
		LastActive:        time.Now(),
		Template:          pkt,
		HeldPackets:       []*packet.Packet{pkt},
		Reassembler:       reassembly.New(pkt.Meta.Seq, 4096),
		PolicyResolved:    true,
		SplitModeValue:    uint8(SplitModeImmediate),
		SplitChunk:        cfg.SplitChunk,
		MaxSegmentPayload: cfg.MaxSegmentPayload,
	}
	if err := state.Reassembler.Push(pkt.Meta.Seq, pkt.Payload()); err != nil {
		t.Fatalf("Push failed: %v", err)
	}
	w.heldBytes = int64(len(pkt.Data))
	w.reassemblyBytes = int64(state.Reassembler.TotalBytes())

	if err := w.injectWindow(context.Background(), flow.Key{}, state, len(pkt.Payload())); err != nil {
		t.Fatalf("injectWindow failed: %v", err)
	}

	snap := st.snapshot()
	if got := snap.SplitsOK; got != 1 {
		t.Fatalf("expected splits_ok=1, got %d", got)
	}
	if len(snap.FailOpen) != 0 {
		t.Fatalf("expected no fail-open counters, got %+v", snap.FailOpen)
	}
}

func TestWorkerStats_CountsTLSMismatchFailOpen(t *testing.T) {
	cfg := DefaultConfig()

	ad := &recordingAdapter{}
	st := newStats()
	w := newWorker(0, cfg, ad, st)

	payload := []byte{0x15, 0x03, 0x03, 0x00, 0x05, 0x01, 0x00, 0x00, 0x00, 0x00}
	pkt := buildTestIPv4Packet(t, 12346, 0x02030405, packet.TCPFlagPSH|packet.TCPFlagACK, payload)
	state := &flow.FlowState{
		State:       flow.StateCollecting,
		BaseSeq:     pkt.Meta.Seq,
		LastActive:  time.Now(),
		Template:    pkt,
		HeldPackets: []*packet.Packet{pkt},
		Reassembler: reassembly.New(pkt.Meta.Seq, 4096),
	}
	if err := state.Reassembler.Push(pkt.Meta.Seq, pkt.Payload()); err != nil {
		t.Fatalf("Push failed: %v", err)
	}
	w.heldBytes = int64(len(pkt.Data))
	w.reassemblyBytes = int64(state.Reassembler.TotalBytes())

	if err := w.trySplitTLSHello(context.Background(), flow.Key{}, state); err != nil {
		t.Fatalf("trySplitTLSHello failed: %v", err)
	}

	snap := st.snapshot()
	if got := snap.FailOpen["tls_mismatch"]; got != 1 {
		t.Fatalf("expected tls_mismatch fail-open count=1, got %d", got)
	}
	if got := len(ad.sends); got != 1 {
		t.Fatalf("expected 1 fail-open send, got %d", got)
	}
}

func TestWorkerStats_CountsFlowLimitPressure(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxFlowsPerWorker = 1

	ad := &recordingAdapter{}
	st := newStats()
	w := newWorker(0, cfg, ad, st)

	existingKey := flow.Key{
		IPVersion: 4,
		SrcPort:   1000,
		DstPort:   443,
		Proto:     6,
	}
	w.flows.GetOrCreate(existingKey, time.Now())

	pkt := buildTestIPv4Packet(t, 12347, 0x03040506, packet.TCPFlagPSH|packet.TCPFlagACK, []byte("hello"))
	if err := w.handlePacket(context.Background(), pkt); err != nil {
		t.Fatalf("handlePacket failed: %v", err)
	}

	snap := st.snapshot()
	if got := snap.Pressure["flow_limit"]; got != 1 {
		t.Fatalf("expected flow_limit pressure count=1, got %d", got)
	}
	if got := len(ad.sends); got != 1 {
		t.Fatalf("expected packet to be passed through once, got %d sends", got)
	}
}

func TestStatsSnapshotString_SortsCounterNames(t *testing.T) {
	snap := StatsSnapshot{
		SplitsOK: 2,
		FailOpen: map[string]uint64{
			"tls_mismatch":    1,
			"collect_timeout": 3,
		},
		Pressure: map[string]uint64{
			"flow_limit": 4,
		},
	}

	got := snap.String()
	want := "splits_ok=2 fail_open={collect_timeout:3,tls_mismatch:1} pressure={flow_limit:4}"
	if got != want {
		t.Fatalf("unexpected stats string: got %q want %q", got, want)
	}
}

func buildTestIPv4Packet(t *testing.T, srcPort uint16, seq uint32, flags uint8, payload []byte) *packet.Packet {
	t.Helper()

	buf := make([]byte, 20+20+len(payload))
	buf[0] = 0x45
	binary.BigEndian.PutUint16(buf[2:4], uint16(len(buf)))
	buf[8] = 64
	buf[9] = 6

	src := netip.MustParseAddr("192.0.2.10").As4()
	dst := netip.MustParseAddr("198.51.100.20").As4()
	copy(buf[12:16], src[:])
	copy(buf[16:20], dst[:])

	binary.BigEndian.PutUint16(buf[20:22], srcPort)
	binary.BigEndian.PutUint16(buf[22:24], 443)
	binary.BigEndian.PutUint32(buf[24:28], seq)
	buf[32] = 0x50
	buf[33] = flags
	binary.BigEndian.PutUint16(buf[34:36], 0xfaf0)
	copy(buf[40:], payload)

	pkt := &packet.Packet{
		Data:   buf,
		Source: packet.SourceCaptured,
	}
	if err := packet.DecodeTCP(pkt); err != nil {
		t.Fatalf("DecodeTCP failed: %v", err)
	}
	return pkt
}
