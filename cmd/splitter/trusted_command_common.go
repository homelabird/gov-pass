package main

import (
	"path/filepath"
	"strings"
)

func isBareTrustedCommandName(name string) bool {
	name = strings.TrimSpace(name)
	return name != "" && name != "." && name != ".." && filepath.Base(name) == name && filepath.VolumeName(name) == ""
}
