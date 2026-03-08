//go:build windows

package main

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	windowsStdoutHandle = func() windows.Handle {
		return windows.Handle(os.Stdout.Fd())
	}
	windowsGetConsoleMode             = windows.GetConsoleMode
	windowsSetConsoleMode             = windows.SetConsoleMode
	windowsGetConsoleScreenBufferInfo = windows.GetConsoleScreenBufferInfo
	windowsSetCursorPosition          = windows.SetConsoleCursorPosition
	windowsFillConsoleCharacters      = fillConsoleOutputCharacter
	windowsFillConsoleAttributes      = fillConsoleOutputAttribute
)

var (
	windowsKernel32                 = windows.NewLazySystemDLL("kernel32.dll")
	procFillConsoleOutputCharacterW = windowsKernel32.NewProc("FillConsoleOutputCharacterW")
	procFillConsoleOutputAttribute  = windowsKernel32.NewProc("FillConsoleOutputAttribute")
)

func clearWindowsConsole() error {
	handle := windowsStdoutHandle()
	if handle == 0 || handle == windows.InvalidHandle {
		return fmt.Errorf("stdout is not attached to a console")
	}
	if enableWindowsVirtualTerminal(handle) {
		fmt.Print("\033[H\033[2J")
		return nil
	}
	return clearWindowsConsoleBuffer(handle)
}

func enableWindowsVirtualTerminal(handle windows.Handle) bool {
	var mode uint32
	if err := windowsGetConsoleMode(handle, &mode); err != nil {
		return false
	}
	if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0 {
		return true
	}
	return windowsSetConsoleMode(handle, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
}

func clearWindowsConsoleBuffer(handle windows.Handle) error {
	var info windows.ConsoleScreenBufferInfo
	if err := windowsGetConsoleScreenBufferInfo(handle, &info); err != nil {
		return err
	}

	width := int32(info.Size.X)
	height := int32(info.Size.Y)
	if width <= 0 || height <= 0 {
		return nil
	}

	cells := uint32(width * height)
	origin := windows.Coord{X: 0, Y: 0}
	var written uint32

	if err := windowsFillConsoleCharacters(handle, ' ', cells, origin, &written); err != nil {
		return err
	}
	if err := windowsFillConsoleAttributes(handle, info.Attributes, cells, origin, &written); err != nil {
		return err
	}
	return windowsSetCursorPosition(handle, origin)
}

func fillConsoleOutputCharacter(handle windows.Handle, char uint16, length uint32, coord windows.Coord, written *uint32) error {
	r1, _, e1 := procFillConsoleOutputCharacterW.Call(
		uintptr(handle),
		uintptr(char),
		uintptr(length),
		uintptr(coordToUint32(coord)),
		uintptr(unsafe.Pointer(written)),
	)
	if r1 == 0 {
		if e1 != windows.ERROR_SUCCESS {
			return e1
		}
		return syscall.EINVAL
	}
	return nil
}

func fillConsoleOutputAttribute(handle windows.Handle, attr uint16, length uint32, coord windows.Coord, written *uint32) error {
	r1, _, e1 := procFillConsoleOutputAttribute.Call(
		uintptr(handle),
		uintptr(attr),
		uintptr(length),
		uintptr(coordToUint32(coord)),
		uintptr(unsafe.Pointer(written)),
	)
	if r1 == 0 {
		if e1 != windows.ERROR_SUCCESS {
			return e1
		}
		return syscall.EINVAL
	}
	return nil
}

func coordToUint32(coord windows.Coord) uint32 {
	return *(*uint32)(unsafe.Pointer(&coord))
}
