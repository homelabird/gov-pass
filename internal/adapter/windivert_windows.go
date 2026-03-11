//go:build windows

package adapter

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
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
	mu     sync.RWMutex
	handle syscall.Handle

	recv chan *packet.Packet
	errs chan error
	ctx  context.Context
	stop context.CancelFunc

	recvLoopCancel context.CancelFunc
	recvLoopDone   chan struct{}
	bufPool        sync.Pool

	closeOnce sync.Once
}

var (
	winDivertDLLMu   sync.Mutex
	winDivertDLLPath string
	winDivertDLL     *syscall.LazyDLL
	procOpen         *syscall.LazyProc
	procRecv         *syscall.LazyProc
	procSend         *syscall.LazyProc
	procShutdown     *syscall.LazyProc
	procClose        *syscall.LazyProc
	procChecksums    *syscall.LazyProc
	procSetParam     *syscall.LazyProc
)

func ConfigureWinDivertDLL(path string) error {
	abs, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil {
		return err
	}
	if abs == "" {
		return fmt.Errorf("WinDivert.dll path is empty")
	}
	if _, err := os.Stat(abs); err != nil {
		return err
	}

	winDivertDLLMu.Lock()
	defer winDivertDLLMu.Unlock()
	if strings.EqualFold(winDivertDLLPath, abs) && procOpen != nil {
		return nil
	}

	dll := syscall.NewLazyDLL(abs)
	openProc := dll.NewProc("WinDivertOpen")
	if err := openProc.Find(); err != nil {
		return err
	}

	winDivertDLLPath = abs
	winDivertDLL = dll
	procOpen = openProc
	procRecv = dll.NewProc("WinDivertRecv")
	procSend = dll.NewProc("WinDivertSend")
	procShutdown = dll.NewProc("WinDivertShutdown")
	procClose = dll.NewProc("WinDivertClose")
	procChecksums = dll.NewProc("WinDivertHelperCalcChecksums")
	procSetParam = dll.NewProc("WinDivertSetParam")
	return nil
}

func NewWinDivert(filter string, opts WinDivertOptions) (*WinDivertAdapter, error) {
	if procOpen == nil {
		return nil, fmt.Errorf("WinDivert.dll is not configured")
	}
	handle, err := openWinDivertHandle(filter)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	ad := &WinDivertAdapter{
		handle: syscall.Handle(handle),
		recv:   make(chan *packet.Packet, 1024),
		errs:   make(chan error, 1),
		ctx:    ctx,
		stop:   cancel,
	}
	ad.bufPool.New = func() any {
		return make([]byte, maxPacketSize)
	}
	if err := ad.applyOptionsToHandle(syscall.Handle(handle), opts); err != nil {
		_ = ad.Close()
		return nil, err
	}
	ad.startRecvLoopForHandle(syscall.Handle(handle))
	return ad, nil
}

func (w *WinDivertAdapter) Recv(ctx context.Context) (*packet.Packet, error) {
	if w.currentHandle() == 0 {
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
	handle := w.currentHandle()
	if handle == 0 {
		return ErrNotImplemented
	}
	if pkt == nil || len(pkt.Data) == 0 {
		return nil
	}
	var sendLen uint32
	r1, _, err := procSend.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(&pkt.Data[0])),
		uintptr(len(pkt.Data)),
		uintptr(unsafe.Pointer(&sendLen)),
		uintptr(unsafe.Pointer(&pkt.Addr)),
	)
	if r1 == 0 {
		return os.NewSyscallError("WinDivertSend", err)
	}
	pkt.Release()
	return nil
}

func (w *WinDivertAdapter) Drop(ctx context.Context, pkt *packet.Packet) error {
	if pkt != nil {
		pkt.Release()
	}
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
		handle, loopCancel, loopDone := w.snapshotLoopState()
		if loopCancel != nil {
			loopCancel()
		}
		if handle == 0 {
			return
		}
		_ = shutdownRecvHandle(handle)
		if loopDone != nil {
			<-loopDone
		}
		closeErr = closeWinDivertHandle(handle)
		w.clearHandle(handle)
	})
	return closeErr
}

func (w *WinDivertAdapter) applyOptions(opts WinDivertOptions) error {
	handle := w.currentHandle()
	if handle == 0 {
		return ErrNotImplemented
	}
	return w.applyOptionsToHandle(handle, opts)
}

func (w *WinDivertAdapter) applyOptionsToHandle(handle syscall.Handle, opts WinDivertOptions) error {
	if opts.QueueLen > 0 {
		if err := setParamHandle(handle, paramQueueLen, opts.QueueLen); err != nil {
			return err
		}
	}
	if opts.QueueTime > 0 {
		if err := setParamHandle(handle, paramQueueTime, opts.QueueTime); err != nil {
			return err
		}
	}
	if opts.QueueSize > 0 {
		if err := setParamHandle(handle, paramQueueSize, opts.QueueSize); err != nil {
			return err
		}
	}
	return nil
}

// UpdateOptions applies queue parameter updates to an already-open handle.
// Callers that need to revert queue values to driver defaults should use
// Reopen, which swaps in a freshly opened handle.
func (w *WinDivertAdapter) UpdateOptions(opts WinDivertOptions) error {
	return w.applyOptions(opts)
}

// Reopen swaps the current WinDivert handle with a freshly opened one so
// restart-only handle state such as filter changes or queue default resets can
// be applied without restarting the entire service.
func (w *WinDivertAdapter) Reopen(filter string, opts WinDivertOptions) error {
	if w.ctx.Err() != nil {
		return w.ctx.Err()
	}

	newHandle, err := openWinDivertHandle(filter)
	if err != nil {
		return err
	}
	if err := w.applyOptionsToHandle(syscall.Handle(newHandle), opts); err != nil {
		_ = closeWinDivertHandle(syscall.Handle(newHandle))
		return err
	}

	loopCtx, loopCancel := context.WithCancel(w.ctx)
	loopDone := make(chan struct{})
	go w.recvLoop(loopCtx, loopDone, syscall.Handle(newHandle))

	oldHandle, oldLoopCancel, oldLoopDone := w.swapHandle(syscall.Handle(newHandle), loopCancel, loopDone)
	if oldLoopCancel != nil {
		oldLoopCancel()
	}
	if oldHandle != 0 {
		_ = shutdownRecvHandle(oldHandle)
	}
	if oldLoopDone != nil {
		select {
		case <-oldLoopDone:
		case <-time.After(2 * time.Second):
		}
	}
	if oldHandle != 0 {
		return closeWinDivertHandle(oldHandle)
	}
	return nil
}

// Flush releases any packets already delivered to the adapter recv buffer by
// reinjecting them (fail-open). It also attempts to shutdown the receive side
// so the recv goroutine exits promptly.
func (w *WinDivertAdapter) Flush(ctx context.Context) error {
	handle, loopCancel, loopDone := w.snapshotLoopState()
	if handle == 0 {
		return nil
	}

	if loopCancel != nil {
		// Cancel recv loop; we still keep the handle open for reinjection.
		loopCancel()
	}
	// Best-effort unblock WinDivertRecv so the goroutine can exit quickly.
	_ = shutdownRecvHandle(handle)

	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-loopDone:
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
	handle := w.currentHandle()
	if handle == 0 {
		return ErrNotImplemented
	}
	return setParamHandle(handle, param, value)
}

func setParamHandle(handle syscall.Handle, param uint32, value uint64) error {
	r1, _, err := procSetParam.Call(
		uintptr(handle),
		uintptr(param),
		uintptr(value),
	)
	if r1 == 0 {
		return os.NewSyscallError("WinDivertSetParam", err)
	}
	return nil
}

func (w *WinDivertAdapter) shutdownRecv() error {
	handle := w.currentHandle()
	if handle == 0 {
		return ErrNotImplemented
	}
	return shutdownRecvHandle(handle)
}

func shutdownRecvHandle(handle syscall.Handle) error {
	r1, _, err := procShutdown.Call(
		uintptr(handle),
		uintptr(windivertShutdownRecv),
	)
	if r1 == 0 {
		return os.NewSyscallError("WinDivertShutdown", err)
	}
	return nil
}

func (w *WinDivertAdapter) startRecvLoopForHandle(handle syscall.Handle) {
	loopCtx, loopCancel := context.WithCancel(w.ctx)
	loopDone := make(chan struct{})
	w.mu.Lock()
	w.recvLoopCancel = loopCancel
	w.recvLoopDone = loopDone
	w.mu.Unlock()
	go w.recvLoop(loopCtx, loopDone, handle)
}

func (w *WinDivertAdapter) recvLoop(loopCtx context.Context, loopDone chan struct{}, handle syscall.Handle) {
	defer close(loopDone)

	buf := make([]byte, maxPacketSize)
	for {
		select {
		case <-loopCtx.Done():
			return
		default:
		}
		if handle == 0 {
			return
		}

		var addr packet.Address
		var recvLen uint32
		r1, _, err := procRecv.Call(
			uintptr(handle),
			uintptr(unsafe.Pointer(&buf[0])),
			uintptr(len(buf)),
			uintptr(unsafe.Pointer(&recvLen)),
			uintptr(unsafe.Pointer(&addr)),
		)
		if r1 == 0 {
			if loopCtx.Err() != nil || w.ctx.Err() != nil {
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
		if loopCtx.Err() != nil || w.ctx.Err() != nil {
			var sendLen uint32
			r2, _, _ := procSend.Call(
				uintptr(handle),
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
				uintptr(handle),
				uintptr(unsafe.Pointer(&buf[0])),
				uintptr(recvLen),
				uintptr(unsafe.Pointer(&sendLen)),
				uintptr(unsafe.Pointer(&addr)),
			)
			if r2 == 0 {
				if loopCtx.Err() != nil || w.ctx.Err() != nil {
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

		payload, backing := copyIntoPoolBuffer(&w.bufPool, buf[:recvLen])
		pkt := &packet.Packet{
			Data:   payload,
			Addr:   addr,
			Source: packet.SourceCaptured,
		}
		pkt.SetDataPool(&w.bufPool, backing)

		select {
		case w.recv <- pkt:
		case <-loopCtx.Done():
			// Deterministic fail-open on shutdown: do not enqueue after cancel.
			var sendLen uint32
			r2, _, _ := procSend.Call(
				uintptr(handle),
				uintptr(unsafe.Pointer(&payload[0])),
				uintptr(len(payload)),
				uintptr(unsafe.Pointer(&sendLen)),
				uintptr(unsafe.Pointer(&addr)),
			)
			if r2 == 0 {
				pkt.Release()
				return
			}
			pkt.Release()
			return
		default:
			// Channel filled after the check above; fail-open by reinjecting.
			var sendLen uint32
			r2, _, sendErr := procSend.Call(
				uintptr(handle),
				uintptr(unsafe.Pointer(&payload[0])),
				uintptr(len(payload)),
				uintptr(unsafe.Pointer(&sendLen)),
				uintptr(unsafe.Pointer(&addr)),
			)
			if r2 == 0 {
				pkt.Release()
				if loopCtx.Err() != nil || w.ctx.Err() != nil {
					return
				}
				select {
				case w.errs <- os.NewSyscallError("WinDivertSend", sendErr):
				default:
				}
				return
			}
			pkt.Release()
		}
	}
}

func openWinDivertHandle(filter string) (uintptr, error) {
	filterPtr, err := syscall.BytePtrFromString(filter)
	if err != nil {
		return 0, err
	}
	handle, _, callErr := procOpen.Call(
		uintptr(unsafe.Pointer(filterPtr)),
		uintptr(windivertLayerNetwork),
		uintptr(int16(0)),
		uintptr(uint64(0)),
	)
	if handle == 0 || handle == ^uintptr(0) {
		return 0, os.NewSyscallError("WinDivertOpen", callErr)
	}
	return handle, nil
}

func closeWinDivertHandle(handle syscall.Handle) error {
	if handle == 0 {
		return nil
	}
	r1, _, err := procClose.Call(uintptr(handle))
	if r1 == 0 {
		return os.NewSyscallError("WinDivertClose", err)
	}
	return nil
}

func (w *WinDivertAdapter) currentHandle() syscall.Handle {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.handle
}

func (w *WinDivertAdapter) snapshotLoopState() (syscall.Handle, context.CancelFunc, chan struct{}) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.handle, w.recvLoopCancel, w.recvLoopDone
}

func (w *WinDivertAdapter) swapHandle(handle syscall.Handle, loopCancel context.CancelFunc, loopDone chan struct{}) (syscall.Handle, context.CancelFunc, chan struct{}) {
	w.mu.Lock()
	defer w.mu.Unlock()
	oldHandle := w.handle
	oldLoopCancel := w.recvLoopCancel
	oldLoopDone := w.recvLoopDone
	w.handle = handle
	w.recvLoopCancel = loopCancel
	w.recvLoopDone = loopDone
	return oldHandle, oldLoopCancel, oldLoopDone
}

func (w *WinDivertAdapter) clearHandle(handle syscall.Handle) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.handle == handle {
		w.handle = 0
	}
	if w.recvLoopDone != nil {
		select {
		case <-w.recvLoopDone:
			w.recvLoopDone = nil
			w.recvLoopCancel = nil
		default:
		}
	}
}
