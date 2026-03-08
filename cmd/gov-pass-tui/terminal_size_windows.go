//go:build windows

package main

import "golang.org/x/sys/windows"

func platformTerminalSize(fd uintptr) (terminalSize, bool) {
	var info windows.ConsoleScreenBufferInfo
	if err := windows.GetConsoleScreenBufferInfo(windows.Handle(fd), &info); err != nil {
		return terminalSize{}, false
	}

	rows := int(info.Window.Bottom-info.Window.Top) + 1
	cols := int(info.Window.Right-info.Window.Left) + 1
	if rows <= 0 || cols <= 0 {
		return terminalSize{}, false
	}

	return terminalSize{
		Rows: rows,
		Cols: cols,
	}, true
}
