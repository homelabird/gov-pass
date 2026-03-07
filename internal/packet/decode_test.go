package packet

import (
	"encoding/binary"
	"errors"
	"net/netip"
	"testing"
)

func TestDecodeTCP_IPv4(t *testing.T) {
	pkt := &Packet{Data: testIPv4TCPPacket()}
	if err := DecodeTCP(pkt); err != nil {
		t.Fatalf("DecodeTCP failed: %v", err)
	}
	if pkt.Meta.IPVersion != IPVersion4 {
		t.Fatalf("unexpected ip version: %d", pkt.Meta.IPVersion)
	}
	if pkt.Meta.IPHeaderLen != 20 || pkt.Meta.TCPHeaderLen != 20 || pkt.Meta.PayloadOffset != 40 {
		t.Fatalf("unexpected offsets: %+v", pkt.Meta)
	}
	if got := pkt.Meta.SrcAddr(); got != netip.MustParseAddr("192.0.2.1") {
		t.Fatalf("unexpected src addr: %s", got)
	}
	if got := pkt.Meta.DstAddr(); got != netip.MustParseAddr("198.51.100.2") {
		t.Fatalf("unexpected dst addr: %s", got)
	}
}

func TestDecodeTCP_IPv6(t *testing.T) {
	pkt := &Packet{Data: testIPv6TCPPacket()}
	if err := DecodeTCP(pkt); err != nil {
		t.Fatalf("DecodeTCP failed: %v", err)
	}
	if pkt.Meta.IPVersion != IPVersion6 {
		t.Fatalf("unexpected ip version: %d", pkt.Meta.IPVersion)
	}
	if pkt.Meta.IPHeaderLen != 40 || pkt.Meta.TCPHeaderLen != 20 || pkt.Meta.PayloadOffset != 60 {
		t.Fatalf("unexpected offsets: %+v", pkt.Meta)
	}
	if got := pkt.Meta.SrcAddr(); got != netip.MustParseAddr("2001:db8::1") {
		t.Fatalf("unexpected src addr: %s", got)
	}
	if got := pkt.Meta.DstAddr(); got != netip.MustParseAddr("2001:db8::2") {
		t.Fatalf("unexpected dst addr: %s", got)
	}
}

func TestDecodeTCP_IPv6DestinationOptions(t *testing.T) {
	pkt := &Packet{Data: testIPv6TCPPacketWithDestOptions()}
	if err := DecodeTCP(pkt); err != nil {
		t.Fatalf("DecodeTCP failed: %v", err)
	}
	if pkt.Meta.IPVersion != IPVersion6 {
		t.Fatalf("unexpected ip version: %d", pkt.Meta.IPVersion)
	}
	if pkt.Meta.IPHeaderLen != 48 || pkt.Meta.TCPHeaderLen != 20 || pkt.Meta.PayloadOffset != 68 {
		t.Fatalf("unexpected offsets: %+v", pkt.Meta)
	}
}

func TestDecodeTCP_IPv6Fragmented(t *testing.T) {
	pkt := &Packet{Data: testIPv6TCPPacketWithFragment(true)}
	err := DecodeTCP(pkt)
	if !errors.Is(err, ErrIPv6Fragment) {
		t.Fatalf("expected ErrIPv6Fragment, got %v", err)
	}
}

func testIPv6TCPPacket() []byte {
	buf := make([]byte, 60)
	buf[0] = 0x60
	binary.BigEndian.PutUint16(buf[4:6], 20)
	buf[6] = protoTCP
	buf[7] = 64
	src := netip.MustParseAddr("2001:db8::1").As16()
	dst := netip.MustParseAddr("2001:db8::2").As16()
	copy(buf[8:24], src[:])
	copy(buf[24:40], dst[:])

	binary.BigEndian.PutUint16(buf[40:42], 12345)
	binary.BigEndian.PutUint16(buf[42:44], 443)
	binary.BigEndian.PutUint32(buf[44:48], 0x01020304)
	buf[52] = 0x50
	buf[53] = TCPFlagSYN
	binary.BigEndian.PutUint16(buf[54:56], 0xfaf0)
	return buf
}

func testIPv6TCPPacketWithDestOptions() []byte {
	buf := make([]byte, 68)
	buf[0] = 0x60
	binary.BigEndian.PutUint16(buf[4:6], 28)
	buf[6] = ipv6ExtDestOpts
	buf[7] = 64
	src := netip.MustParseAddr("2001:db8::1").As16()
	dst := netip.MustParseAddr("2001:db8::2").As16()
	copy(buf[8:24], src[:])
	copy(buf[24:40], dst[:])

	buf[40] = protoTCP
	buf[41] = 0

	binary.BigEndian.PutUint16(buf[48:50], 12345)
	binary.BigEndian.PutUint16(buf[50:52], 443)
	binary.BigEndian.PutUint32(buf[52:56], 0x01020304)
	buf[60] = 0x50
	buf[61] = TCPFlagSYN
	binary.BigEndian.PutUint16(buf[62:64], 0xfaf0)
	return buf
}

func testIPv6TCPPacketWithFragment(moreFragments bool) []byte {
	buf := make([]byte, 68)
	buf[0] = 0x60
	binary.BigEndian.PutUint16(buf[4:6], 28)
	buf[6] = ipv6ExtFragment
	buf[7] = 64
	src := netip.MustParseAddr("2001:db8::1").As16()
	dst := netip.MustParseAddr("2001:db8::2").As16()
	copy(buf[8:24], src[:])
	copy(buf[24:40], dst[:])

	buf[40] = protoTCP
	var frag uint16
	if moreFragments {
		frag = 0x0001
	}
	binary.BigEndian.PutUint16(buf[42:44], frag)
	return buf
}
