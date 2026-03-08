//go:build !windows && !linux && !freebsd

package main

func platformTerminalSize(uintptr) (terminalSize, bool) {
	return terminalSize{}, false
}
