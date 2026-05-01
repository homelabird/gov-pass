package packaging_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitterCLIsUseCheckedEngineConstructor(t *testing.T) {
	root := repoRoot(t)
	for _, file := range []string{
		filepath.Join("cmd", "splitter", "main_linux.go"),
		filepath.Join("cmd", "splitter", "main_freebsd.go"),
		filepath.Join("cmd", "splitter", "main_windows.go"),
	} {
		t.Run(file, func(t *testing.T) {
			text := readTextFile(t, filepath.Join(root, file))
			if !strings.Contains(text, `engine.NewChecked(cfg, ad)`) {
				t.Fatalf("%s does not use checked engine construction", file)
			}
			if strings.Contains(text, `engine.New(cfg, ad)`) {
				t.Fatalf("%s still uses panic-based engine construction", file)
			}
		})
	}
}
