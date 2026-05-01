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
	"fk-gov/internal/driver"
	"fk-gov/internal/engine"
	"golang.org/x/sys/windows"
)

type windowsJSONConfig struct {
	Engine    *engineJSONConfig    `json:"engine,omitempty"`
	WinDivert *winDivertJSONConfig `json:"windivert,omitempty"`
}

type winDivertJSONConfig struct {
	Filter               *string `json:"filter,omitempty"`
	QueueLen             *uint64 `json:"queue_len,omitempty"`
	QueueTimeMs          *uint64 `json:"queue_time_ms,omitempty"`
	QueueSizeBytes       *uint64 `json:"queue_size_bytes,omitempty"`
	WinDivertDir         *string `json:"windivert_dir,omitempty"`
	WinDivertSys         *string `json:"windivert_sys,omitempty"`
	AllowServiceTakeover *bool   `json:"allow_service_takeover,omitempty"`
	AutoInstallDriver    *bool   `json:"auto_install_driver,omitempty"`
	AutoDownloadFiles    *bool   `json:"auto_download_files,omitempty"`
}

type windowsCLIArgs struct {
	SplitMode                  string
	SplitChunk                 int
	CollectTimeout             time.Duration
	MaxBufferBytes             int
	MaxHeldPackets             int
	MaxSegPayload              int
	Workers                    int
	FlowTimeout                time.Duration
	GCInterval                 time.Duration
	MaxFlows                   int
	MaxReassembly              int
	MaxHeldBytes               int
	ShutdownFailOpenTimeout    time.Duration
	ShutdownFailOpenMaxPackets int
	AdapterFlushTimeout        time.Duration
	StatsInterval              time.Duration

	Filter    string
	QueueLen  uint64
	QueueTime uint64
	QueueSize uint64

	WinDivertDir         string
	WinDivertSys         string
	AllowServiceTakeover bool

	AutoInstall   bool
	AutoUninstall bool
	AutoDownload  bool

	ConfigPath string
}

var windowsKnownFolderPath = windows.KnownFolderPath

var (
	ensureSecureWindowsConfigDir        = ensureSecureWindowsDir
	hardenWindowsConfigFileACL          = hardenWindowsFileACL
	writeWindowsJSONConfigIfMissingFile = writeWindowsJSONConfigIfMissing
)

func defaultProgramDataDir() string {
	return defaultWindowsKnownFolderDir(windows.FOLDERID_ProgramData, `C:\ProgramData`)
}

func defaultProgramFilesDir() string {
	return defaultWindowsKnownFolderDir(windows.FOLDERID_ProgramFiles, `C:\Program Files`)
}

func defaultWindowsKnownFolderDir(folderID *windows.KNOWNFOLDERID, fallback string) string {
	base, err := windowsKnownFolderPath(folderID, windows.KF_FLAG_DEFAULT)
	if err == nil && strings.TrimSpace(base) != "" {
		return filepath.Clean(base)
	}
	return fallback
}

func defaultServiceConfigPath() string {
	return filepath.Join(defaultProgramDataDir(), "gov-pass", "config.json")
}

func defaultInstallRootDir() string {
	return filepath.Join(defaultProgramFilesDir(), "gov-pass")
}

func validateWindowsServiceManagedPath(label string, path string, allowedRoots ...string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("%s path is empty", label)
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("%s path must be absolute in service mode", label)
	}
	if err := validateManagedWindowsPath(path, allowedRoots...); err != nil {
		return fmt.Errorf("%s %w", label, err)
	}
	return nil
}

func validateWindowsServiceConfigPath(path string) error {
	return validateWindowsServiceManagedPath("config", path, filepath.Join(defaultProgramDataDir(), "gov-pass"))
}

func validateWindowsServiceLogPath(path string) error {
	return validateWindowsServiceManagedPath("service log", path, filepath.Join(defaultProgramDataDir(), "gov-pass"))
}

func validateWindowsServiceDriverDir(dir string) error {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil
	}
	return validateWindowsServiceManagedPath("WinDivert directory", dir, filepath.Join(defaultProgramDataDir(), "gov-pass"), defaultInstallRootDir())
}

func readWindowsJSONConfig(path string) (windowsJSONConfig, error) {
	b, err := readJSONConfigFile(path)
	if err != nil {
		return windowsJSONConfig{}, err
	}
	var cfg windowsJSONConfig
	if err := decodeStrictJSONConfig(b, &cfg); err != nil {
		return windowsJSONConfig{}, err
	}
	return cfg, nil
}

func writeWindowsJSONConfigIfMissing(path string, cfg windowsJSONConfig) error {
	dir := filepath.Dir(path)
	if err := rejectWindowsReparsePath(dir); err != nil {
		return err
	}
	if err := windowsMkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := rejectWindowsReparsePath(dir); err != nil {
		return err
	}

	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')

	if err := rejectWindowsReparsePath(path); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			existing, _, openErr := openRegularConfigFile(path)
			if openErr != nil {
				return openErr
			}
			_ = existing.Close()
			return nil
		}
		return err
	}
	defer func() {
		_ = f.Close()
	}()

	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("config path must be a regular file: %s", path)
	}
	if _, err := f.Write(b); err != nil {
		return err
	}
	return f.Close()
}

func applyWindowsJSONConfig(dstEngine *engine.Config, dstWin *windowsRunConfig, cfg windowsJSONConfig) error {
	if dstEngine == nil || dstWin == nil {
		return errors.New("nil config destination")
	}

	if err := applyEngineJSONConfig(dstEngine, cfg.Engine); err != nil {
		return err
	}
	if statsInterval, err := parseEngineStatsIntervalConfig(cfg.Engine); err != nil {
		return err
	} else if statsInterval != nil {
		dstWin.StatsInterval = *statsInterval
	}

	if cfg.WinDivert != nil {
		if cfg.WinDivert.Filter != nil && strings.TrimSpace(*cfg.WinDivert.Filter) != "" {
			dstWin.Filter = upgradeLegacyWinDivertFilter(*cfg.WinDivert.Filter)
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
		if cfg.WinDivert.AllowServiceTakeover != nil {
			dstWin.AllowServiceTakeover = *cfg.WinDivert.AllowServiceTakeover
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
	shutdownFailOpenTimeout := cfg.ShutdownFailOpenTimeout.String()
	adapterFlushTimeout := cfg.AdapterFlushTimeout.String()
	statsInterval := wc.StatsInterval.String()

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
		ShutdownFailOpenTimeout:     &shutdownFailOpenTimeout,
		ShutdownFailOpenMaxPackets:  &cfg.ShutdownFailOpenMaxPackets,
		AdapterFlushTimeout:         &adapterFlushTimeout,
		StatsInterval:               &statsInterval,
	}

	filter := wc.Filter
	queueLen := wc.AdapterOpts.QueueLen
	queueTime := wc.AdapterOpts.QueueTime
	queueSize := wc.AdapterOpts.QueueSize
	allowTakeover := wc.AllowServiceTakeover
	autoInstall := wc.AutoInstallDriver
	autoDownload := wc.AutoDownloadFiles

	winCfg := &winDivertJSONConfig{
		Filter:               &filter,
		QueueLen:             &queueLen,
		QueueTimeMs:          &queueTime,
		QueueSizeBytes:       &queueSize,
		AllowServiceTakeover: &allowTakeover,
		AutoInstallDriver:    &autoInstall,
		AutoDownloadFiles:    &autoDownload,
	}

	// Keep explicit windivert_dir/sys out of the default template; the MSI layout
	// places the driver files next to the exe, and the runtime resolves exeDir.
	return windowsJSONConfig{
		Engine:    engineCfg,
		WinDivert: winCfg,
	}
}

func validateWindowsRunConfig(wc windowsRunConfig) error {
	if strings.TrimSpace(wc.Filter) == "" {
		return errors.New("filter is empty")
	}
	if err := validateWinDivertOptions(wc.AdapterOpts); err != nil {
		return err
	}
	if err := driver.ValidateServiceName(wc.WinDivertSvcName); err != nil {
		return fmt.Errorf("windivert service name invalid: %w", err)
	}
	if err := driver.ValidateDriverFileName(wc.WinDivertSys); err != nil {
		return fmt.Errorf("windivert sys filename invalid: %w", err)
	}
	return nil
}

func validateWinDivertOptions(opts adapter.WinDivertOptions) error {
	if err := validateWinDivertQueueParam("queue_len", opts.QueueLen, winDivertQueueLenMin, winDivertQueueLenMax); err != nil {
		return err
	}
	if err := validateWinDivertQueueParam("queue_time_ms", opts.QueueTime, winDivertQueueTimeMin, winDivertQueueTimeMax); err != nil {
		return err
	}
	if err := validateWinDivertQueueParam("queue_size_bytes", opts.QueueSize, winDivertQueueSizeMin, winDivertQueueSizeMax); err != nil {
		return err
	}
	return nil
}

func validateWinDivertQueueParam(name string, value uint64, min uint64, max uint64) error {
	if value == 0 {
		return nil
	}
	if value < min || value > max {
		return fmt.Errorf("%s must be 0 or between %d and %d", name, min, max)
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
		WinDivertDir:         "",
		WinDivertSys:         "",
		WinDivertSvcName:     defaultWinDivertServiceName,
		AllowServiceTakeover: false,
		AutoInstallDriver:    true,
		AutoUninstallDriver:  true,
		AutoDownloadFiles:    true,
		StatsInterval:        defaultStatsInterval,
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
	managedProgramDataPath := false
	if asService && configPath != "" {
		if err := validateWindowsServiceConfigPath(configPath); err != nil {
			return engine.Config{}, windowsRunConfig{}, err
		}
		managedProgramDataPath = validateManagedWindowsPath(configPath, programDataRoot) == nil
	}
	if asService && configPath != "" && managedProgramDataPath {
		// Ensure ProgramData state is not user-writable. This prevents config
		// tampering and DLL hijacking via windivert_dir in service mode.
		if err := ensureSecureWindowsConfigDir(programDataRoot); err != nil {
			return engine.Config{}, windowsRunConfig{}, fmt.Errorf("secure ProgramData dir failed: %w", err)
		}
		if existing, _, err := openRegularConfigFile(configPath); err == nil {
			_ = existing.Close()
			if err := hardenWindowsConfigFileACL(configPath); err != nil {
				return engine.Config{}, windowsRunConfig{}, fmt.Errorf("secure config file failed: %w", err)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return engine.Config{}, windowsRunConfig{}, fmt.Errorf("secure config file failed: %w", err)
		}
	}

	if configPath != "" {
		fileCfg, err := readWindowsJSONConfig(configPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) && asService && usingDefaultPath {
				// First service run: create a default config template, then
				// read it back so create races cannot silently ignore config.
				tpl := windowsJSONConfigFromDefaults(cfg, wc)
				if err := writeWindowsJSONConfigIfMissingFile(configPath, tpl); err != nil {
					return engine.Config{}, windowsRunConfig{}, fmt.Errorf("create default config failed: %w", err)
				}
				if asService && managedProgramDataPath {
					if err := hardenWindowsConfigFileACL(configPath); err != nil {
						return engine.Config{}, windowsRunConfig{}, fmt.Errorf("secure config file failed: %w", err)
					}
				}
				fileCfg, err = readWindowsJSONConfig(configPath)
				if err != nil {
					return engine.Config{}, windowsRunConfig{}, fmt.Errorf("read config failed (%s): %w", configPath, err)
				}
			} else {
				return engine.Config{}, windowsRunConfig{}, fmt.Errorf("read config failed (%s): %w", configPath, err)
			}
		}
		if err := applyWindowsJSONConfig(&cfg, &wc, fileCfg); err != nil {
			return engine.Config{}, windowsRunConfig{}, fmt.Errorf("apply config failed: %w", err)
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
	if setFlags["shutdown-fail-open-timeout"] {
		cfg.ShutdownFailOpenTimeout = args.ShutdownFailOpenTimeout
	}
	if setFlags["shutdown-fail-open-max-pkts"] {
		cfg.ShutdownFailOpenMaxPackets = args.ShutdownFailOpenMaxPackets
	}
	if setFlags["adapter-flush-timeout"] {
		cfg.AdapterFlushTimeout = args.AdapterFlushTimeout
	}
	if setFlags["stats-interval"] {
		wc.StatsInterval = args.StatsInterval
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
	if setFlags["allow-service-takeover"] {
		wc.AllowServiceTakeover = args.AllowServiceTakeover
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
		if err := validateWindowsServiceDriverDir(wc.WinDivertDir); err != nil {
			return engine.Config{}, windowsRunConfig{}, err
		}
	}

	if err := validateEngineConfig(cfg); err != nil {
		return engine.Config{}, windowsRunConfig{}, err
	}
	if wc.StatsInterval < 0 {
		return engine.Config{}, windowsRunConfig{}, errors.New("stats-interval must be >= 0")
	}
	if err := validateWindowsRunConfig(wc); err != nil {
		return engine.Config{}, windowsRunConfig{}, err
	}
	return cfg, wc, nil
}
