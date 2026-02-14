package packet

import (
	"encoding/binary"
	"errors"
)

var (
	ErrNotIPv4      = errors.New("not ipv4")
	ErrNotTCP       = errors.New("not tcp")
	ErrTooShort     = errors.New("packet too short")
	ErrIPv4Fragment = errors.New("ipv4 fragment")
)

const (
	protoTCP = 6

	TCPFlagFIN = 0x01
	TCPFlagSYN = 0x02
	TCPFlagRST = 0x04
	TCPFlagPSH = 0x08
	TCPFlagACK = 0x10
)

type Packet struct {
	Data   []byte
	Addr   Address
	Meta   Meta
	Source Source
	NFQID  uint32
}

// Address holds raw WinDivert address bytes for send/recv.
// The size is intentionally large to avoid struct size mismatches.
type Address struct {
	Data [256]byte
}

type Meta struct {
	SrcIP         [4]byte
	DstIP         [4]byte
	SrcPort       uint16
	DstPort       uint16
	Proto         uint8
	Seq           uint32
	Ack           uint32
	Flags         uint8
	IPHeaderLen   int
	TCPHeaderLen  int
	PayloadOffset int
}

type Source uint8

const (
	SourceUnknown Source = iota
	SourceCaptured
	SourceInjected
)

func (p *Packet) Payload() []byte {
	if p.Meta.PayloadOffset <= 0 || p.Meta.PayloadOffset > len(p.Data) {
		return nil
	}
	return p.Data[p.Meta.PayloadOffset:]
}

func (p *Packet) HasFlag(flag uint8) bool {
	return (p.Meta.Flags & flag) != 0
}

// DecodeIPv4TCP fills Meta for IPv4/TCP packets.
func DecodeIPv4TCP(pkt *Packet) error {
	if len(pkt.Data) < 20 {
		return ErrTooShort
	}
	vihl := pkt.Data[0]
	if vihl>>4 != 4 {
		return ErrNotIPv4
	}
	ihl := int(vihl&0x0f) * 4
	if ihl < 20 || len(pkt.Data) < ihl+20 {
		return ErrTooShort
	}
	flagsOffset := binary.BigEndian.Uint16(pkt.Data[6:8])
	if (flagsOffset&0x1fff) != 0 || (flagsOffset&0x2000) != 0 {
		return ErrIPv4Fragment
	}
	if pkt.Data[9] != protoTCP {
		return ErrNotTCP
	}

	copy(pkt.Meta.SrcIP[:], pkt.Data[12:16])
	copy(pkt.Meta.DstIP[:], pkt.Data[16:20])
	pkt.Meta.Proto = protoTCP
	pkt.Meta.IPHeaderLen = ihl

	tcpStart := ihl
	pkt.Meta.SrcPort = binary.BigEndian.Uint16(pkt.Data[tcpStart : tcpStart+2])
	pkt.Meta.DstPort = binary.BigEndian.Uint16(pkt.Data[tcpStart+2 : tcpStart+4])
	pkt.Meta.Seq = binary.BigEndian.Uint32(pkt.Data[tcpStart+4 : tcpStart+8])
	pkt.Meta.Ack = binary.BigEndian.Uint32(pkt.Data[tcpStart+8 : tcpStart+12])

	dataOffset := int(pkt.Data[tcpStart+12]>>4) * 4
	if dataOffset < 20 || len(pkt.Data) < tcpStart+dataOffset {
		return ErrTooShort
	}
	pkt.Meta.Flags = pkt.Data[tcpStart+13]
	pkt.Meta.TCPHeaderLen = dataOffset
	pkt.Meta.PayloadOffset = tcpStart + dataOffset
	return nil
}

func IPv4ID(data []byte) uint16 {
	if len(data) < 6 {
		return 0
	}
	return binary.BigEndian.Uint16(data[4:6])
}

func SetIPv4ID(data []byte, id uint16) {
	if len(data) < 6 {
		return
	}
	binary.BigEndian.PutUint16(data[4:6], id)
}

func SetIPv4TotalLength(data []byte, total uint16) {
	if len(data) < 4 {
		return
	}
	binary.BigEndian.PutUint16(data[2:4], total)
}

func SetIPv4ChecksumZero(data []byte) {
	if len(data) < 12 {
		return
	}
	data[10] = 0
	data[11] = 0
}

func SetIPv4Checksum(data []byte, sum uint16) {
	if len(data) < 12 {
		return
	}
	binary.BigEndian.PutUint16(data[10:12], sum)
}

func SetTCPSeq(data []byte, ipHeaderLen int, seq uint32) {
	if len(data) < ipHeaderLen+8 {
		return
	}
	binary.BigEndian.PutUint32(data[ipHeaderLen+4:ipHeaderLen+8], seq)
}

func SetTCPChecksumZero(data []byte, ipHeaderLen int) {
	if len(data) < ipHeaderLen+18 {
		return
	}
	data[ipHeaderLen+16] = 0
	data[ipHeaderLen+17] = 0
}

func SetTCPChecksum(data []byte, ipHeaderLen int, sum uint16) {
	if len(data) < ipHeaderLen+18 {
		return
	}
	binary.BigEndian.PutUint16(data[ipHeaderLen+16:ipHeaderLen+18], sum)
}

func SetTCPFlags(data []byte, ipHeaderLen int, flags uint8) {
	if len(data) < ipHeaderLen+14 {
		return
	}
	data[ipHeaderLen+13] = flags
}
