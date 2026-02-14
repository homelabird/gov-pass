//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fk-gov/internal/adapter"
	"fk-gov/internal/engine"
)

type windowsJSONConfig struct {
	Engine    *engineJSONConfig    `json:"engine,omitempty"`
	WinDivert *winDivertJSONConfig `json:"windivert,omitempty"`
}

type engineJSONConfig struct {
	SplitMode                   *string `json:"split_mode,omitempty"`
	SplitChunk                  *int    `json:"split_chunk,omitempty"`
	CollectTimeout              *string `json:"collect_timeout,omitempty"`
	MaxBufferBytes              *int    `json:"max_buffer_bytes,omitempty"`
	MaxHeldPackets              *int    `json:"max_held_packets,omitempty"`
	MaxSegmentPayload           *int    `json:"max_segment_payload,omitempty"`
	Workers                     *int    `json:"workers,omitempty"`
	FlowIdleTimeout             *string `json:"flow_idle_timeout,omitempty"`
	GCInterval                  *string `json:"gc_interval,omitempty"`
	MaxFlowsPerWorker           *int    `json:"max_flows_per_worker,omitempty"`
	MaxReassemblyBytesPerWorker *int    `json:"max_reassembly_bytes_per_worker,omitempty"`
	MaxHeldBytesPerWorker       *int    `json:"max_held_bytes_per_worker,omitempty"`
}

type winDivertJSONConfig struct {
	Filter            *string `json:"filter,omitempty"`
	QueueLen          *uint64 `json:"queue_len,omitempty"`
	QueueTimeMs       *uint64 `json:"queue_time_ms,omitempty"`
	QueueSizeBytes    *uint64 `json:"queue_size_bytes,omitempty"`
	WinDivertDir      *string `json:"windivert_dir,omitempty"`
	WinDivertSys      *string `json:"windivert_sys,omitempty"`
	AutoInstallDriver *bool   `json:"auto_install_driver,omitempty"`
	AutoDownloadFiles *bool   `json:"auto_download_files,omitempty"`
}

type windowsCLIArgs struct {
	SplitMode      string
	SplitChunk     int
	CollectTimeout time.Duration
	MaxBufferBytes int
	MaxHeldPackets int
	MaxSegPayload  int
	Workers        int
	FlowTimeout    time.Duration
	GCInterval     time.Duration
	MaxFlows       int
	MaxReassembly  int
	MaxHeldBytes   int

	Filter    string
	QueueLen  uint64
	QueueTime uint64
	QueueSize uint64

	WinDivertDir string
	WinDivertSys string

	AutoInstall   bool
	AutoUninstall bool
	AutoDownload  bool

	ConfigPath string
}

func defaultProgramDataDir() string {
	base := os.Getenv("ProgramData")
	if base == "" {
		base = `C:\ProgramData`
	}
	return base
}

func defaultServiceConfigPath() string {
	return filepath.Join(defaultProgramDataDir(), "gov-pass", "config.json")
}

func readWindowsJSONConfig(path string) (windowsJSONConfig, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return windowsJSONConfig{}, err
	}
	var cfg windowsJSONConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		return windowsJSONConfig{}, err
	}
	return cfg, nil
}

func writeWindowsJSONConfigIfMissing(path string, cfg windowsJSONConfig) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')

	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil
		}
		return err
	}
	defer func() {
		_ = f.Close()
	}()

	if _, err := f.Write(b); err != nil {
		return err
	}
	return f.Close()
}

func applyWindowsJSONConfig(dstEngine *engine.Config, dstWin *windowsRunConfig, cfg windowsJSONConfig) error {
	if dstEngine == nil || dstWin == nil {
		return errors.New("nil config destination")
	}

	if cfg.Engine != nil {
		if cfg.Engine.SplitMode != nil && strings.TrimSpace(*cfg.Engine.SplitMode) != "" {
			mode, err := parseSplitMode(*cfg.Engine.SplitMode)
			if err != nil {
				return fmt.Errorf("engine.split_mode: %w", err)
			}
			dstEngine.SplitMode = mode
		}
		if cfg.Engine.SplitChunk != nil {
			dstEngine.SplitChunk = *cfg.Engine.SplitChunk
		}
		if cfg.Engine.CollectTimeout != nil && strings.TrimSpace(*cfg.Engine.CollectTimeout) != "" {
			d, err := time.ParseDuration(*cfg.Engine.CollectTimeout)
			if err != nil {
				return fmt.Errorf("engine.collect_timeout: %w", err)
			}
			dstEngine.CollectTimeout = d
		}
		if cfg.Engine.MaxBufferBytes != nil {
			dstEngine.MaxBufferBytes = *cfg.Engine.MaxBufferBytes
		}
		if cfg.Engine.MaxHeldPackets != nil {
			dstEngine.MaxHeldPackets = *cfg.Engine.MaxHeldPackets
		}
		if cfg.Engine.MaxSegmentPayload != nil {
			dstEngine.MaxSegmentPayload = *cfg.Engine.MaxSegmentPayload
		}
		if cfg.Engine.Workers != nil {
			dstEngine.WorkerCount = *cfg.Engine.Workers
		}
		if cfg.Engine.FlowIdleTimeout != nil && strings.TrimSpace(*cfg.Engine.FlowIdleTimeout) != "" {
			d, err := time.ParseDuration(*cfg.Engine.FlowIdleTimeout)
			if err != nil {
				return fmt.Errorf("engine.flow_idle_timeout: %w", err)
			}
			dstEngine.FlowIdleTimeout = d
		}
		if cfg.Engine.GCInterval != nil && strings.TrimSpace(*cfg.Engine.GCInterval) != "" {
			d, err := time.ParseDuration(*cfg.Engine.GCInterval)
			if err != nil {
				return fmt.Errorf("engine.gc_interval: %w", err)
			}
			dstEngine.GCInterval = d
		}
		if cfg.Engine.MaxFlowsPerWorker != nil {
			dstEngine.MaxFlowsPerWorker = *cfg.Engine.MaxFlowsPerWorker
		}
		if cfg.Engine.MaxReassemblyBytesPerWorker != nil {
			dstEngine.MaxReassemblyBytesPerWorker = *cfg.Engine.MaxReassemblyBytesPerWorker
		}
		if cfg.Engine.MaxHeldBytesPerWorker != nil {
			dstEngine.MaxHeldBytesPerWorker = *cfg.Engine.MaxHeldBytesPerWorker
		}
	}

	if cfg.WinDivert != nil {
		if cfg.WinDivert.Filter != nil && strings.TrimSpace(*cfg.WinDivert.Filter) != "" {
			dstWin.Filter = *cfg.WinDivert.Filter
		}
		if cfg.WinDivert.QueueLen != nil {
			dstWin.AdapterOpts.QueueLen = *cfg.WinDivert.QueueLen
		}
		if cfg.WinDivert.QueueTimeMs != nil {
			dstWin.AdapterOpts.QueueTime = *cfg.WinDivert.QueueTimeMs
		}
		if cfg.WinDivert.QueueSizeBytes != nil {
			dstWin.AdapterOpts.QueueSize = *cfg.WinDivert.QueueSizeBytes
		}
		if cfg.WinDivert.WinDivertDir != nil {
			dstWin.WinDivertDir = strings.TrimSpace(*cfg.WinDivert.WinDivertDir)
		}
		if cfg.WinDivert.WinDivertSys != nil {
			dstWin.WinDivertSys = strings.TrimSpace(*cfg.WinDivert.WinDivertSys)
		}
		if cfg.WinDivert.AutoInstallDriver != nil {
			dstWin.AutoInstallDriver = *cfg.WinDivert.AutoInstallDriver
		}
		if cfg.WinDivert.AutoDownloadFiles != nil {
			dstWin.AutoDownloadFiles = *cfg.WinDivert.AutoDownloadFiles
		}
	}

	return nil
}

func windowsJSONConfigFromDefaults(cfg engine.Config, wc windowsRunConfig) windowsJSONConfig {
	mode := "tls-hello"
	if cfg.SplitMode == engine.SplitModeImmediate {
		mode = "immediate"
	}
	collectTimeout := cfg.CollectTimeout.String()
	flowTimeout := cfg.FlowIdleTimeout.String()
	gcInterval := cfg.GCInterval.String()

	engineCfg := &engineJSONConfig{
		SplitMode:                   &mode,
		SplitChunk:                  &cfg.SplitChunk,
		CollectTimeout:              &collectTimeout,
		MaxBufferBytes:              &cfg.MaxBufferBytes,
		MaxHeldPackets:              &cfg.MaxHeldPackets,
		MaxSegmentPayload:           &cfg.MaxSegmentPayload,
		Workers:                     &cfg.WorkerCount,
		FlowIdleTimeout:             &flowTimeout,
		GCInterval:                  &gcInterval,
		MaxFlowsPerWorker:           &cfg.MaxFlowsPerWorker,
		MaxReassemblyBytesPerWorker: &cfg.MaxReassemblyBytesPerWorker,
		MaxHeldBytesPerWorker:       &cfg.MaxHeldBytesPerWorker,
	}

	filter := wc.Filter
	queueLen := wc.AdapterOpts.QueueLen
	queueTime := wc.AdapterOpts.QueueTime
	queueSize := wc.AdapterOpts.QueueSize
	autoInstall := wc.AutoInstallDriver
	autoDownload := wc.AutoDownloadFiles

	winCfg := &winDivertJSONConfig{
		Filter:            &filter,
		QueueLen:          &queueLen,
		QueueTimeMs:       &queueTime,
		QueueSizeBytes:    &queueSize,
		AutoInstallDriver: &autoInstall,
		AutoDownloadFiles: &autoDownload,
	}

	// Keep explicit windivert_dir/sys out of the default template; the MSI layout
	// places the driver files next to the exe, and the runtime resolves exeDir.
	return windowsJSONConfig{
		Engine:    engineCfg,
		WinDivert: winCfg,
	}
}

func validateEngineConfig(cfg engine.Config) error {
	if cfg.SplitChunk < 1 {
		return errors.New("split-chunk must be >= 1")
	}
	if cfg.MaxBufferBytes < 1 {
		return errors.New("max-buffer must be >= 1")
	}
	if cfg.MaxHeldPackets < 1 {
		return errors.New("max-held-pkts must be >= 1")
	}
	if cfg.MaxSegmentPayload < 0 {
		return errors.New("max-seg-payload must be >= 0")
	}
	if cfg.WorkerCount < 1 {
		return errors.New("workers must be >= 1")
	}
	if cfg.CollectTimeout < 1*time.Millisecond {
		return errors.New("collect-timeout must be >= 1ms")
	}
	if cfg.FlowIdleTimeout < 1*time.Millisecond {
		return errors.New("flow-timeout must be >= 1ms")
	}
	if cfg.GCInterval < 1*time.Millisecond {
		return errors.New("gc-interval must be >= 1ms")
	}
	if cfg.MaxFlowsPerWorker < 0 {
		return errors.New("max-flows-per-worker must be >= 0")
	}
	if cfg.MaxReassemblyBytesPerWorker < 0 {
		return errors.New("max-reassembly-bytes-per-worker must be >= 0")
	}
	if cfg.MaxHeldBytesPerWorker < 0 {
		return errors.New("max-held-bytes-per-worker must be >= 0")
	}
	return nil
}

func validateWindowsRunConfig(wc windowsRunConfig) error {
	if strings.TrimSpace(wc.Filter) == "" {
		return errors.New("filter is empty")
	}
	return nil
}

func windowsDefaults() (engine.Config, windowsRunConfig) {
	cfg := engine.DefaultConfig()
	wc := windowsRunConfig{
		Filter: defaultWinDivertFilter,
		AdapterOpts: adapter.WinDivertOptions{
			QueueLen:  defaultQueueLen,
			QueueTime: defaultQueueTimeMs,
			QueueSize: defaultQueueSize,
		},
		WinDivertDir:        "",
		WinDivertSys:        "",
		WinDivertSvcName:    defaultWinDivertServiceName,
		AutoInstallDriver:   true,
		AutoUninstallDriver: true,
		AutoDownloadFiles:   true,
	}
	return cfg, wc
}

func effectiveWindowsConfig(args windowsCLIArgs, setFlags map[string]bool, asService bool) (engine.Config, windowsRunConfig, error) {
	cfg, wc := windowsDefaults()

	configPath := strings.TrimSpace(args.ConfigPath)
	usingDefaultPath := false
	if configPath == "" && asService {
		configPath = defaultServiceConfigPath()
		usingDefaultPath = true
	}

	programDataRoot := filepath.Join(defaultProgramDataDir(), "gov-pass")
	if asService && configPath != "" && isUnderDir(configPath, programDataRoot) {
		// Ensure ProgramData state is not user-writable. This prevents config
		// tampering and DLL hijacking via windivert_dir in service mode.
		if err := ensureSecureWindowsDir(programDataRoot); err != nil {
			return engine.Config{}, windowsRunConfig{}, fmt.Errorf("secure ProgramData dir failed: %w", err)
		}
		if _, err := os.Stat(configPath); err == nil {
			if err := hardenWindowsFileACL(configPath); err != nil {
				return engine.Config{}, windowsRunConfig{}, fmt.Errorf("secure config file failed: %w", err)
			}
		}
	}

	if configPath != "" {
		fileCfg, err := readWindowsJSONConfig(configPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) && asService && usingDefaultPath {
				// First service run: create a default config template and continue with defaults.
				tpl := windowsJSONConfigFromDefaults(cfg, wc)
				if err := writeWindowsJSONConfigIfMissing(configPath, tpl); err != nil {
					return engine.Config{}, windowsRunConfig{}, fmt.Errorf("create default config failed: %w", err)
				}
				if asService && isUnderDir(configPath, programDataRoot) {
					if err := hardenWindowsFileACL(configPath); err != nil {
						return engine.Config{}, windowsRunConfig{}, fmt.Errorf("secure config file failed: %w", err)
					}
				}
			} else {
				return engine.Config{}, windowsRunConfig{}, fmt.Errorf("read config failed (%s): %w", configPath, err)
			}
		} else {
			if err := applyWindowsJSONConfig(&cfg, &wc, fileCfg); err != nil {
				return engine.Config{}, windowsRunConfig{}, fmt.Errorf("apply config failed: %w", err)
			}
		}
	}

	// Explicit CLI flags override config file.
	if setFlags["split-mode"] {
		mode, err := parseSplitMode(args.SplitMode)
		if err != nil {
			return engine.Config{}, windowsRunConfig{}, fmt.Errorf("invalid split-mode: %w", err)
		}
		cfg.SplitMode = mode
	}
	if setFlags["split-chunk"] {
		cfg.SplitChunk = args.SplitChunk
	}
	if setFlags["collect-timeout"] {
		cfg.CollectTimeout = args.CollectTimeout
	}
	if setFlags["max-buffer"] {
		cfg.MaxBufferBytes = args.MaxBufferBytes
	}
	if setFlags["max-held-pkts"] {
		cfg.MaxHeldPackets = args.MaxHeldPackets
	}
	if setFlags["max-seg-payload"] {
		cfg.MaxSegmentPayload = args.MaxSegPayload
	}
	if setFlags["workers"] {
		cfg.WorkerCount = args.Workers
	}
	if setFlags["flow-timeout"] {
		cfg.FlowIdleTimeout = args.FlowTimeout
	}
	if setFlags["gc-interval"] {
		cfg.GCInterval = args.GCInterval
	}
	if setFlags["max-flows-per-worker"] {
		cfg.MaxFlowsPerWorker = args.MaxFlows
	}
	if setFlags["max-reassembly-bytes-per-worker"] {
		cfg.MaxReassemblyBytesPerWorker = args.MaxReassembly
	}
	if setFlags["max-held-bytes-per-worker"] {
		cfg.MaxHeldBytesPerWorker = args.MaxHeldBytes
	}
	if setFlags["filter"] {
		wc.Filter = args.Filter
	}
	if setFlags["queue-len"] {
		wc.AdapterOpts.QueueLen = args.QueueLen
	}
	if setFlags["queue-time"] {
		wc.AdapterOpts.QueueTime = args.QueueTime
	}
	if setFlags["queue-size"] {
		wc.AdapterOpts.QueueSize = args.QueueSize
	}
	if setFlags["windivert-dir"] {
		wc.WinDivertDir = args.WinDivertDir
	}
	if setFlags["windivert-sys"] {
		wc.WinDivertSys = args.WinDivertSys
	}
	if setFlags["auto-install"] {
		wc.AutoInstallDriver = args.AutoInstall
	}
	if setFlags["auto-uninstall"] {
		wc.AutoUninstallDriver = args.AutoUninstall
	}
	if setFlags["auto-download-windivert"] {
		wc.AutoDownloadFiles = args.AutoDownload
	}

	// In service mode, never uninstall the driver on stop/uninstall.
	if asService {
		wc.AutoUninstallDriver = false
	}

	if err := validateEngineConfig(cfg); err != nil {
		return engine.Config{}, windowsRunConfig{}, err
	}
	if err := validateWindowsRunConfig(wc); err != nil {
		return engine.Config{}, windowsRunConfig{}, err
	}
	return cfg, wc, nil
}
