package main

import (
	"os"
	"strconv"
)

type terminalSize struct {
	Rows int
	Cols int
}

func currentTerminalSize() terminalSize {
	if size, ok := platformTerminalSize(os.Stdout.Fd()); ok {
		return normalizeTerminalSize(size)
	}
	if size, ok := envTerminalSize(); ok {
		return normalizeTerminalSize(size)
	}
	return terminalSize{Rows: 28, Cols: 96}
}

func envTerminalSize() (terminalSize, bool) {
	rows, rowErr := strconv.Atoi(os.Getenv("LINES"))
	cols, colErr := strconv.Atoi(os.Getenv("COLUMNS"))
	if rowErr != nil || colErr != nil {
		return terminalSize{}, false
	}
	return terminalSize{Rows: rows, Cols: cols}, true
}

func normalizeTerminalSize(size terminalSize) terminalSize {
	if size.Rows < 8 {
		size.Rows = 8
	}
	if size.Cols < 16 {
		size.Cols = 16
	}
	return size
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
