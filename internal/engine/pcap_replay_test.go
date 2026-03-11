package engine

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"fk-gov/internal/packet"
)

const (
	pcapMagicLE         = 0xa1b2c3d4
	pcapVersionMajor    = 2
	pcapVersionMinor    = 4
	pcapLinkTypeEther   = 1
	etherTypeIPv4       = 0x0800
	etherHeaderLen      = 14
	pcapGlobalHeaderLen = 24
	pcapPacketHeaderLen = 16
)

type replayAdapter struct {
	mu     sync.Mutex
	pkts   []*packet.Packet
	next   int
	sends  []*packet.Packet
	drops  []*packet.Packet
	calcs  int
	closed bool
}

func newReplayAdapter(pkts []*packet.Packet) *replayAdapter {
	return &replayAdapter{pkts: pkts}
}

func (a *replayAdapter) Recv(ctx context.Context) (*packet.Packet, error) {
	a.mu.Lock()
	if a.next >= len(a.pkts) {
		a.mu.Unlock()
		<-ctx.Done()
		return nil, ctx.Err()
	}
	pkt := a.pkts[a.next]
	a.next++
	a.mu.Unlock()
	return pkt, nil
}

func (a *replayAdapter) Send(ctx context.Context, pkt *packet.Packet) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sends = append(a.sends, pkt)
	return nil
}

func (a *replayAdapter) Drop(ctx context.Context, pkt *packet.Packet) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.drops = append(a.drops, pkt)
	return nil
}

func (a *replayAdapter) CalcChecksums(pkt *packet.Packet) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calcs++
	return nil
}

func (a *replayAdapter) Flush(ctx context.Context) error {
	return nil
}

func (a *replayAdapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.closed = true
	return nil
}

func (a *replayAdapter) snapshot() (sends []*packet.Packet, drops []*packet.Packet, calcs int, closed bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	sends = append([]*packet.Packet(nil), a.sends...)
	drops = append([]*packet.Packet(nil), a.drops...)
	return sends, drops, a.calcs, a.closed
}

func TestPcapReplay_ClientHelloSplitPreservesPayload(t *testing.T) {
	cfg := DefaultConfig()
	cfg.WorkerCount = 1
	cfg.SplitMode = SplitModeTLSHello
	cfg.SplitChunk = 5

	record := buildClientHelloRecordForReplay("www.example.com")
	raw := buildClassicPcapEthernet(
		t,
		splitClientHelloPackets(t, 12002, 0x03040506, record),
	)
	replayPkts := mustParseClassicPcapEthernetPackets(t, raw)
	ad := newReplayAdapter(replayPkts)
	eng := New(cfg, ad)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- eng.Run(ctx)
	}()

	waitForSplit(t, eng, ad, len(replayPkts))
	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("engine run failed: %v", err)
	}
	sends, drops, _, closed := ad.snapshot()
	if !closed {
		t.Fatal("expected adapter to be closed")
	}
	if got := len(drops); got != len(replayPkts) {
		t.Fatalf("expected %d dropped originals, got %d", len(replayPkts), got)
	}
	if got := len(sends); got < 2 {
		t.Fatalf("expected at least 2 injected segments, got %d", got)
	}
	if got := len(decodedPayload(t, sends[0])); got != cfg.SplitChunk {
		t.Fatalf("expected first injected payload len=%d, got %d", cfg.SplitChunk, got)
	}
	if got := joinPayloads(sends); !bytes.Equal(got, record) {
		t.Fatalf("replayed payload mismatch: got %d bytes want %d", len(got), len(record))
	}

	snap := eng.Stats()
	if got := snap.SplitsOK; got != 1 {
		t.Fatalf("expected splits_ok=1, got %d", got)
	}
	if len(snap.FailOpen) != 0 {
		t.Fatalf("expected no fail-open counters, got %+v", snap.FailOpen)
	}
}

func TestPcapReplay_SNIPolicyOverridesSplitChunk(t *testing.T) {
	cfg := DefaultConfig()
	cfg.WorkerCount = 1
	cfg.SplitMode = SplitModeTLSHello
	cfg.SplitChunk = 5
	cfg.Policies = []Policy{{
		SNISuffixes:   []string{"example.com"},
		HasSplitChunk: true,
		SplitChunk:    9,
	}}

	record := buildClientHelloRecordForReplay("api.example.com")
	raw := buildClassicPcapEthernet(
		t,
		splitClientHelloPackets(t, 12003, 0x04050607, record),
	)
	replayPkts := mustParseClassicPcapEthernetPackets(t, raw)
	ad := newReplayAdapter(replayPkts)
	eng := New(cfg, ad)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- eng.Run(ctx)
	}()

	waitForSplit(t, eng, ad, len(replayPkts))
	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("engine run failed: %v", err)
	}
	sends, _, _, _ := ad.snapshot()
	if got := len(sends); got < 2 {
		t.Fatalf("expected at least 2 injected segments, got %d", got)
	}
	if got := len(decodedPayload(t, sends[0])); got != 9 {
		t.Fatalf("expected policy split_chunk=9, got first payload len=%d", got)
	}
	if got := joinPayloads(sends); !bytes.Equal(got, record) {
		t.Fatalf("replayed payload mismatch: got %d bytes want %d", len(got), len(record))
	}
}

func waitForSplit(t *testing.T, eng *Engine, ad *replayAdapter, wantDrops int) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline:
			sends, drops, _, _ := ad.snapshot()
			t.Fatalf("timed out waiting for split: stats=%s sends=%d drops=%d", eng.Stats(), len(sends), len(drops))
		case <-tick.C:
			sends, drops, _, _ := ad.snapshot()
			if eng.Stats().SplitsOK >= 1 && len(drops) >= wantDrops && len(sends) >= 2 {
				return
			}
		}
	}
}

func buildClassicPcapEthernet(t *testing.T, pkts []*packet.Packet) []byte {
	t.Helper()

	buf := make([]byte, 0, pcapGlobalHeaderLen+len(pkts)*(pcapPacketHeaderLen+etherHeaderLen+128))
	buf = binary.LittleEndian.AppendUint32(buf, pcapMagicLE)
	buf = binary.LittleEndian.AppendUint16(buf, pcapVersionMajor)
	buf = binary.LittleEndian.AppendUint16(buf, pcapVersionMinor)
	buf = binary.LittleEndian.AppendUint32(buf, 0)
	buf = binary.LittleEndian.AppendUint32(buf, 0)
	buf = binary.LittleEndian.AppendUint32(buf, 65535)
	buf = binary.LittleEndian.AppendUint32(buf, pcapLinkTypeEther)

	for i, pkt := range pkts {
		frame := ethernetFrameForPacket(t, pkt)
		buf = binary.LittleEndian.AppendUint32(buf, uint32(1700000000+i))
		buf = binary.LittleEndian.AppendUint32(buf, 0)
		buf = binary.LittleEndian.AppendUint32(buf, uint32(len(frame)))
		buf = binary.LittleEndian.AppendUint32(buf, uint32(len(frame)))
		buf = append(buf, frame...)
	}
	return buf
}

func ethernetFrameForPacket(t *testing.T, pkt *packet.Packet) []byte {
	t.Helper()
	if pkt == nil || len(pkt.Data) == 0 {
		t.Fatal("cannot encode empty packet")
	}
	ethType := uint16(0)
	switch packet.Version(pkt.Data) {
	case packet.IPVersion4:
		ethType = etherTypeIPv4
	default:
		t.Fatalf("unsupported packet version in pcap fixture: %d", packet.Version(pkt.Data))
	}

	frame := make([]byte, etherHeaderLen+len(pkt.Data))
	copy(frame[0:6], []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55})
	copy(frame[6:12], []byte{0x66, 0x77, 0x88, 0x99, 0xaa, 0xbb})
	binary.BigEndian.PutUint16(frame[12:14], ethType)
	copy(frame[14:], pkt.Data)
	return frame
}

func mustParseClassicPcapEthernetPackets(t *testing.T, raw []byte) []*packet.Packet {
	t.Helper()
	pkts, err := parseClassicPcapEthernetPackets(raw)
	if err != nil {
		t.Fatalf("parseClassicPcapEthernetPackets failed: %v", err)
	}
	return pkts
}

func parseClassicPcapEthernetPackets(raw []byte) ([]*packet.Packet, error) {
	if len(raw) < pcapGlobalHeaderLen {
		return nil, errors.New("pcap too short")
	}

	magic := binary.LittleEndian.Uint32(raw[0:4])
	if magic != pcapMagicLE {
		return nil, fmt.Errorf("unsupported pcap magic: 0x%08x", magic)
	}
	linkType := binary.LittleEndian.Uint32(raw[20:24])
	if linkType != pcapLinkTypeEther {
		return nil, fmt.Errorf("unsupported pcap link type: %d", linkType)
	}

	var pkts []*packet.Packet
	for off := pcapGlobalHeaderLen; off < len(raw); {
		if len(raw)-off < pcapPacketHeaderLen {
			return nil, errors.New("truncated pcap packet header")
		}
		inclLen := int(binary.LittleEndian.Uint32(raw[off+8 : off+12]))
		off += pcapPacketHeaderLen
		if inclLen < etherHeaderLen || len(raw)-off < inclLen {
			return nil, errors.New("truncated pcap packet payload")
		}

		frame := raw[off : off+inclLen]
		off += inclLen
		ethType := binary.BigEndian.Uint16(frame[12:14])
		if ethType != etherTypeIPv4 {
			continue
		}

		pkt := &packet.Packet{
			Data:   append([]byte(nil), frame[etherHeaderLen:]...),
			Source: packet.SourceCaptured,
		}
		if err := packet.DecodeTCP(pkt); err != nil {
			return nil, fmt.Errorf("decode replay packet: %w", err)
		}
		pkts = append(pkts, pkt)
	}
	if len(pkts) == 0 {
		return nil, errors.New("pcap contained no replayable tcp packets")
	}
	return pkts, nil
}
