//go:build !windows && !linux

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "gov-pass tray UI is supported on Windows and Linux only.")
	os.Exit(1)
}
