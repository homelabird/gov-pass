//go:build linux || freebsd

package main

import "golang.org/x/sys/unix"

func platformTerminalSize(fd uintptr) (terminalSize, bool) {
	ws, err := unix.IoctlGetWinsize(int(fd), unix.TIOCGWINSZ)
	if err != nil || ws == nil {
		return terminalSize{}, false
	}
	if ws.Row == 0 || ws.Col == 0 {
		return terminalSize{}, false
	}
	return terminalSize{
		Rows: int(ws.Row),
		Cols: int(ws.Col),
	}, true
}
