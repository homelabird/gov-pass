//go:build freebsd

package adapter

import (
	"context"
	"encoding/binary"
	"errors"
	"sync"
	"time"

	"golang.org/x/sys/unix"

	"fk-gov/internal/packet"
)

const (
	divertMaxPacket   = 0xFFFF
	divertReadTimeout = 200 * time.Millisecond
)

// DivertOptions configures the FreeBSD divert socket.
type DivertOptions struct {
	Port uint16
}

// DivertAdapter handles pf divert recv/send.
type DivertAdapter struct {
	fd int
	port uint16

	closeOnce sync.Once
}

func NewDivert(opts DivertOptions) (*DivertAdapter, error) {
	if opts.Port == 0 {
		return nil, errors.New("divert port must be > 0")
	}

	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_RAW, unix.IPPROTO_DIVERT)
	if err != nil {
		return nil, err
	}

	ad := &DivertAdapter{
		fd:   fd,
		port: opts.Port,
	}
	if err := ad.bind(opts.Port); err != nil {
		_ = ad.Close()
		return nil, err
	}
	if err := ad.setReadTimeout(divertReadTimeout); err != nil {
		_ = ad.Close()
		return nil, err
	}
	return ad, nil
}

func (d *DivertAdapter) Recv(ctx context.Context) (*packet.Packet, error) {
	if d.fd < 0 {
		return nil, ErrNotImplemented
	}

	buf := make([]byte, divertMaxPacket)
	for {
		n, from, err := unix.Recvfrom(d.fd, buf, 0)
		if err != nil {
			if err == unix.EINTR {
				continue
			}
			if err == unix.EAGAIN || err == unix.EWOULDBLOCK {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				continue
			}
			return nil, err
		}
		if n == 0 {
			continue
		}

		addr, err := encodeDivertAddr(from)
		if err != nil {
			if sendErr := unix.Sendto(d.fd, buf[:n], 0, from); sendErr != nil {
				return nil, sendErr
			}
			continue
		}
		payload := append([]byte(nil), buf[:n]...)
		return &packet.Packet{
			Data:   payload,
			Addr:   addr,
			Source: packet.SourceCaptured,
		}, nil
	}
}

func (d *DivertAdapter) Send(ctx context.Context, pkt *packet.Packet) error {
	if d.fd < 0 {
		return ErrNotImplemented
	}
	if pkt == nil || len(pkt.Data) == 0 {
		return nil
	}
	to, err := decodeDivertAddr(pkt.Addr, d.port)
	if err != nil {
		return err
	}
	return unix.Sendto(d.fd, pkt.Data, 0, to)
}

func (d *DivertAdapter) Drop(ctx context.Context, pkt *packet.Packet) error {
	return nil
}

func (d *DivertAdapter) CalcChecksums(pkt *packet.Packet) error {
	if pkt == nil || len(pkt.Data) < 20 {
		return nil
	}
	ipHeaderLen := int(pkt.Data[0]&0x0f) * 4
	if ipHeaderLen < 20 || len(pkt.Data) < ipHeaderLen+20 {
		return nil
	}
	packet.SetIPv4ChecksumZero(pkt.Data)
	packet.SetTCPChecksumZero(pkt.Data, ipHeaderLen)

	ipSum := packet.IPv4Checksum(pkt.Data, ipHeaderLen)
	tcpSum := packet.TCPChecksumIPv4(pkt.Data, ipHeaderLen)
	packet.SetIPv4Checksum(pkt.Data, ipSum)
	packet.SetTCPChecksum(pkt.Data, ipHeaderLen, tcpSum)
	return nil
}

func (d *DivertAdapter) Close() error {
	var err error
	d.closeOnce.Do(func() {
		if d.fd >= 0 {
			err = unix.Close(d.fd)
			d.fd = -1
		}
	})
	return err
}

func (d *DivertAdapter) bind(port uint16) error {
	addr := &unix.SockaddrInet4{Port: int(port)}
	return unix.Bind(d.fd, addr)
}

func (d *DivertAdapter) setReadTimeout(timeout time.Duration) error {
	tv := unix.NsecToTimeval(timeout.Nanoseconds())
	return unix.SetsockoptTimeval(d.fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &tv)
}

func encodeDivertAddr(sa unix.Sockaddr) (packet.Address, error) {
	var addr packet.Address
	switch v := sa.(type) {
	case *unix.SockaddrInet4:
		binary.BigEndian.PutUint16(addr.Data[0:2], uint16(unix.AF_INET))
		binary.BigEndian.PutUint16(addr.Data[2:4], uint16(v.Port))
		copy(addr.Data[4:8], v.Addr[:])
		return addr, nil
	case *unix.SockaddrInet6:
		binary.BigEndian.PutUint16(addr.Data[0:2], uint16(unix.AF_INET6))
		binary.BigEndian.PutUint16(addr.Data[2:4], uint16(v.Port))
		copy(addr.Data[4:20], v.Addr[:])
		binary.BigEndian.PutUint32(addr.Data[20:24], v.ZoneId)
		return addr, nil
	default:
		return addr, errors.New("divert addr unsupported")
	}
}

func decodeDivertAddr(addr packet.Address, fallbackPort uint16) (unix.Sockaddr, error) {
	family := binary.BigEndian.Uint16(addr.Data[0:2])
	switch family {
	case uint16(unix.AF_INET):
		port := binary.BigEndian.Uint16(addr.Data[2:4])
		if port == 0 {
			port = fallbackPort
		}
		if port == 0 {
			return nil, errors.New("divert addr missing port")
		}
		var ip [4]byte
		copy(ip[:], addr.Data[4:8])
		return &unix.SockaddrInet4{
			Port: int(port),
			Addr: ip,
		}, nil
	case uint16(unix.AF_INET6):
		port := binary.BigEndian.Uint16(addr.Data[2:4])
		if port == 0 {
			port = fallbackPort
		}
		if port == 0 {
			return nil, errors.New("divert addr missing port")
		}
		var ip [16]byte
		copy(ip[:], addr.Data[4:20])
		zone := binary.BigEndian.Uint32(addr.Data[20:24])
		return &unix.SockaddrInet6{
			Port:   int(port),
			Addr:   ip,
			ZoneId: zone,
		}, nil
	default:
		return legacyDivertAddr(addr, fallbackPort)
	}
}

func legacyDivertAddr(addr packet.Address, fallbackPort uint16) (unix.Sockaddr, error) {
	port := binary.BigEndian.Uint16(addr.Data[0:2])
	if port == 0 {
		port = fallbackPort
	}
	if port == 0 {
		return nil, errors.New("divert addr missing port")
	}
	var ip [4]byte
	copy(ip[:], addr.Data[2:6])
	return &unix.SockaddrInet4{
		Port: int(port),
		Addr: ip,
	}, nil
}
