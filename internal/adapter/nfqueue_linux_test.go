//go:build linux

package adapter

import (
	"context"
	"encoding/binary"
	"errors"
	"net/netip"
	"testing"

	nfqueue "github.com/florianl/go-nfqueue"

	"fk-gov/internal/packet"
)

func TestNFQueueIfIndexPrefersOutDev(t *testing.T) {
	outDev := uint32(7)
	physOutDev := uint32(9)
	got := nfqueueIfIndex(nfqueue.Attribute{
		OutDev:     &outDev,
		PhysOutDev: &physOutDev,
	})
	if got != outDev {
		t.Fatalf("expected outdev=%d, got %d", outDev, got)
	}
}

func TestNFQueueIfIndexFallsBackToPhysOutDev(t *testing.T) {
	physOutDev := uint32(9)
	got := nfqueueIfIndex(nfqueue.Attribute{
		PhysOutDev: &physOutDev,
	})
	if got != physOutDev {
		t.Fatalf("expected physOutDev=%d, got %d", physOutDev, got)
	}
}

func TestNFQueuePacketVersion(t *testing.T) {
	ad := &NFQueueAdapter{}

	if got := ad.packetVersion(&packet.Packet{Meta: packet.Meta{IPVersion: packet.IPVersion6}}); got != packet.IPVersion6 {
		t.Fatalf("expected version from meta, got %d", got)
	}
	if got := ad.packetVersion(&packet.Packet{Data: []byte{0x45}}); got != packet.IPVersion4 {
		t.Fatalf("expected version 4 from data, got %d", got)
	}
	if got := ad.packetVersion(&packet.Packet{Data: []byte{0x60}}); got != packet.IPVersion6 {
		t.Fatalf("expected version 6 from data, got %d", got)
	}
}

func TestNFQueueCapturedSendRequiresQueueVerdict(t *testing.T) {
	ad := &NFQueueAdapter{}
	pkt := &packet.Packet{
		Source: packet.SourceCaptured,
		NFQID:  42,
		Meta:   packet.Meta{IPVersion: packet.IPVersion4},
		Data:   []byte{0x45},
	}

	err := ad.Send(context.Background(), pkt)
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("Send error = %v, want ErrNotImplemented", err)
	}
}

func TestNFQueueCapturedEmptySendStillRequiresQueueVerdict(t *testing.T) {
	ad := &NFQueueAdapter{}
	pkt := &packet.Packet{
		Source: packet.SourceCaptured,
		NFQID:  42,
		Meta:   packet.Meta{IPVersion: packet.IPVersion4},
	}

	err := ad.Send(context.Background(), pkt)
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("Send error = %v, want ErrNotImplemented", err)
	}
}

func TestNFQueueCapturedDropRequiresQueueVerdict(t *testing.T) {
	ad := &NFQueueAdapter{}
	pkt := &packet.Packet{
		Source: packet.SourceCaptured,
		NFQID:  42,
		Meta:   packet.Meta{IPVersion: packet.IPVersion6},
		Data:   []byte{0x60},
	}

	err := ad.Drop(context.Background(), pkt)
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("Drop error = %v, want ErrNotImplemented", err)
	}
}

func TestNFQueueInjectedUnknownVersionNoop(t *testing.T) {
	ad := &NFQueueAdapter{}
	pkt := &packet.Packet{
		Source: packet.SourceInjected,
		Data:   []byte{0x00},
	}

	if err := ad.Send(context.Background(), pkt); err != nil {
		t.Fatalf("Send error = %v, want nil", err)
	}
}

func TestNFQueueSendInjectedIPv4RequiresRawSocket(t *testing.T) {
	ad := &NFQueueAdapter{rawFD4: -1, rawFD6: -1}
	pkt := adapterTestIPv4TCPPacket([]byte("hello"))
	pkt.Source = packet.SourceInjected

	err := ad.Send(context.Background(), pkt)
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("Send error = %v, want ErrNotImplemented", err)
	}
}

func TestNFQueueSendInjectedIPv6RequiresRawSocket(t *testing.T) {
	ad := &NFQueueAdapter{rawFD4: -1, rawFD6: -1}
	pkt := adapterTestIPv6TCPPacket([]byte("hello"))
	pkt.Source = packet.SourceInjected

	err := ad.Send(context.Background(), pkt)
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("Send error = %v, want ErrNotImplemented", err)
	}
}

func TestNFQueueDropInjectedPacketNoop(t *testing.T) {
	ad := &NFQueueAdapter{}
	pkt := &packet.Packet{Source: packet.SourceInjected, Data: []byte{0x45}}

	if err := ad.Drop(context.Background(), pkt); err != nil {
		t.Fatalf("Drop error = %v, want nil", err)
	}
}

func TestNFQueueRecvContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ad := &NFQueueAdapter{
		recv: make(chan *packet.Packet),
		errs: make(chan error, 1),
		ctx:  context.Background(),
	}

	_, err := ad.Recv(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Recv error = %v, want context.Canceled", err)
	}
}

func TestNFQueueCalcChecksumsIPv4(t *testing.T) {
	ad := &NFQueueAdapter{}
	pkt := adapterTestIPv4TCPPacket([]byte("hello"))
	binary.BigEndian.PutUint16(pkt.Data[10:12], 0)
	binary.BigEndian.PutUint16(pkt.Data[pkt.Meta.IPHeaderLen+16:pkt.Meta.IPHeaderLen+18], 0)

	if err := ad.CalcChecksums(pkt); err != nil {
		t.Fatalf("CalcChecksums error = %v", err)
	}
	if got := binary.BigEndian.Uint16(pkt.Data[10:12]); got == 0 {
		t.Fatal("IPv4 checksum was not populated")
	}
	if got := binary.BigEndian.Uint16(pkt.Data[pkt.Meta.IPHeaderLen+16 : pkt.Meta.IPHeaderLen+18]); got == 0 {
		t.Fatal("TCP checksum was not populated")
	}
	if got := packet.IPv4Checksum(pkt.Data, pkt.Meta.IPHeaderLen); got != 0 {
		t.Fatalf("IPv4 checksum validation = 0x%04x, want 0", got)
	}
	if got := packet.TCPChecksumIPv4(pkt.Data, pkt.Meta.IPHeaderLen); got != 0 {
		t.Fatalf("TCP checksum validation = 0x%04x, want 0", got)
	}
}

func TestNFQueueCalcChecksumsIPv6(t *testing.T) {
	ad := &NFQueueAdapter{}
	pkt := adapterTestIPv6TCPPacket([]byte("hello"))
	binary.BigEndian.PutUint16(pkt.Data[pkt.Meta.IPHeaderLen+16:pkt.Meta.IPHeaderLen+18], 0)

	if err := ad.CalcChecksums(pkt); err != nil {
		t.Fatalf("CalcChecksums error = %v", err)
	}
	if got := binary.BigEndian.Uint16(pkt.Data[pkt.Meta.IPHeaderLen+16 : pkt.Meta.IPHeaderLen+18]); got == 0 {
		t.Fatal("TCP checksum was not populated")
	}
	if got := packet.TCPChecksumIPv6(pkt.Data, pkt.Meta.IPHeaderLen); got != 0 {
		t.Fatalf("TCP checksum validation = 0x%04x, want 0", got)
	}
}

func TestNFQueueCalcChecksumsMalformedNoop(t *testing.T) {
	ad := &NFQueueAdapter{}
	pkt := &packet.Packet{Data: []byte{0x45}}

	if err := ad.CalcChecksums(pkt); err != nil {
		t.Fatalf("CalcChecksums error = %v, want nil", err)
	}
}

func TestNFQueueRecvBufferSizeDefaults(t *testing.T) {
	if got := nfqueueRecvBufferSize(NFQueueOptions{}); got != defaultNFQueueRecvBuffer {
		t.Fatalf("recv buffer = %d, want default %d", got, defaultNFQueueRecvBuffer)
	}
	if got := nfqueueRecvBufferSize(NFQueueOptions{QueueMaxLen: 4096}); got != 4096 {
		t.Fatalf("recv buffer = %d, want queue maxlen", got)
	}
	if got := nfqueueRecvBufferSize(NFQueueOptions{QueueMaxLen: 4096, RecvBufferSize: 2048}); got != 2048 {
		t.Fatalf("recv buffer = %d, want explicit override", got)
	}
}

func TestNFQueueRecvBufferSizeCapsLargeValues(t *testing.T) {
	if got := nfqueueRecvBufferSize(NFQueueOptions{QueueMaxLen: MaxNFQueueRecvBuffer + 1}); got != MaxNFQueueRecvBuffer {
		t.Fatalf("recv buffer = %d, want cap %d", got, MaxNFQueueRecvBuffer)
	}
	if got := nfqueueRecvBufferSize(NFQueueOptions{RecvBufferSize: MaxNFQueueRecvBuffer + 1}); got != MaxNFQueueRecvBuffer {
		t.Fatalf("recv buffer = %d, want cap %d", got, MaxNFQueueRecvBuffer)
	}
}

func adapterTestIPv4TCPPacket(payload []byte) *packet.Packet {
	buf := make([]byte, 20+20+len(payload))
	buf[0] = 0x45
	binary.BigEndian.PutUint16(buf[2:4], uint16(len(buf)))
	buf[8] = 64
	buf[9] = 6
	src := netip.MustParseAddr("192.0.2.1").As4()
	dst := netip.MustParseAddr("198.51.100.2").As4()
	copy(buf[12:16], src[:])
	copy(buf[16:20], dst[:])
	binary.BigEndian.PutUint16(buf[20:22], 12345)
	binary.BigEndian.PutUint16(buf[22:24], 443)
	binary.BigEndian.PutUint32(buf[24:28], 0x01020304)
	buf[32] = 0x50
	buf[33] = packet.TCPFlagPSH | packet.TCPFlagACK
	binary.BigEndian.PutUint16(buf[34:36], 0xfaf0)
	copy(buf[40:], payload)

	pkt := &packet.Packet{Data: buf}
	if err := packet.DecodeTCP(pkt); err != nil {
		panic(err)
	}
	return pkt
}

func adapterTestIPv6TCPPacket(payload []byte) *packet.Packet {
	buf := make([]byte, 40+20+len(payload))
	buf[0] = 0x60
	binary.BigEndian.PutUint16(buf[4:6], uint16(20+len(payload)))
	buf[6] = 6
	buf[7] = 64
	src := netip.MustParseAddr("2001:db8::1").As16()
	dst := netip.MustParseAddr("2001:db8::2").As16()
	copy(buf[8:24], src[:])
	copy(buf[24:40], dst[:])
	binary.BigEndian.PutUint16(buf[40:42], 12345)
	binary.BigEndian.PutUint16(buf[42:44], 443)
	binary.BigEndian.PutUint32(buf[44:48], 0x01020304)
	buf[52] = 0x50
	buf[53] = packet.TCPFlagPSH | packet.TCPFlagACK
	binary.BigEndian.PutUint16(buf[54:56], 0xfaf0)
	copy(buf[60:], payload)

	pkt := &packet.Packet{Data: buf}
	if err := packet.DecodeTCP(pkt); err != nil {
		panic(err)
	}
	return pkt
}
