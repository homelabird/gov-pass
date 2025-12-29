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
