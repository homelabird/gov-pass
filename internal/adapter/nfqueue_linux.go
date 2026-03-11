//go:build linux

package adapter

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"time"

	nfqueue "github.com/florianl/go-nfqueue"
	"github.com/mdlayher/netlink"
	"golang.org/x/sys/unix"

	"fk-gov/internal/packet"
)

const nfqueueMaxPacket = 0xFFFF

// NFQueueAdapter handles NFQUEUE recv and raw socket injection.
type NFQueueAdapter struct {
	queue4 *nfqueue.Nfqueue
	queue6 *nfqueue.Nfqueue
	recv   chan *packet.Packet
	errs   chan error
	ctx    context.Context
	stop   context.CancelFunc

	rawFD4 int
	rawFD6 int
	mark   uint32

	closeOnce sync.Once
	bufPool   sync.Pool

	flushing atomic.Bool
	inFlight atomic.Int32
}

func NewNFQueue(opts NFQueueOptions) (*NFQueueAdapter, error) {
	copyRange := opts.CopyRange
	if copyRange == 0 {
		copyRange = nfqueueMaxPacket
	}

	ctx, cancel := context.WithCancel(context.Background())
	ad := &NFQueueAdapter{
		recv:   make(chan *packet.Packet, 1024),
		errs:   make(chan error, 1),
		ctx:    ctx,
		stop:   cancel,
		rawFD4: -1,
		rawFD6: -1,
		mark:   opts.Mark,
	}
	ad.bufPool.New = func() any {
		return make([]byte, copyRange)
	}

	var err error
	ad.queue4, err = openNFQueueSocket(opts, copyRange, uint8(unix.AF_INET))
	if err != nil {
		cancel()
		return nil, err
	}
	ad.queue6, err = openNFQueueSocket(opts, copyRange, uint8(unix.AF_INET6))
	if err != nil {
		_ = ad.queue4.Close()
		cancel()
		return nil, err
	}

	if err := ad.openRawSockets(); err != nil {
		_ = ad.Close()
		return nil, err
	}
	if err := ad.queue4.RegisterWithErrorFunc(ctx, ad.onPacket(packet.IPVersion4), ad.onError); err != nil {
		_ = ad.Close()
		return nil, err
	}
	if err := ad.queue6.RegisterWithErrorFunc(ctx, ad.onPacket(packet.IPVersion6), ad.onError); err != nil {
		_ = ad.Close()
		return nil, err
	}

	return ad, nil
}

func (n *NFQueueAdapter) Recv(ctx context.Context) (*packet.Packet, error) {
	select {
	case pkt := <-n.recv:
		return pkt, nil
	case err := <-n.errs:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-n.ctx.Done():
		return nil, n.ctx.Err()
	}
}

func (n *NFQueueAdapter) Send(ctx context.Context, pkt *packet.Packet) error {
	if pkt == nil || len(pkt.Data) == 0 {
		return nil
	}
	if pkt.Source == packet.SourceCaptured {
		err := n.setVerdict(pkt, nfqueue.NfAccept)
		if err == nil {
			pkt.Release()
		}
		return err
	}
	err := n.inject(pkt)
	if err == nil {
		pkt.Release()
	}
	return err
}

func (n *NFQueueAdapter) Drop(ctx context.Context, pkt *packet.Packet) error {
	if pkt == nil {
		return nil
	}
	if pkt.Source != packet.SourceCaptured {
		return nil
	}
	err := n.setVerdict(pkt, nfqueue.NfDrop)
	if err == nil {
		pkt.Release()
	}
	return err
}

func (n *NFQueueAdapter) CalcChecksums(pkt *packet.Packet) error {
	if pkt == nil || len(pkt.Data) == 0 {
		return nil
	}
	if err := packet.DecodeTCP(pkt); err != nil {
		return nil
	}
	packet.SetTCPChecksumZero(pkt.Data, pkt.Meta.IPHeaderLen)
	switch pkt.Meta.IPVersion {
	case packet.IPVersion4:
		packet.SetIPv4ChecksumZero(pkt.Data)
		ipSum := packet.IPv4Checksum(pkt.Data, pkt.Meta.IPHeaderLen)
		tcpSum := packet.TCPChecksumIPv4(pkt.Data, pkt.Meta.IPHeaderLen)
		packet.SetIPv4Checksum(pkt.Data, ipSum)
		packet.SetTCPChecksum(pkt.Data, pkt.Meta.IPHeaderLen, tcpSum)
	case packet.IPVersion6:
		tcpSum := packet.TCPChecksumIPv6(pkt.Data, pkt.Meta.IPHeaderLen)
		packet.SetTCPChecksum(pkt.Data, pkt.Meta.IPHeaderLen, tcpSum)
	}
	return nil
}

// Flush releases any packets already delivered to the adapter recv buffer by
// accepting them (fail-open). It also stops new callbacks best-effort so no
// additional packets are enqueued while draining.
func (n *NFQueueAdapter) Flush(ctx context.Context) error {
	if n.queue4 == nil && n.queue6 == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	n.flushing.Store(true)
	if n.stop != nil {
		n.stop()
	}

	// Wait for in-flight callbacks to finish so no new packets are enqueued after
	// we decide the recv buffer is drained.
	for {
		if n.inFlight.Load() == 0 {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}

	var errs []error
	for {
		select {
		case pkt := <-n.recv:
			if pkt == nil {
				continue
			}
			if err := n.setVerdict(pkt, nfqueue.NfAccept); err != nil {
				errs = append(errs, err)
			} else {
				pkt.Release()
			}
		default:
			if len(errs) > 0 {
				return errors.Join(errs...)
			}
			return nil
		}
	}
}

func (n *NFQueueAdapter) Close() error {
	var err error
	n.closeOnce.Do(func() {
		if n.stop != nil {
			n.stop()
		}
		var errs []error
		if n.queue4 != nil {
			if e := n.queue4.Close(); e != nil {
				errs = append(errs, e)
			}
		}
		if n.queue6 != nil {
			if e := n.queue6.Close(); e != nil {
				errs = append(errs, e)
			}
		}
		if n.rawFD4 >= 0 {
			if e := unix.Close(n.rawFD4); e != nil {
				errs = append(errs, e)
			}
			n.rawFD4 = -1
		}
		if n.rawFD6 >= 0 {
			if e := unix.Close(n.rawFD6); e != nil {
				errs = append(errs, e)
			}
			n.rawFD6 = -1
		}
		if len(errs) > 0 {
			err = errors.Join(errs...)
		}
	})
	return err
}

func (n *NFQueueAdapter) onPacket(version uint8) nfqueue.HookFunc {
	return func(a nfqueue.Attribute) int {
		n.inFlight.Add(1)
		defer n.inFlight.Add(-1)

		if a.Payload == nil || a.PacketID == nil {
			return 0
		}
		id := *a.PacketID

		// If we're flushing/shutting down, do not enqueue. Immediately fail-open.
		if n.flushing.Load() || n.ctx.Err() != nil {
			_ = n.setVerdictByVersion(version, id, nfqueue.NfAccept)
			return 0
		}

		payload, backing := copyIntoPoolBuffer(&n.bufPool, *a.Payload)
		pkt := &packet.Packet{
			Data:    payload,
			Source:  packet.SourceCaptured,
			NFQID:   id,
			IfIndex: nfqueueIfIndex(a),
		}
		pkt.Meta.IPVersion = version
		pkt.SetDataPool(&n.bufPool, backing)

		select {
		case n.recv <- pkt:
			return 0
		default:
			_ = n.setVerdictByVersion(version, id, nfqueue.NfAccept)
			pkt.Release()
			return 0
		}
	}
}

func (n *NFQueueAdapter) onError(err error) int {
	if opErr, ok := err.(*netlink.OpError); ok {
		if opErr.Timeout() {
			return 0
		}
	}
	select {
	case n.errs <- err:
	default:
	}
	return -1
}

func (n *NFQueueAdapter) setVerdict(pkt *packet.Packet, verdict int) error {
	if pkt == nil {
		return nil
	}
	return n.setVerdictByVersion(n.packetVersion(pkt), pkt.NFQID, verdict)
}

func (n *NFQueueAdapter) setVerdictByVersion(version uint8, id uint32, verdict int) error {
	queue := n.queueForVersion(version)
	if queue == nil {
		return ErrNotImplemented
	}
	return queue.SetVerdict(id, verdict)
}

func (n *NFQueueAdapter) openRawSockets() error {
	fd4, err := openRawSocketIPv4(n.mark)
	if err != nil {
		return err
	}
	fd6, err := openRawSocketIPv6(n.mark)
	if err != nil {
		_ = unix.Close(fd4)
		return err
	}
	n.rawFD4 = fd4
	n.rawFD6 = fd6
	return nil
}

func (n *NFQueueAdapter) inject(pkt *packet.Packet) error {
	switch n.packetVersion(pkt) {
	case packet.IPVersion4:
		return n.injectIPv4(pkt)
	case packet.IPVersion6:
		return n.injectIPv6(pkt)
	default:
		return nil
	}
}

func openNFQueueSocket(opts NFQueueOptions, copyRange uint32, family uint8) (*nfqueue.Nfqueue, error) {
	cfg := nfqueue.Config{
		NfQueue:      opts.QueueNum,
		MaxPacketLen: copyRange,
		MaxQueueLen:  opts.QueueMaxLen,
		Copymode:     nfqueue.NfQnlCopyPacket,
		AfFamily:     family,
	}

	queue, err := nfqueue.Open(&cfg)
	if err != nil {
		return nil, err
	}
	if err := queue.SetOption(netlink.NoENOBUFS, true); err != nil {
		_ = queue.Close()
		return nil, err
	}
	return queue, nil
}

func openRawSocketIPv4(mark uint32) (int, error) {
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_RAW, unix.IPPROTO_RAW)
	if err != nil {
		return -1, err
	}
	if err := unix.SetsockoptInt(fd, unix.IPPROTO_IP, unix.IP_HDRINCL, 1); err != nil {
		_ = unix.Close(fd)
		return -1, err
	}
	if err := applySocketMark(fd, mark); err != nil {
		_ = unix.Close(fd)
		return -1, err
	}
	return fd, nil
}

func openRawSocketIPv6(mark uint32) (int, error) {
	fd, err := unix.Socket(unix.AF_INET6, unix.SOCK_RAW, unix.IPPROTO_TCP)
	if err != nil {
		return -1, err
	}
	if err := applySocketMark(fd, mark); err != nil {
		_ = unix.Close(fd)
		return -1, err
	}
	return fd, nil
}

func applySocketMark(fd int, mark uint32) error {
	if mark == 0 {
		return nil
	}
	return unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_MARK, int(mark))
}

func (n *NFQueueAdapter) injectIPv4(pkt *packet.Packet) error {
	if n.rawFD4 < 0 {
		return ErrNotImplemented
	}
	if len(pkt.Data) < 20 {
		return nil
	}

	var dst unix.SockaddrInet4
	copy(dst.Addr[:], pkt.Data[16:20])
	return unix.Sendto(n.rawFD4, pkt.Data, 0, &dst)
}

func (n *NFQueueAdapter) injectIPv6(pkt *packet.Packet) error {
	if n.rawFD6 < 0 {
		return ErrNotImplemented
	}
	if pkt == nil || len(pkt.Data) < 40 {
		return nil
	}
	if pkt.Meta.IPVersion != packet.IPVersion6 || pkt.Meta.IPHeaderLen == 0 {
		if err := packet.DecodeTCP(pkt); err != nil {
			return nil
		}
	}
	if pkt.Meta.IPVersion != packet.IPVersion6 || pkt.Meta.IPHeaderLen <= 0 || pkt.Meta.IPHeaderLen > len(pkt.Data) {
		return nil
	}

	payload := pkt.Data[pkt.Meta.IPHeaderLen:]
	if len(payload) == 0 {
		return nil
	}

	dst := &unix.SockaddrInet6{}
	copy(dst.Addr[:], pkt.Meta.DstIP[:])

	info := &unix.Inet6Pktinfo{Ifindex: pkt.IfIndex}
	copy(info.Addr[:], pkt.Meta.SrcIP[:])
	oob := unix.PktInfo6(info)

	nn, err := unix.SendmsgN(n.rawFD6, payload, oob, dst, 0)
	if err != nil {
		return err
	}
	if nn != len(payload) {
		return io.ErrShortWrite
	}
	return nil
}

func (n *NFQueueAdapter) packetVersion(pkt *packet.Packet) uint8 {
	if pkt == nil {
		return 0
	}
	if pkt.Meta.IPVersion != 0 {
		return pkt.Meta.IPVersion
	}
	return packet.Version(pkt.Data)
}

func (n *NFQueueAdapter) queueForVersion(version uint8) *nfqueue.Nfqueue {
	switch version {
	case packet.IPVersion6:
		return n.queue6
	case packet.IPVersion4:
		return n.queue4
	default:
		return nil
	}
}

func nfqueueIfIndex(a nfqueue.Attribute) uint32 {
	if a.OutDev != nil && *a.OutDev != 0 {
		return *a.OutDev
	}
	if a.PhysOutDev != nil && *a.PhysOutDev != 0 {
		return *a.PhysOutDev
	}
	return 0
}
