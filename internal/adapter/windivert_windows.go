//go:build windows

package adapter

import (
	"context"
	"os"
	"sync"
	"syscall"
	"unsafe"

	"fk-gov/internal/packet"
)

const (
	windivertLayerNetwork = 0
	maxPacketSize         = 0xFFFF

	paramQueueLen  = 0
	paramQueueTime = 1
	paramQueueSize = 2

	windivertShutdownRecv = 1
)

type WinDivertAdapter struct {
	handle syscall.Handle

	recv chan *packet.Packet
	errs chan error
	ctx  context.Context
	stop context.CancelFunc
	done chan struct{}

	closeOnce sync.Once
}

var (
	winDivertDLL  = syscall.NewLazyDLL("WinDivert.dll")
	procOpen      = winDivertDLL.NewProc("WinDivertOpen")
	procRecv      = winDivertDLL.NewProc("WinDivertRecv")
	procSend      = winDivertDLL.NewProc("WinDivertSend")
	procShutdown  = winDivertDLL.NewProc("WinDivertShutdown")
	procClose     = winDivertDLL.NewProc("WinDivertClose")
	procChecksums = winDivertDLL.NewProc("WinDivertHelperCalcChecksums")
	procSetParam  = winDivertDLL.NewProc("WinDivertSetParam")
)

func NewWinDivert(filter string, opts WinDivertOptions) (*WinDivertAdapter, error) {
	filterPtr, err := syscall.BytePtrFromString(filter)
	if err != nil {
		return nil, err
	}
	handle, _, callErr := procOpen.Call(
		uintptr(unsafe.Pointer(filterPtr)),
		uintptr(windivertLayerNetwork),
		uintptr(int16(0)),
		uintptr(uint64(0)),
	)
	if handle == 0 || handle == ^uintptr(0) {
		return nil, os.NewSyscallError("WinDivertOpen", callErr)
	}

	ctx, cancel := context.WithCancel(context.Background())
	ad := &WinDivertAdapter{
		handle: syscall.Handle(handle),
		recv:   make(chan *packet.Packet, 1024),
		errs:   make(chan error, 1),
		ctx:    ctx,
		stop:   cancel,
		done:   make(chan struct{}),
	}
	if err := ad.applyOptions(opts); err != nil {
		_ = ad.Close()
		return nil, err
	}
	ad.startRecvLoop()
	return ad, nil
}

func (w *WinDivertAdapter) Recv(ctx context.Context) (*packet.Packet, error) {
	if w.handle == 0 {
		return nil, ErrNotImplemented
	}
	select {
	case pkt := <-w.recv:
		return pkt, nil
	case err := <-w.errs:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-w.ctx.Done():
		return nil, w.ctx.Err()
	}
}

func (w *WinDivertAdapter) Send(ctx context.Context, pkt *packet.Packet) error {
	if w.handle == 0 {
		return ErrNotImplemented
	}
	if pkt == nil || len(pkt.Data) == 0 {
		return nil
	}
	var sendLen uint32
	r1, _, err := procSend.Call(
		uintptr(w.handle),
		uintptr(unsafe.Pointer(&pkt.Data[0])),
		uintptr(len(pkt.Data)),
		uintptr(unsafe.Pointer(&sendLen)),
		uintptr(unsafe.Pointer(&pkt.Addr)),
	)
	if r1 == 0 {
		return os.NewSyscallError("WinDivertSend", err)
	}
	return nil
}

func (w *WinDivertAdapter) Drop(ctx context.Context, pkt *packet.Packet) error {
	return nil
}

func (w *WinDivertAdapter) CalcChecksums(pkt *packet.Packet) error {
	if pkt == nil || len(pkt.Data) == 0 {
		return nil
	}
	r1, _, err := procChecksums.Call(
		uintptr(unsafe.Pointer(&pkt.Data[0])),
		uintptr(len(pkt.Data)),
		uintptr(uint64(0)),
	)
	if r1 == 0 {
		return os.NewSyscallError("WinDivertHelperCalcChecksums", err)
	}
	return nil
}

func (w *WinDivertAdapter) Close() error {
	var closeErr error
	w.closeOnce.Do(func() {
		if w.stop != nil {
			w.stop()
		}
		if w.handle == 0 {
			return
		}
		r1, _, err := procClose.Call(uintptr(w.handle))
		if r1 == 0 {
			closeErr = os.NewSyscallError("WinDivertClose", err)
		}
		w.handle = 0
	})
	return closeErr
}

func (w *WinDivertAdapter) applyOptions(opts WinDivertOptions) error {
	if w.handle == 0 {
		return ErrNotImplemented
	}
	if opts.QueueLen > 0 {
		if err := w.setParam(paramQueueLen, opts.QueueLen); err != nil {
			return err
		}
	}
	if opts.QueueTime > 0 {
		if err := w.setParam(paramQueueTime, opts.QueueTime); err != nil {
			return err
		}
	}
	if opts.QueueSize > 0 {
		if err := w.setParam(paramQueueSize, opts.QueueSize); err != nil {
			return err
		}
	}
	return nil
}

// UpdateOptions applies queue parameter updates to an already-open handle.
// Note: WinDivert does not support "reset to default" via SetParam; callers
// should treat 0 values as "restart required".
func (w *WinDivertAdapter) UpdateOptions(opts WinDivertOptions) error {
	return w.applyOptions(opts)
}

// Flush releases any packets already delivered to the adapter recv buffer by
// reinjecting them (fail-open). It also attempts to shutdown the receive side
// so the recv goroutine exits promptly.
func (w *WinDivertAdapter) Flush(ctx context.Context) error {
	if w.handle == 0 {
		return nil
	}

	if w.stop != nil {
		// Cancel recv loop; we still keep the handle open for reinjection.
		w.stop()
	}
	// Best-effort unblock WinDivertRecv so the goroutine can exit quickly.
	_ = w.shutdownRecv()

	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-w.done:
	case <-ctx.Done():
		// Continue draining what we already have, but surface timeout/cancel.
	}

	var firstErr error
	for {
		select {
		case pkt := <-w.recv:
			if pkt == nil {
				continue
			}
			if err := w.Send(context.Background(), pkt); err != nil && firstErr == nil {
				firstErr = err
			}
		default:
			if firstErr == nil && ctx.Err() != nil {
				return ctx.Err()
			}
			return firstErr
		}
	}
}

func (w *WinDivertAdapter) setParam(param uint32, value uint64) error {
	r1, _, err := procSetParam.Call(
		uintptr(w.handle),
		uintptr(param),
		uintptr(value),
	)
	if r1 == 0 {
		return os.NewSyscallError("WinDivertSetParam", err)
	}
	return nil
}

func (w *WinDivertAdapter) shutdownRecv() error {
	if w.handle == 0 {
		return ErrNotImplemented
	}
	r1, _, err := procShutdown.Call(
		uintptr(w.handle),
		uintptr(windivertShutdownRecv),
	)
	if r1 == 0 {
		return os.NewSyscallError("WinDivertShutdown", err)
	}
	return nil
}

func (w *WinDivertAdapter) startRecvLoop() {
	go func() {
		defer close(w.done)

		buf := make([]byte, maxPacketSize)
		for {
			select {
			case <-w.ctx.Done():
				return
			default:
			}
			if w.handle == 0 {
				return
			}

			var addr packet.Address
			var recvLen uint32
			r1, _, err := procRecv.Call(
				uintptr(w.handle),
				uintptr(unsafe.Pointer(&buf[0])),
				uintptr(len(buf)),
				uintptr(unsafe.Pointer(&recvLen)),
				uintptr(unsafe.Pointer(&addr)),
			)
			if r1 == 0 {
				if w.ctx.Err() != nil {
					return
				}
				select {
				case w.errs <- os.NewSyscallError("WinDivertRecv", err):
				default:
				}
				return
			}
			if recvLen == 0 {
				continue
			}

			// If we're already shutting down, fail-open this packet directly and exit.
			// Avoid enqueueing after cancellation, which can leave packets stuck in
			// the buffer until Close() drops the handle.
			if w.ctx.Err() != nil {
				var sendLen uint32
				r2, _, _ := procSend.Call(
					uintptr(w.handle),
					uintptr(unsafe.Pointer(&buf[0])),
					uintptr(recvLen),
					uintptr(unsafe.Pointer(&sendLen)),
					uintptr(unsafe.Pointer(&addr)),
				)
				if r2 == 0 {
					return
				}
				return
			}

			// Fast-path: if the channel is full, fail-open by immediately reinjecting.
			if len(w.recv) == cap(w.recv) {
				var sendLen uint32
				r2, _, sendErr := procSend.Call(
					uintptr(w.handle),
					uintptr(unsafe.Pointer(&buf[0])),
					uintptr(recvLen),
					uintptr(unsafe.Pointer(&sendLen)),
					uintptr(unsafe.Pointer(&addr)),
				)
				if r2 == 0 {
					if w.ctx.Err() != nil {
						return
					}
					select {
					case w.errs <- os.NewSyscallError("WinDivertSend", sendErr):
					default:
					}
					return
				}
				continue
			}

			payload := make([]byte, recvLen)
			copy(payload, buf[:recvLen])
			pkt := &packet.Packet{
				Data:   payload,
				Addr:   addr,
				Source: packet.SourceCaptured,
			}

			select {
			case w.recv <- pkt:
			case <-w.ctx.Done():
				// Deterministic fail-open on shutdown: do not enqueue after cancel.
				var sendLen uint32
				r2, _, _ := procSend.Call(
					uintptr(w.handle),
					uintptr(unsafe.Pointer(&payload[0])),
					uintptr(len(payload)),
					uintptr(unsafe.Pointer(&sendLen)),
					uintptr(unsafe.Pointer(&addr)),
				)
				if r2 == 0 {
					return
				}
				return
			default:
				// Channel filled after the check above; fail-open by reinjecting.
				var sendLen uint32
				r2, _, sendErr := procSend.Call(
					uintptr(w.handle),
					uintptr(unsafe.Pointer(&payload[0])),
					uintptr(len(payload)),
					uintptr(unsafe.Pointer(&sendLen)),
					uintptr(unsafe.Pointer(&addr)),
				)
				if r2 == 0 {
					if w.ctx.Err() != nil {
						return
					}
					select {
					case w.errs <- os.NewSyscallError("WinDivertSend", sendErr):
					default:
					}
					return
				}
			}
		}
	}()
}
