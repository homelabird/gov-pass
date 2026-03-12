package engine

import (
	"context"
	"encoding/binary"
	"errors"
	"net/netip"
	"testing"
	"time"

	"fk-gov/internal/flow"
	"fk-gov/internal/packet"
	"fk-gov/internal/reassembly"
)

type injectScriptAdapter struct {
	sends            []*packet.Packet
	drops            []*packet.Packet
	injectedSendFail int
	injectedSends    int
	dropErr          error
}

func (a *injectScriptAdapter) Recv(ctx context.Context) (*packet.Packet, error) {
	return nil, context.Canceled
}

func (a *injectScriptAdapter) Send(ctx context.Context, pkt *packet.Packet) error {
	if pkt != nil && pkt.Source == packet.SourceInjected {
		a.injectedSends++
		if a.injectedSendFail > 0 && a.injectedSends == a.injectedSendFail {
			return errors.New("send failed")
		}
	}
	a.sends = append(a.sends, pkt)
	return nil
}

func (a *injectScriptAdapter) Drop(ctx context.Context, pkt *packet.Packet) error {
	a.drops = append(a.drops, pkt)
	if a.dropErr != nil {
		return a.dropErr
	}
	return nil
}

func (a *injectScriptAdapter) CalcChecksums(pkt *packet.Packet) error { return nil }
func (a *injectScriptAdapter) Flush(ctx context.Context) error        { return nil }
func (a *injectScriptAdapter) Close() error                           { return nil }

func TestBuildPacket_PreservesIfIndex(t *testing.T) {
	tpl := &packet.Packet{
		Data:    testIPv6PacketWithPayload(false, []byte("abcdef")),
		IfIndex: 7,
	}
	if err := packet.DecodeTCP(tpl); err != nil {
		t.Fatalf("DecodeTCP failed: %v", err)
	}

	got, err := buildPacket(tpl, tpl.Meta.Seq, []byte("xyz"), tpl.Meta.Flags, nil)
	if err != nil {
		t.Fatalf("buildPacket failed: %v", err)
	}
	if got.IfIndex != tpl.IfIndex {
		t.Fatalf("expected IfIndex=%d, got %d", tpl.IfIndex, got.IfIndex)
	}
}

func TestInjectWindow_FailOpensOnIPv6ExtensionHeaders(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SplitChunk = 3
	ad := &recordingAdapter{}
	w := newWorker(0, cfg, ad, newStats())

	tpl := &packet.Packet{
		Data:    testIPv6PacketWithPayload(true, []byte("abcdef")),
		Source:  packet.SourceCaptured,
		NFQID:   42,
		IfIndex: 9,
	}
	if err := packet.DecodeTCP(tpl); err != nil {
		t.Fatalf("DecodeTCP failed: %v", err)
	}

	st := &flow.FlowState{
		State:             flow.StateCollecting,
		BaseSeq:           tpl.Meta.Seq,
		LastActive:        time.Now(),
		Template:          tpl,
		HeldPackets:       []*packet.Packet{tpl},
		Reassembler:       reassembly.New(tpl.Meta.Seq, 4096),
		PolicyResolved:    true,
		SplitModeValue:    uint8(SplitModeImmediate),
		SplitChunk:        cfg.SplitChunk,
		MaxSegmentPayload: cfg.MaxSegmentPayload,
	}
	if err := st.Reassembler.Push(tpl.Meta.Seq, tpl.Payload()); err != nil {
		t.Fatalf("Push failed: %v", err)
	}
	w.heldBytes = int64(len(tpl.Data))
	w.reassemblyBytes = int64(st.Reassembler.TotalBytes())

	if err := w.injectWindow(context.Background(), flow.Key{}, st, 3); err != nil {
		t.Fatalf("injectWindow failed: %v", err)
	}
	if got := len(ad.sends); got != 1 {
		t.Fatalf("expected 1 fail-open send, got %d", got)
	}
	if ad.sends[0] != tpl {
		t.Fatal("expected original packet to be fail-opened")
	}
	if st.State != flow.StatePassThrough {
		t.Fatalf("expected state passthrough, got %v", st.State)
	}
	if st.Template != nil || st.Reassembler != nil || len(st.HeldPackets) != 0 {
		t.Fatalf("expected collecting state cleared, got %+v", st)
	}
	stats := w.stats.snapshot()
	if got := stats.FailOpen["ipv6_extension_headers"]; got != 1 {
		t.Fatalf("expected ipv6_extension_headers fail-open count=1, got %d", got)
	}
}

func TestHandlePacket_AllowsConfiguredHeldPacketLimit(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxHeldPackets = 1
	ad := &recordingAdapter{}
	w := newWorker(0, cfg, ad, newStats())

	pkt := buildTestIPv4Packet(t, 12345, 0x01020304, packet.TCPFlagPSH|packet.TCPFlagACK, []byte{1, 2, 3})
	key := flow.KeyFromMeta(pkt.Meta)

	if err := w.handlePacket(context.Background(), pkt); err != nil {
		t.Fatalf("handlePacket failed: %v", err)
	}

	st, ok := w.flows.Get(key)
	if !ok {
		t.Fatal("expected flow state to be created")
	}
	if st.State != flow.StateCollecting {
		t.Fatalf("expected collecting state, got %v", st.State)
	}
	if len(st.HeldPackets) != 1 || st.HeldPackets[0] != pkt {
		t.Fatalf("held packets = %#v, want first packet retained", st.HeldPackets)
	}
	if got := len(ad.sends); got != 0 {
		t.Fatalf("expected no fail-open sends, got %d", got)
	}
}

func TestInjectWindow_DropFailureDoesNotLeaveHeldOriginalsForShutdown(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SplitChunk = 3
	ad := &injectScriptAdapter{dropErr: errors.New("drop failed")}
	w := newWorker(0, cfg, ad, newStats())

	payload := []byte("abcdefghij")
	tpl := buildTestIPv4Packet(t, 12345, 0x01020304, packet.TCPFlagPSH|packet.TCPFlagACK, payload)
	st := &flow.FlowState{
		State:             flow.StateCollecting,
		BaseSeq:           tpl.Meta.Seq,
		LastActive:        time.Now(),
		Template:          tpl,
		HeldPackets:       []*packet.Packet{tpl},
		Reassembler:       reassembly.New(tpl.Meta.Seq, 4096),
		PolicyResolved:    true,
		SplitModeValue:    uint8(SplitModeImmediate),
		SplitChunk:        cfg.SplitChunk,
		MaxSegmentPayload: cfg.MaxSegmentPayload,
	}
	if err := st.Reassembler.Push(tpl.Meta.Seq, tpl.Payload()); err != nil {
		t.Fatalf("Push failed: %v", err)
	}
	w.heldBytes = int64(len(tpl.Data))
	w.reassemblyBytes = int64(st.Reassembler.TotalBytes())

	if err := w.injectWindow(context.Background(), flow.Key{}, st, len(payload)); err != nil {
		t.Fatalf("injectWindow failed: %v", err)
	}
	if st.State != flow.StateInjected {
		t.Fatalf("state = %v, want injected", st.State)
	}
	if len(st.HeldPackets) != 0 || st.Template != nil || st.Reassembler != nil {
		t.Fatalf("expected collecting state cleared, got %+v", st)
	}
	sendCount := len(ad.sends)
	if sendCount < 2 {
		t.Fatalf("expected split segments to be sent, got %d", sendCount)
	}
	if err := w.shutdownFailOpen(context.Background()); err != nil {
		t.Fatalf("shutdownFailOpen failed: %v", err)
	}
	if got := len(ad.sends); got != sendCount {
		t.Fatalf("shutdown reinjected originals: sends %d -> %d", sendCount, got)
	}
}

func TestInjectWindow_FirstSendFailureFailOpensOriginals(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SplitChunk = 3
	ad := &injectScriptAdapter{injectedSendFail: 1}
	w := newWorker(0, cfg, ad, newStats())

	payload := []byte("abcdefghij")
	tpl := buildTestIPv4Packet(t, 12345, 0x01020304, packet.TCPFlagPSH|packet.TCPFlagACK, payload)
	st := &flow.FlowState{
		State:             flow.StateCollecting,
		BaseSeq:           tpl.Meta.Seq,
		LastActive:        time.Now(),
		Template:          tpl,
		HeldPackets:       []*packet.Packet{tpl},
		Reassembler:       reassembly.New(tpl.Meta.Seq, 4096),
		PolicyResolved:    true,
		SplitModeValue:    uint8(SplitModeImmediate),
		SplitChunk:        cfg.SplitChunk,
		MaxSegmentPayload: cfg.MaxSegmentPayload,
	}
	if err := st.Reassembler.Push(tpl.Meta.Seq, tpl.Payload()); err != nil {
		t.Fatalf("Push failed: %v", err)
	}
	w.heldBytes = int64(len(tpl.Data))
	w.reassemblyBytes = int64(st.Reassembler.TotalBytes())

	if err := w.injectWindow(context.Background(), flow.Key{}, st, len(payload)); err != nil {
		t.Fatalf("injectWindow failed: %v", err)
	}
	if st.State != flow.StatePassThrough {
		t.Fatalf("state = %v, want pass-through", st.State)
	}
	if len(ad.sends) != 1 || ad.sends[0] != tpl {
		t.Fatalf("expected original packet to be fail-opened, got %#v", ad.sends)
	}
	if len(st.HeldPackets) != 0 || st.Template != nil || st.Reassembler != nil {
		t.Fatalf("expected collecting state cleared, got %+v", st)
	}
}

func testIPv6PacketWithPayload(withDestOptions bool, payload []byte) []byte {
	ipHeaderLen := 40
	nextHeader := byte(6)
	if withDestOptions {
		ipHeaderLen += 8
		nextHeader = 60
	}

	buf := make([]byte, ipHeaderLen+20+len(payload))
	buf[0] = 0x60
	binary.BigEndian.PutUint16(buf[4:6], uint16(len(buf)-40))
	buf[6] = nextHeader
	buf[7] = 64

	src := netip.MustParseAddr("2001:db8::1").As16()
	dst := netip.MustParseAddr("2001:db8::2").As16()
	copy(buf[8:24], src[:])
	copy(buf[24:40], dst[:])

	tcpStart := 40
	if withDestOptions {
		buf[40] = 6
		buf[41] = 0
		tcpStart = 48
	}

	binary.BigEndian.PutUint16(buf[tcpStart:tcpStart+2], 12345)
	binary.BigEndian.PutUint16(buf[tcpStart+2:tcpStart+4], 443)
	binary.BigEndian.PutUint32(buf[tcpStart+4:tcpStart+8], 0x01020304)
	buf[tcpStart+12] = 0x50
	buf[tcpStart+13] = packet.TCPFlagPSH | packet.TCPFlagACK
	binary.BigEndian.PutUint16(buf[tcpStart+14:tcpStart+16], 0xfaf0)
	copy(buf[tcpStart+20:], payload)

	return buf
}
