package packet

import (
	"encoding/binary"
	"testing"
)

func TestIPv4Checksum(t *testing.T) {
	pkt := testIPv4TCPPacket()
	sum := IPv4Checksum(pkt, 20)
	if sum != 0x3253 {
		t.Fatalf("unexpected ipv4 checksum: got 0x%04x", sum)
	}
}

func TestTCPChecksumIPv4(t *testing.T) {
	pkt := testIPv4TCPPacket()
	sum := TCPChecksumIPv4(pkt, 20)
	if sum != 0x92c0 {
		t.Fatalf("unexpected tcp checksum: got 0x%04x", sum)
	}
}

func TestTCPChecksumIPv6(t *testing.T) {
	pkt := testIPv6TCPPacket()
	sum := TCPChecksumIPv6(pkt, 40)
	if sum != 0x2383 {
		t.Fatalf("unexpected tcp checksum: got 0x%04x", sum)
	}
}

func TestTCPChecksumIPv4HonorsTotalLength(t *testing.T) {
	pkt := append(testIPv4TCPPacket(), 0xde, 0xad, 0xbe, 0xef)
	sum := TCPChecksumIPv4(pkt, 20)
	if sum != 0x92c0 {
		t.Fatalf("unexpected tcp checksum with trailing bytes: got 0x%04x", sum)
	}
	binary.BigEndian.PutUint16(pkt[2:4], uint16(len(pkt)+1))
	if sum := TCPChecksumIPv4(pkt, 20); sum != 0 {
		t.Fatalf("expected zero checksum for oversized IPv4 total length, got 0x%04x", sum)
	}
}

func TestTCPChecksumIPv6HonorsPayloadLength(t *testing.T) {
	pkt := append(testIPv6TCPPacket(), 0xde, 0xad, 0xbe, 0xef)
	sum := TCPChecksumIPv6(pkt, 40)
	if sum != 0x2383 {
		t.Fatalf("unexpected tcp checksum with trailing bytes: got 0x%04x", sum)
	}
	binary.BigEndian.PutUint16(pkt[4:6], uint16(len(pkt)-39))
	if sum := TCPChecksumIPv6(pkt, 40); sum != 0 {
		t.Fatalf("expected zero checksum for oversized IPv6 payload length, got 0x%04x", sum)
	}
}

func TestTCPChecksumIPv6RejectsUnsupportedJumboPayload(t *testing.T) {
	pkt := testIPv6TCPPacket()
	binary.BigEndian.PutUint16(pkt[4:6], 0)
	if sum := TCPChecksumIPv6(pkt, 40); sum != 0 {
		t.Fatalf("expected zero checksum for unsupported IPv6 jumbo payload, got 0x%04x", sum)
	}
}

func TestTCPSettersIgnoreInvalidHeaderOffsets(t *testing.T) {
	data := make([]byte, 20)
	maxInt := int(^uint(0) >> 1)
	for _, ipHeaderLen := range []int{-1, 20, maxInt} {
		t.Run("", func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("setter panicked for ipHeaderLen=%d: %v", ipHeaderLen, r)
				}
			}()
			SetTCPSeq(data, ipHeaderLen, 1)
			SetTCPChecksumZero(data, ipHeaderLen)
			SetTCPChecksum(data, ipHeaderLen, 1)
			SetTCPFlags(data, ipHeaderLen, TCPFlagACK)
		})
	}
}

func testIPv4TCPPacket() []byte {
	buf := make([]byte, 40)
	buf[0] = 0x45
	buf[1] = 0x00
	binary.BigEndian.PutUint16(buf[2:4], 40)
	binary.BigEndian.PutUint16(buf[4:6], 0x1c46)
	binary.BigEndian.PutUint16(buf[6:8], 0x4000)
	buf[8] = 64
	buf[9] = 6
	copy(buf[12:16], []byte{192, 0, 2, 1})
	copy(buf[16:20], []byte{198, 51, 100, 2})

	binary.BigEndian.PutUint16(buf[20:22], 12345)
	binary.BigEndian.PutUint16(buf[22:24], 443)
	binary.BigEndian.PutUint32(buf[24:28], 0x01020304)
	binary.BigEndian.PutUint32(buf[28:32], 0)
	buf[32] = 0x50
	buf[33] = 0x02
	binary.BigEndian.PutUint16(buf[34:36], 0xfaf0)
	return buf
}
