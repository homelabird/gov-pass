package packet

import (
	"encoding/binary"
	"errors"
	"net/netip"
	"sync"
)

var (
	ErrNotIP        = errors.New("not ip")
	ErrNotIPv4      = errors.New("not ipv4")
	ErrNotIPv6      = errors.New("not ipv6")
	ErrNotTCP       = errors.New("not tcp")
	ErrTooShort     = errors.New("packet too short")
	ErrIPv4Fragment = errors.New("ipv4 fragment")
	ErrIPv6Fragment = errors.New("ipv6 fragment")
)

const (
	IPVersion4 = 4
	IPVersion6 = 6

	protoTCP = 6

	ipv6ExtHopByHop = 0
	ipv6ExtRouting  = 43
	ipv6ExtFragment = 44
	ipv6ExtESP      = 50
	ipv6ExtAH       = 51
	ipv6ExtDestOpts = 60
	ipv6ExtMaxHops  = 8

	TCPFlagFIN = 0x01
	TCPFlagSYN = 0x02
	TCPFlagRST = 0x04
	TCPFlagPSH = 0x08
	TCPFlagACK = 0x10
)

type Packet struct {
	Data    []byte
	Addr    Address
	Meta    Meta
	Source  Source
	NFQID   uint32
	IfIndex uint32

	dataPool    *sync.Pool
	dataBacking []byte
}

// Address holds raw WinDivert address bytes for send/recv.
// The size is intentionally large to avoid struct size mismatches.
type Address struct {
	Data [256]byte
}

type Meta struct {
	IPVersion     uint8
	SrcIP         [16]byte
	DstIP         [16]byte
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

func Version(data []byte) uint8 {
	if len(data) == 0 {
		return 0
	}
	return data[0] >> 4
}

func (m Meta) SrcAddr() netip.Addr {
	return addrFromMeta(m.IPVersion, m.SrcIP)
}

func (m Meta) DstAddr() netip.Addr {
	return addrFromMeta(m.IPVersion, m.DstIP)
}

func (p *Packet) Payload() []byte {
	if p.Meta.PayloadOffset <= 0 || p.Meta.PayloadOffset > len(p.Data) {
		return nil
	}
	return p.Data[p.Meta.PayloadOffset:]
}

func (p *Packet) HasFlag(flag uint8) bool {
	return (p.Meta.Flags & flag) != 0
}

// SetDataPool registers the backing buffer with a pool so the packet can
// return it once the adapter has accepted/dropped/reinjected the packet.
func (p *Packet) SetDataPool(pool *sync.Pool, backing []byte) {
	if p == nil || pool == nil || backing == nil {
		return
	}
	p.dataPool = pool
	p.dataBacking = backing
}

// Release returns the packet's backing buffer to its pool, if any.
//
// It intentionally leaves the packet's visible fields intact because callers
// may still need metadata such as len(Data) during later cleanup/accounting
// after the adapter has already accepted or dropped the packet.
func (p *Packet) Release() {
	if p == nil || p.dataPool == nil || p.dataBacking == nil {
		return
	}
	backing := p.dataBacking[:cap(p.dataBacking)]
	p.dataPool.Put(backing)
	p.dataBacking = nil
	p.dataPool = nil
}

// DecodeTCP fills Meta for IPv4/TCP or IPv6/TCP packets.
func DecodeTCP(pkt *Packet) error {
	if len(pkt.Data) < 1 {
		return ErrTooShort
	}
	switch Version(pkt.Data) {
	case IPVersion4:
		return decodeIPv4TCP(pkt)
	case IPVersion6:
		return decodeIPv6TCP(pkt)
	default:
		return ErrNotIP
	}
}

// DecodeIPv4TCP fills Meta for IPv4/TCP packets.
func DecodeIPv4TCP(pkt *Packet) error {
	if len(pkt.Data) < 1 {
		return ErrTooShort
	}
	if Version(pkt.Data) != IPVersion4 {
		return ErrNotIPv4
	}
	return decodeIPv4TCP(pkt)
}

// DecodeIPv6TCP fills Meta for IPv6/TCP packets.
func DecodeIPv6TCP(pkt *Packet) error {
	if len(pkt.Data) < 1 {
		return ErrTooShort
	}
	if Version(pkt.Data) != IPVersion6 {
		return ErrNotIPv6
	}
	return decodeIPv6TCP(pkt)
}

func decodeIPv4TCP(pkt *Packet) error {
	if len(pkt.Data) < 20 {
		return ErrTooShort
	}
	vihl := pkt.Data[0]
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

	pkt.Meta = Meta{}
	pkt.Meta.IPVersion = IPVersion4
	copy(pkt.Meta.SrcIP[:4], pkt.Data[12:16])
	copy(pkt.Meta.DstIP[:4], pkt.Data[16:20])
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

func decodeIPv6TCP(pkt *Packet) error {
	if len(pkt.Data) < 40 {
		return ErrTooShort
	}

	pkt.Meta = Meta{}
	pkt.Meta.IPVersion = IPVersion6
	copy(pkt.Meta.SrcIP[:], pkt.Data[8:24])
	copy(pkt.Meta.DstIP[:], pkt.Data[24:40])

	nextHeader := pkt.Data[6]
	tcpStart := 40
	for i := 0; i < ipv6ExtMaxHops; i++ {
		switch nextHeader {
		case protoTCP:
			if len(pkt.Data) < tcpStart+20 {
				return ErrTooShort
			}
			pkt.Meta.Proto = protoTCP
			pkt.Meta.IPHeaderLen = tcpStart
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
		case ipv6ExtHopByHop, ipv6ExtRouting, ipv6ExtDestOpts:
			if len(pkt.Data) < tcpStart+8 {
				return ErrTooShort
			}
			nextHeader = pkt.Data[tcpStart]
			extLen := (int(pkt.Data[tcpStart+1]) + 1) * 8
			if extLen < 8 || len(pkt.Data) < tcpStart+extLen {
				return ErrTooShort
			}
			tcpStart += extLen
		case ipv6ExtFragment:
			if len(pkt.Data) < tcpStart+8 {
				return ErrTooShort
			}
			frag := binary.BigEndian.Uint16(pkt.Data[tcpStart+2 : tcpStart+4])
			if (frag&0xfff8) != 0 || (frag&0x0001) != 0 {
				return ErrIPv6Fragment
			}
			nextHeader = pkt.Data[tcpStart]
			tcpStart += 8
		case ipv6ExtAH:
			if len(pkt.Data) < tcpStart+8 {
				return ErrTooShort
			}
			nextHeader = pkt.Data[tcpStart]
			extLen := (int(pkt.Data[tcpStart+1]) + 2) * 4
			if extLen < 8 || len(pkt.Data) < tcpStart+extLen {
				return ErrTooShort
			}
			tcpStart += extLen
		case ipv6ExtESP:
			return ErrNotTCP
		default:
			return ErrNotTCP
		}
	}

	return ErrTooShort
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

func SetIPv6PayloadLength(data []byte, payloadLen uint16) {
	if len(data) < 6 {
		return
	}
	binary.BigEndian.PutUint16(data[4:6], payloadLen)
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

func addrFromMeta(version uint8, raw [16]byte) netip.Addr {
	switch version {
	case IPVersion4:
		return netip.AddrFrom4([4]byte{raw[0], raw[1], raw[2], raw[3]})
	case IPVersion6:
		return netip.AddrFrom16(raw)
	default:
		return netip.Addr{}
	}
}
