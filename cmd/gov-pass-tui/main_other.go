//go:build !windows && !linux && !freebsd

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "gov-pass TUI is supported on Linux, FreeBSD, and Windows only.")
	os.Exit(1)
}
