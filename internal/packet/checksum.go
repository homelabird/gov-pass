package packet

import (
	"encoding/binary"

	"fk-gov/internal/safecast"
)

func Checksum(data []byte) uint16 {
	return finalizeChecksum(checksumSum(data))
}

func IPv4Checksum(data []byte, headerLen int) uint16 {
	if headerLen < 20 || len(data) < headerLen {
		return 0
	}
	return Checksum(data[:headerLen])
}

func TCPChecksumIPv4(data []byte, headerLen int) uint16 {
	if headerLen < 20 || len(data) < headerLen+20 {
		return 0
	}
	totalLen := int(binary.BigEndian.Uint16(data[2:4]))
	if totalLen < headerLen+20 || totalLen > len(data) {
		return 0
	}
	tcpLen := totalLen - headerLen

	sum := uint32(0)
	sum += uint32(binary.BigEndian.Uint16(data[12:14]))
	sum += uint32(binary.BigEndian.Uint16(data[14:16]))
	sum += uint32(binary.BigEndian.Uint16(data[16:18]))
	sum += uint32(binary.BigEndian.Uint16(data[18:20]))
	sum += uint32(data[9])
	tcpLen32, ok := safecast.IntToUint32(tcpLen)
	if !ok {
		return 0
	}
	sum += tcpLen32

	sum += checksumSum(data[headerLen:totalLen])
	return finalizeChecksum(sum)
}

func TCPChecksumIPv6(data []byte, headerLen int) uint16 {
	if headerLen < 40 || len(data) < headerLen+20 {
		return 0
	}
	payloadLen := int(binary.BigEndian.Uint16(data[4:6]))
	if payloadLen == 0 {
		return 0
	}
	totalLen := 40 + payloadLen
	if totalLen > len(data) {
		return 0
	}
	if totalLen < headerLen+20 {
		return 0
	}
	tcpLen := totalLen - headerLen

	sum := uint32(0)
	for i := 8; i < 24; i += 2 {
		sum += uint32(binary.BigEndian.Uint16(data[i : i+2]))
	}
	for i := 24; i < 40; i += 2 {
		sum += uint32(binary.BigEndian.Uint16(data[i : i+2]))
	}
	tcpLen32, ok := safecast.IntToUint32(tcpLen)
	if !ok {
		return 0
	}
	sum += tcpLen32 >> 16
	sum += tcpLen32 & 0xffff
	sum += uint32(protoTCP)

	sum += checksumSum(data[headerLen:totalLen])
	return finalizeChecksum(sum)
}

func checksumSum(data []byte) uint32 {
	var sum uint32
	for len(data) > 1 {
		sum += uint32(binary.BigEndian.Uint16(data[:2]))
		data = data[2:]
	}
	if len(data) == 1 {
		sum += uint32(data[0]) << 8
	}
	return sum
}

func foldChecksum(sum uint32) uint32 {
	for (sum >> 16) != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return sum
}

func finalizeChecksum(sum uint32) uint16 {
	sum = foldChecksum(sum)
	folded, ok := safecast.Uint32ToUint16(sum)
	if !ok {
		return 0
	}
	return ^folded
}
