//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestEffectiveWindowsConfig_TrimsCLIWinDivertFilter(t *testing.T) {
	_, wc, err := effectiveWindowsConfig(
		windowsCLIArgs{Filter: " outbound and ipv6 and tcp.DstPort == 443 "},
		map[string]bool{"filter": true},
		false,
	)
	if err != nil {
		t.Fatalf("effective config: %v", err)
	}
	if wc.Filter != "outbound and ipv6 and tcp.DstPort == 443" {
		t.Fatalf("Filter = %q, want trimmed", wc.Filter)
	}
}

func writeWindowsTempJSON(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cfg.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestEffectiveWindowsConfig_ServiceDefaultCreateReadsBackConfig(t *testing.T) {
	programData := t.TempDir()
	origKnownFolder := windowsKnownFolderPath
	origEnsureDir := ensureSecureWindowsConfigDir
	origHardenFile := hardenWindowsConfigFileACL
	origWriteConfig := writeWindowsJSONConfigIfMissingFile
	t.Cleanup(func() {
		windowsKnownFolderPath = origKnownFolder
		ensureSecureWindowsConfigDir = origEnsureDir
		hardenWindowsConfigFileACL = origHardenFile
		writeWindowsJSONConfigIfMissingFile = origWriteConfig
	})

	windowsKnownFolderPath = func(folderID *windows.KNOWNFOLDERID, flags uint32) (string, error) {
		switch folderID {
		case windows.FOLDERID_ProgramData:
			return programData, nil
		case windows.FOLDERID_ProgramFiles:
			return filepath.Join(programData, "Program Files"), nil
		default:
			return "", errors.New("unexpected folder")
		}
	}
	ensureSecureWindowsConfigDir = func(dir string) error {
		return os.MkdirAll(dir, 0o755)
	}
	hardenWindowsConfigFileACL = func(path string) error {
		return nil
	}
	writeWindowsJSONConfigIfMissingFile = func(path string, cfg windowsJSONConfig) error {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		return os.WriteFile(path, []byte(`{
  "engine": {
    "split_chunk": 17,
    "stats_interval": "7s"
  }
}`), 0o644)
	}

	cfg, wc, err := effectiveWindowsConfig(windowsCLIArgs{}, nil, true)
	if err != nil {
		t.Fatalf("effective config: %v", err)
	}
	if cfg.SplitChunk != 17 {
		t.Fatalf("SplitChunk = %d, want config value 17", cfg.SplitChunk)
	}
	if wc.StatsInterval != 7*time.Second {
		t.Fatalf("StatsInterval = %s, want config value 7s", wc.StatsInterval)
	}
}

func TestWriteWindowsJSONConfigIfMissingRejectsReparseBeforeMkdir(t *testing.T) {
	installWindowsPathTestStubs(t,
		map[string]bool{
			filepath.Clean(`C:\`):            true,
			filepath.Clean(`C:\ProgramData`): true,
		},
		map[string]bool{
			filepath.Clean(`C:\ProgramData\gov-pass`): true,
		},
	)

	origMkdirAll := windowsMkdirAll
	mkdirCalled := false
	windowsMkdirAll = func(string, os.FileMode) error {
		mkdirCalled = true
		return nil
	}
	t.Cleanup(func() { windowsMkdirAll = origMkdirAll })

	err := writeWindowsJSONConfigIfMissing(`C:\ProgramData\gov-pass\config.json`, windowsJSONConfig{})
	if err == nil || !strings.Contains(err.Error(), "reparse") {
		t.Fatalf("expected reparse rejection, got %v", err)
	}
	if mkdirCalled {
		t.Fatal("MkdirAll was called after pre-existing config directory reparse point")
	}
}

func TestWriteWindowsJSONConfigIfMissingRejectsReparseAfterMkdir(t *testing.T) {
	existing := map[string]bool{
		filepath.Clean(`C:\`):            true,
		filepath.Clean(`C:\ProgramData`): true,
	}
	reparse := map[string]bool{}
	installWindowsPathTestStubs(t, existing, reparse)

	origMkdirAll := windowsMkdirAll
	mkdirCalled := false
	windowsMkdirAll = func(path string, _ os.FileMode) error {
		mkdirCalled = true
		clean := filepath.Clean(path)
		existing[clean] = true
		reparse[clean] = true
		return nil
	}
	t.Cleanup(func() { windowsMkdirAll = origMkdirAll })

	err := writeWindowsJSONConfigIfMissing(`C:\ProgramData\gov-pass\config.json`, windowsJSONConfig{})
	if err == nil || !strings.Contains(err.Error(), "reparse") {
		t.Fatalf("expected post-create reparse rejection, got %v", err)
	}
	if !mkdirCalled {
		t.Fatal("MkdirAll was not called")
	}
}

func TestEffectiveWindowsConfig_StatsIntervalFlagOverridesConfig(t *testing.T) {
	path := writeWindowsTempJSON(t, `{
  "engine": {
    "stats_interval": "2m"
  }
}`)
	args := windowsCLIArgs{
		ConfigPath:    path,
		StatsInterval: 5 * time.Second,
	}

	_, wc, err := effectiveWindowsConfig(args, map[string]bool{"stats-interval": true}, false)
	if err != nil {
		t.Fatalf("effective config: %v", err)
	}
	if wc.StatsInterval != 5*time.Second {
		t.Fatalf("StatsInterval = %s, want CLI override 5s", wc.StatsInterval)
	}
}

func TestWindowsExampleConfigApplies(t *testing.T) {
	path := filepath.Join("..", "..", "docs", "examples", "splitter.windows.json")
	fileCfg, err := readWindowsJSONConfig(path)
	if err != nil {
		t.Fatalf("read example config: %v", err)
	}

	cfg, wc := windowsDefaults()
	if err := applyWindowsJSONConfig(&cfg, &wc, fileCfg); err != nil {
		t.Fatalf("apply example config: %v", err)
	}
	if err := validateWindowsRunConfig(wc); err != nil {
		t.Fatalf("validate example config: %v", err)
	}
}

func TestValidateWindowsRunConfigAllowsDriverDefaultQueueOptions(t *testing.T) {
	_, wc := windowsDefaults()
	wc.AdapterOpts.QueueLen = 0
	wc.AdapterOpts.QueueTime = 0
	wc.AdapterOpts.QueueSize = 0

	if err := validateWindowsRunConfig(wc); err != nil {
		t.Fatalf("validate config with driver default queue options: %v", err)
	}
}

func TestValidateWindowsRunConfigRejectsInvalidWinDivertQueueOptions(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*windowsRunConfig)
		want   string
	}{
		{
			name: "queue_len_too_small",
			mutate: func(wc *windowsRunConfig) {
				wc.AdapterOpts.QueueLen = winDivertQueueLenMin - 1
			},
			want: "queue_len",
		},
		{
			name: "queue_len_too_large",
			mutate: func(wc *windowsRunConfig) {
				wc.AdapterOpts.QueueLen = winDivertQueueLenMax + 1
			},
			want: "queue_len",
		},
		{
			name: "queue_time_too_small",
			mutate: func(wc *windowsRunConfig) {
				wc.AdapterOpts.QueueTime = winDivertQueueTimeMin - 1
			},
			want: "queue_time_ms",
		},
		{
			name: "queue_time_too_large",
			mutate: func(wc *windowsRunConfig) {
				wc.AdapterOpts.QueueTime = winDivertQueueTimeMax + 1
			},
			want: "queue_time_ms",
		},
		{
			name: "queue_size_too_small",
			mutate: func(wc *windowsRunConfig) {
				wc.AdapterOpts.QueueSize = winDivertQueueSizeMin - 1
			},
			want: "queue_size_bytes",
		},
		{
			name: "queue_size_too_large",
			mutate: func(wc *windowsRunConfig) {
				wc.AdapterOpts.QueueSize = winDivertQueueSizeMax + 1
			},
			want: "queue_size_bytes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, wc := windowsDefaults()
			tt.mutate(&wc)

			err := validateWindowsRunConfig(wc)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q validation error, got %v", tt.want, err)
			}
		})
	}
}

func TestReadWindowsJSONConfigRejectsUnknownFields(t *testing.T) {
	path := writeWindowsTempJSON(t, `{
  "windivert": {
    "filter": "outbound and tcp.DstPort == 443",
    "queue_length": 4096
  }
}`)

	_, err := readWindowsJSONConfig(path)
	if err == nil || !strings.Contains(err.Error(), `unknown field "queue_length"`) {
		t.Fatalf("expected unknown field error, got %v", err)
	}
}

func TestDefaultWindowsDirsPreferKnownFolders(t *testing.T) {
	orig := windowsKnownFolderPath
	t.Cleanup(func() { windowsKnownFolderPath = orig })

	windowsKnownFolderPath = func(folderID *windows.KNOWNFOLDERID, flags uint32) (string, error) {
		switch folderID {
		case windows.FOLDERID_ProgramData:
			return `D:\KnownProgramData`, nil
		case windows.FOLDERID_ProgramFiles:
			return `D:\KnownProgramFiles`, nil
		default:
			return "", errors.New("unexpected folder")
		}
	}
	t.Setenv("ProgramData", `C:\PoisonedProgramData`)
	t.Setenv("ProgramFiles", `C:\PoisonedProgramFiles`)

	if got := defaultProgramDataDir(); got != filepath.Clean(`D:\KnownProgramData`) {
		t.Fatalf("defaultProgramDataDir = %q", got)
	}
	if got := defaultProgramFilesDir(); got != filepath.Clean(`D:\KnownProgramFiles`) {
		t.Fatalf("defaultProgramFilesDir = %q", got)
	}
	wantLog := filepath.Join(`D:\KnownProgramData`, "gov-pass", "splitter.log")
	if got := defaultServiceLogPath(); got != wantLog {
		t.Fatalf("defaultServiceLogPath = %q, want %q", got, wantLog)
	}
}

func TestDefaultWindowsDirsFallBackToSystemDefaults(t *testing.T) {
	orig := windowsKnownFolderPath
	t.Cleanup(func() { windowsKnownFolderPath = orig })

	windowsKnownFolderPath = func(folderID *windows.KNOWNFOLDERID, flags uint32) (string, error) {
		return "", errors.New("known folder unavailable")
	}
	t.Setenv("ProgramData", `C:\PoisonedProgramData`)
	t.Setenv("ProgramFiles", `C:\PoisonedProgramFiles`)

	if got := defaultProgramDataDir(); got != `C:\ProgramData` {
		t.Fatalf("defaultProgramDataDir = %q", got)
	}
	if got := defaultProgramFilesDir(); got != `C:\Program Files` {
		t.Fatalf("defaultProgramFilesDir = %q", got)
	}
}
