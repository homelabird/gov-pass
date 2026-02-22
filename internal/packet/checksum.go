package packet

import "encoding/binary"

func Checksum(data []byte) uint16 {
	sum := checksumSum(data)
	sum = foldChecksum(sum)
	return ^uint16(sum)
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
	if totalLen <= 0 || totalLen > len(data) {
		totalLen = len(data)
	}
	tcpLen := totalLen - headerLen
	if tcpLen < 0 || headerLen+tcpLen > len(data) {
		return 0
	}

	sum := uint32(0)
	sum += uint32(binary.BigEndian.Uint16(data[12:14]))
	sum += uint32(binary.BigEndian.Uint16(data[14:16]))
	sum += uint32(binary.BigEndian.Uint16(data[16:18]))
	sum += uint32(binary.BigEndian.Uint16(data[18:20]))
	sum += uint32(data[9])
	sum += uint32(tcpLen)

	sum += checksumSum(data[headerLen : headerLen+tcpLen])
	sum = foldChecksum(sum)
	return ^uint16(sum)
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
