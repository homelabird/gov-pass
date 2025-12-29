package driver

import (
	"os"
	"path/filepath"
	"strings"
)

// PrependPath adds dir to PATH for the current process if not already present.
func PrependPath(dir string) error {
	if dir == "" {
		return nil
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if _, err := os.Stat(abs); err != nil {
		return err
	}
	pathEnv := os.Getenv("PATH")
	parts := strings.Split(pathEnv, string(os.PathListSeparator))
	for _, part := range parts {
		if strings.EqualFold(part, abs) {
			return nil
		}
	}
	newPath := abs
	if pathEnv != "" {
		newPath = abs + string(os.PathListSeparator) + pathEnv
	}
	return os.Setenv("PATH", newPath)
}
