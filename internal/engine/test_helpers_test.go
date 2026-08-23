package engine

import (
	"encoding/binary"
	"net/netip"
	"testing"

	"fk-gov/internal/packet"
)

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

	pkt := &packet.Packet{Data: buf, Source: packet.SourceCaptured}
	if err := packet.DecodeTCP(pkt); err != nil {
		t.Fatalf("DecodeTCP failed: %v", err)
	}
	return pkt
}
