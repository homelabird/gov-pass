//go:build linux

package adapter

import (
	"context"
	"errors"
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
	queue *nfqueue.Nfqueue
	recv  chan *packet.Packet
	errs  chan error
	ctx   context.Context
	stop  context.CancelFunc

	rawFD int
	mark  uint32

	closeOnce sync.Once

	flushing atomic.Bool
	inFlight atomic.Int32
}

func NewNFQueue(opts NFQueueOptions) (*NFQueueAdapter, error) {
	copyRange := opts.CopyRange
	if copyRange == 0 {
		copyRange = nfqueueMaxPacket
	}

	cfg := nfqueue.Config{
		NfQueue:      opts.QueueNum,
		MaxPacketLen: copyRange,
		MaxQueueLen:  opts.QueueMaxLen,
		Copymode:     nfqueue.NfQnlCopyPacket,
		AfFamily:     uint8(unix.AF_INET),
	}

	queue, err := nfqueue.Open(&cfg)
	if err != nil {
		return nil, err
	}

	if err := queue.SetOption(netlink.NoENOBUFS, true); err != nil {
		_ = queue.Close()
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	ad := &NFQueueAdapter{
		queue: queue,
		recv:  make(chan *packet.Packet, 1024),
		errs:  make(chan error, 1),
		ctx:   ctx,
		stop:  cancel,
		rawFD: -1,
		mark:  opts.Mark,
	}

	if err := ad.openRawSocket(); err != nil {
		_ = queue.Close()
		return nil, err
	}

	if err := queue.RegisterWithErrorFunc(ctx, ad.onPacket, ad.onError); err != nil {
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
		return n.setVerdict(pkt.NFQID, nfqueue.NfAccept)
	}
	return n.inject(pkt)
}

func (n *NFQueueAdapter) Drop(ctx context.Context, pkt *packet.Packet) error {
	if pkt == nil {
		return nil
	}
	if pkt.Source != packet.SourceCaptured {
		return nil
	}
	return n.setVerdict(pkt.NFQID, nfqueue.NfDrop)
}

func (n *NFQueueAdapter) CalcChecksums(pkt *packet.Packet) error {
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

// Flush releases any packets already delivered to the adapter recv buffer by
// accepting them (fail-open). It also stops new callbacks best-effort so no
// additional packets are enqueued while draining.
func (n *NFQueueAdapter) Flush(ctx context.Context) error {
	if n.queue == nil {
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
			if err := n.setVerdict(pkt.NFQID, nfqueue.NfAccept); err != nil {
				errs = append(errs, err)
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
		if n.queue != nil {
			if e := n.queue.Close(); e != nil {
				err = e
			}
		}
		if n.rawFD >= 0 {
			_ = unix.Close(n.rawFD)
			n.rawFD = -1
		}
	})
	return err
}

func (n *NFQueueAdapter) onPacket(a nfqueue.Attribute) int {
	n.inFlight.Add(1)
	defer n.inFlight.Add(-1)

	if a.Payload == nil || a.PacketID == nil {
		return 0
	}
	id := *a.PacketID

	// If we're flushing/shutting down, do not enqueue. Immediately fail-open.
	if n.flushing.Load() || n.ctx.Err() != nil {
		_ = n.setVerdict(id, nfqueue.NfAccept)
		return 0
	}

	payload := append([]byte(nil), (*a.Payload)...)
	pkt := &packet.Packet{
		Data:   payload,
		Source: packet.SourceCaptured,
		NFQID:  id,
	}

	select {
	case n.recv <- pkt:
		return 0
	default:
		_ = n.setVerdict(id, nfqueue.NfAccept)
		return 0
	}
}

func (n *NFQueueAdapter) onError(err error) int {
	if opErr, ok := err.(*netlink.OpError); ok {
		if opErr.Timeout() || opErr.Temporary() {
			return 0
		}
	}
	select {
	case n.errs <- err:
	default:
	}
	return -1
}

func (n *NFQueueAdapter) setVerdict(id uint32, verdict int) error {
	if n.queue == nil {
		return ErrNotImplemented
	}
	return n.queue.SetVerdict(id, verdict)
}

func (n *NFQueueAdapter) openRawSocket() error {
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_RAW, unix.IPPROTO_RAW)
	if err != nil {
		return err
	}
	if err := unix.SetsockoptInt(fd, unix.IPPROTO_IP, unix.IP_HDRINCL, 1); err != nil {
		_ = unix.Close(fd)
		return err
	}
	if n.mark != 0 {
		if err := unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_MARK, int(n.mark)); err != nil {
			_ = unix.Close(fd)
			return err
		}
	}
	n.rawFD = fd
	return nil
}

func (n *NFQueueAdapter) inject(pkt *packet.Packet) error {
	if n.rawFD < 0 {
		return ErrNotImplemented
	}
	if len(pkt.Data) < 20 {
		return nil
	}

	var dst unix.SockaddrInet4
	copy(dst.Addr[:], pkt.Data[16:20])
	return unix.Sendto(n.rawFD, pkt.Data, 0, &dst)
}
