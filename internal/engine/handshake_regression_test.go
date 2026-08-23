package engine

import (
	"encoding/binary"
	"testing"

	"fk-gov/internal/packet"
)

func splitClientHelloPackets(t *testing.T, srcPort uint16, baseSeq uint32, record []byte) []*packet.Packet {
	t.Helper()
	if len(record) < 8 {
		t.Fatalf("client hello record too short: %d", len(record))
	}
	cut := len(record) / 2
	if cut < 6 {
		cut = 6
	}
	if cut >= len(record) {
		cut = len(record) - 1
	}
	first := buildTestIPv4Packet(t, srcPort, baseSeq, packet.TCPFlagPSH|packet.TCPFlagACK, record[:cut])
	second := buildTestIPv4Packet(t, srcPort, baseSeq+uint32(cut), packet.TCPFlagPSH|packet.TCPFlagACK, record[cut:])
	return []*packet.Packet{first, second}
}

func joinPayloads(pkts []*packet.Packet) []byte {
	total := 0
	for _, pkt := range pkts {
		total += len(decodedPayload(nil, pkt))
	}
	out := make([]byte, 0, total)
	for _, pkt := range pkts {
		out = append(out, decodedPayload(nil, pkt)...)
	}
	return out
}

func decodedPayload(t *testing.T, pkt *packet.Packet) []byte {
	if pkt == nil {
		return nil
	}
	if pkt.Meta.TCPHeaderLen == 0 {
		if err := packet.DecodeTCP(pkt); err != nil {
			if t != nil {
				t.Fatalf("DecodeTCP failed: %v", err)
			}
			return nil
		}
	}
	return pkt.Payload()
}

func buildClientHelloRecordForReplay(serverName string) []byte {
	extensions := []byte{}
	if serverName != "" {
		name := []byte(serverName)
		serverNameData := make([]byte, 2+1+2+len(name))
		binary.BigEndian.PutUint16(serverNameData[0:2], uint16(3+len(name)))
		serverNameData[2] = 0
		binary.BigEndian.PutUint16(serverNameData[3:5], uint16(len(name)))
		copy(serverNameData[5:], name)

		ext := make([]byte, 4+len(serverNameData))
		binary.BigEndian.PutUint16(ext[0:2], 0)
		binary.BigEndian.PutUint16(ext[2:4], uint16(len(serverNameData)))
		copy(ext[4:], serverNameData)
		extensions = append(extensions, ext...)
	}

	body := make([]byte, 0, 64+len(extensions))
	body = append(body, 0x03, 0x03)
	body = append(body, make([]byte, 32)...)
	body = append(body, 0x00)
	body = append(body, 0x00, 0x02, 0x13, 0x01)
	body = append(body, 0x01, 0x00)
	body = append(body, byte(len(extensions)>>8), byte(len(extensions)))
	body = append(body, extensions...)

	handshake := make([]byte, 4+len(body))
	handshake[0] = 0x01
	handshake[1] = byte(len(body) >> 16)
	handshake[2] = byte(len(body) >> 8)
	handshake[3] = byte(len(body))
	copy(handshake[4:], body)

	record := make([]byte, 5+len(handshake))
	record[0] = 0x16
	record[1] = 0x03
	record[2] = 0x03
	binary.BigEndian.PutUint16(record[3:5], uint16(len(handshake)))
	copy(record[5:], handshake)
	return record
}
