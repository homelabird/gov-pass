//go:build windows

package adapter

import (
	"context"
	"os"
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
)

type WinDivertAdapter struct {
	handle syscall.Handle
}

var (
	winDivertDLL  = syscall.NewLazyDLL("WinDivert.dll")
	procOpen      = winDivertDLL.NewProc("WinDivertOpen")
	procRecv      = winDivertDLL.NewProc("WinDivertRecv")
	procSend      = winDivertDLL.NewProc("WinDivertSend")
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

	ad := &WinDivertAdapter{handle: syscall.Handle(handle)}
	if err := ad.applyOptions(opts); err != nil {
		_ = ad.Close()
		return nil, err
	}
	return ad, nil
}

func (w *WinDivertAdapter) Recv(ctx context.Context) (*packet.Packet, error) {
	if w.handle == 0 {
		return nil, ErrNotImplemented
	}

	buf := make([]byte, maxPacketSize)
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
		return nil, os.NewSyscallError("WinDivertRecv", err)
	}
	if recvLen == 0 {
		return nil, nil
	}
	return &packet.Packet{
		Data: buf[:recvLen],
		Addr: addr,
		Source: packet.SourceCaptured,
	}, nil
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
	if w.handle == 0 {
		return nil
	}
	r1, _, err := procClose.Call(uintptr(w.handle))
	if r1 == 0 {
		return os.NewSyscallError("WinDivertClose", err)
	}
	w.handle = 0
	return nil
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
