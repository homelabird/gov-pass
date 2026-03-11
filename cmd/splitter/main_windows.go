//go:build windows

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"fk-gov/internal/adapter"
	"fk-gov/internal/driver"
	"fk-gov/internal/engine"
)

const (
	defaultQueueLen             uint64 = 4096
	defaultQueueTimeMs          uint64 = 2000
	defaultQueueSize            uint64 = 32 * 1024 * 1024
	defaultWinDivertServiceName        = "WinDivert"
	defaultAppServiceName              = "gov-pass"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	defaultCfg, _ := windowsDefaults()

	splitMode := flag.String("split-mode", "tls-hello", "split trigger: tls-hello or immediate")
	splitChunk := flag.Int("split-chunk", defaultCfg.SplitChunk, "first split size in bytes")
	collectTimeout := flag.Duration("collect-timeout", defaultCfg.CollectTimeout, "reassembly collect timeout")
	maxBuffer := flag.Int("max-buffer", defaultCfg.MaxBufferBytes, "max reassembly buffer size in bytes")
	maxHeld := flag.Int("max-held-pkts", defaultCfg.MaxHeldPackets, "max held packets per flow")
	maxSegPayload := flag.Int("max-seg-payload", defaultCfg.MaxSegmentPayload, "max segment payload size (0=unlimited)")
	workers := flag.Int("workers", defaultCfg.WorkerCount, "worker count for sharded processing")
	flowTimeout := flag.Duration("flow-timeout", defaultCfg.FlowIdleTimeout, "idle timeout for flow cleanup")
	gcInterval := flag.Duration("gc-interval", defaultCfg.GCInterval, "flow GC interval")
	maxFlows := flag.Int("max-flows-per-worker", defaultCfg.MaxFlowsPerWorker, "max tracked flows per worker (0=unlimited)")
	maxReassembly := flag.Int("max-reassembly-bytes-per-worker", defaultCfg.MaxReassemblyBytesPerWorker, "max total reassembly bytes per worker (0=unlimited)")
	maxHeldBytes := flag.Int("max-held-bytes-per-worker", defaultCfg.MaxHeldBytesPerWorker, "max total held packet bytes per worker (0=unlimited)")
	shutdownFailOpenTimeout := flag.Duration("shutdown-fail-open-timeout", defaultCfg.ShutdownFailOpenTimeout, "shutdown fail-open drain timeout per worker (0=use default)")
	shutdownFailOpenMaxPkts := flag.Int("shutdown-fail-open-max-pkts", defaultCfg.ShutdownFailOpenMaxPackets, "shutdown fail-open max packets per worker (0=use default)")
	adapterFlushTimeout := flag.Duration("adapter-flush-timeout", defaultCfg.AdapterFlushTimeout, "adapter flush timeout on shutdown (0=use default)")
	filter := flag.String("filter", defaultWinDivertFilter, "WinDivert filter")
	queueLen := flag.Uint("queue-len", uint(defaultQueueLen), "WinDivert queue length (0=driver default)")
	queueTime := flag.Uint("queue-time", uint(defaultQueueTimeMs), "WinDivert queue time in ms (0=driver default)")
	queueSize := flag.Uint("queue-size", uint(defaultQueueSize), "WinDivert queue size in bytes (0=driver default)")
	driverDir := flag.String("windivert-dir", "", "directory containing WinDivert.dll/.sys/.cat (default: exe dir)")
	driverSys := flag.String("windivert-sys", "", "driver sys filename (default: WinDivert64.sys or WinDivert.sys)")
	autoInstall := flag.Bool("auto-install", true, "auto install/start WinDivert driver")
	autoUninstall := flag.Bool("auto-uninstall", true, "auto uninstall if installed by this run")
	autoDownload := flag.Bool("auto-download-windivert", true, "auto download pinned WinDivert zip if required files are missing")
	configPath := flag.String("config", "", "path to config json (default in service: %ProgramData%\\gov-pass\\config.json)")
	asService := flag.Bool("service", false, "run as Windows service (SCM)")
	serviceName := flag.String("service-name", defaultAppServiceName, "Windows service name (used with --service)")
	serviceLog := flag.String("service-log", "", "log file path for --service (default: %ProgramData%\\gov-pass\\splitter.log)")
	serviceLogMaxBytes := flag.Int64("service-log-max-bytes", defaultServiceLogMaxBytes, "max service log file size in bytes before rotation")
	serviceLogMaxFiles := flag.Int("service-log-max-files", defaultServiceLogMaxFiles, "max number of rotated service log files to keep")
	printReloadability := flag.Bool("print-reloadability", false, "print reloadable vs restart-required settings and exit")
	flag.Parse()

	setFlags := make(map[string]bool)
	flag.CommandLine.Visit(func(f *flag.Flag) {
		setFlags[f.Name] = true
	})

	if *printReloadability {
		printWindowsReloadability(os.Stdout)
		return nil
	}

	if err := driver.ValidateServiceName(*serviceName); err != nil {
		return fmt.Errorf("--service-name is invalid: %w", err)
	}

	if !*asService && isWindowsServiceProcess() {
		*asService = true
	}

	args := windowsCLIArgs{
		SplitMode:                  *splitMode,
		SplitChunk:                 *splitChunk,
		CollectTimeout:             *collectTimeout,
		MaxBufferBytes:             *maxBuffer,
		MaxHeldPackets:             *maxHeld,
		MaxSegPayload:              *maxSegPayload,
		Workers:                    *workers,
		FlowTimeout:                *flowTimeout,
		GCInterval:                 *gcInterval,
		MaxFlows:                   *maxFlows,
		MaxReassembly:              *maxReassembly,
		MaxHeldBytes:               *maxHeldBytes,
		ShutdownFailOpenTimeout:    *shutdownFailOpenTimeout,
		ShutdownFailOpenMaxPackets: *shutdownFailOpenMaxPkts,
		AdapterFlushTimeout:        *adapterFlushTimeout,

		Filter:    *filter,
		QueueLen:  uint64(*queueLen),
		QueueTime: uint64(*queueTime),
		QueueSize: uint64(*queueSize),

		WinDivertDir: strings.TrimSpace(*driverDir),
		WinDivertSys: strings.TrimSpace(*driverSys),

		AutoInstall:   *autoInstall,
		AutoUninstall: *autoUninstall,
		AutoDownload:  *autoDownload,
		ConfigPath:    strings.TrimSpace(*configPath),
	}

	if *asService {
		return runService(*serviceName, serviceLogConfig{
			Path:     *serviceLog,
			MaxBytes: *serviceLogMaxBytes,
			MaxFiles: *serviceLogMaxFiles,
		}, func(ctx context.Context, reload <-chan struct{}) error {
			return runWindowsService(ctx, args, setFlags, reload)
		})
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, wc, err := effectiveWindowsConfig(args, setFlags, false)
	if err != nil {
		return err
	}
	return runWindows(ctx, cfg, wc)
}

type windowsRunConfig struct {
	Filter      string
	AdapterOpts adapter.WinDivertOptions

	WinDivertDir     string
	WinDivertSys     string
	WinDivertSvcName string

	AllowServiceTakeover bool
	AutoInstallDriver    bool
	AutoUninstallDriver  bool
	AutoDownloadFiles    bool
}

func runWindows(ctx context.Context, cfg engine.Config, wc windowsRunConfig) error {
	exeDir := ""
	if exe, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exe)
	}

	var err error
	var driverDir string
	wc, driverDir, err = ensureWinDivertFiles(ctx, wc, exeDir)
	if err != nil {
		return err
	}

	if err := adapter.ConfigureWinDivertDLL(filepath.Join(driverDir, "WinDivert.dll")); err != nil {
		return fmt.Errorf("configure WinDivert.dll failed: %w", err)
	}

	report, cleanup, err := driver.EnsureWithReport(ctx, driver.Config{
		Dir:           driverDir,
		SysName:       wc.WinDivertSys,
		ServiceName:   wc.WinDivertSvcName,
		AutoInstall:   wc.AutoInstallDriver,
		AutoUninstall: wc.AutoUninstallDriver,
		AutoStop:      true,
	})
	if err != nil {
		logWinDivertReport(report)
		return fmt.Errorf("driver ensure failed: %w", err)
	}
	logWinDivertReport(report)
	if cleanup != nil {
		defer func() {
			if err := cleanup(); err != nil {
				logError("windivert_cleanup_failed", "driver cleanup failed", err)
			}
		}()
	}

	ad, err := adapter.NewWinDivert(wc.Filter, wc.AdapterOpts)
	if err != nil {
		return fmt.Errorf("WinDivert open failed: %w", err)
	}
	eng := engine.New(cfg, ad)

	logInfo("engine_started", "engine started", "workers", cfg.WorkerCount, "split_mode", cfg.SplitMode, "split_chunk", cfg.SplitChunk)
	err = eng.Run(ctx)
	logInfo("engine_stats", "engine stats", "stats", eng.Stats())
	if err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("engine stopped: %w", err)
	}
	return nil
}

func runWindowsService(ctx context.Context, args windowsCLIArgs, setFlags map[string]bool, reload <-chan struct{}) error {
	cfg, wc, err := effectiveWindowsConfig(args, setFlags, true)
	if err != nil {
		return err
	}

	exeDir := ""
	if exe, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exe)
	}

	var driverDir string
	wc, driverDir, err = ensureWinDivertFiles(ctx, wc, exeDir)
	if err != nil {
		return err
	}

	if err := adapter.ConfigureWinDivertDLL(filepath.Join(driverDir, "WinDivert.dll")); err != nil {
		return fmt.Errorf("configure WinDivert.dll failed: %w", err)
	}

	// Service mode: do not stop/uninstall the global WinDivert driver on shutdown.
	report, cleanup, err := driver.EnsureWithReport(ctx, driver.Config{
		Dir:           driverDir,
		SysName:       wc.WinDivertSys,
		ServiceName:   wc.WinDivertSvcName,
		AutoInstall:   wc.AutoInstallDriver,
		AutoUninstall: wc.AutoUninstallDriver,
		AutoStop:      false,
	})
	if err != nil {
		logWinDivertReport(report)
		return fmt.Errorf("driver ensure failed: %w", err)
	}
	logWinDivertReport(report)
	if cleanup != nil {
		defer func() {
			if err := cleanup(); err != nil {
				logError("windivert_cleanup_failed", "driver cleanup failed", err)
			}
		}()
	}

	ad, err := adapter.NewWinDivert(wc.Filter, wc.AdapterOpts)
	if err != nil {
		return fmt.Errorf("WinDivert open failed: %w", err)
	}
	eng := engine.New(cfg, ad)

	errCh := make(chan error, 1)
	go func() {
		runErr := eng.Run(ctx)
		logInfo("engine_stats", "engine stats", "stats", eng.Stats())
		errCh <- runErr
	}()

	curCfg := cfg
	curWc := wc
	logInfo("engine_started", fmt.Sprintf("engine started (workers=%d)", curCfg.WorkerCount), "workers", curCfg.WorkerCount, "split_mode", curCfg.SplitMode, "split_chunk", curCfg.SplitChunk)

	for {
		select {
		case <-reload:
			newCfg, newWc, err := effectiveWindowsConfig(args, setFlags, true)
			if err != nil {
				logError("reload_failed", "reload failed", err)
				continue
			}

			if strings.TrimSpace(newWc.WinDivertDir) != strings.TrimSpace(curWc.WinDivertDir) ||
				strings.TrimSpace(newWc.WinDivertSys) != strings.TrimSpace(curWc.WinDivertSys) {
				logWarn("reload_restart_required_driver_path", "reload: windivert_dir/sys changed; requires service restart to apply", "windivert_dir", newWc.WinDivertDir, "windivert_sys", newWc.WinDivertSys)
			}

			reopened := false
			if requiresWinDivertReopenForReload(curWc.Filter, curWc.AdapterOpts, newWc.Filter, newWc.AdapterOpts) {
				if err := ad.Reopen(newWc.Filter, newWc.AdapterOpts); err != nil {
					logError("reload_handle_reopen_failed", "reload: WinDivert handle reopen failed", err)
					continue
				}
				reopened = true
				curWc.Filter = newWc.Filter
				curWc.AdapterOpts = newWc.AdapterOpts
				logInfo("reload_handle_reopened", "reload: WinDivert handle reopened",
					"filter", curWc.Filter,
					"queue_len", curWc.AdapterOpts.QueueLen,
					"queue_time_ms", curWc.AdapterOpts.QueueTime,
					"queue_size_bytes", curWc.AdapterOpts.QueueSize,
				)
			}

			// Best-effort update of queue parameters in-place when reopen is not
			// required. Reopen paths already applied the full option set.
			if !reopened && newWc.AdapterOpts.QueueLen != curWc.AdapterOpts.QueueLen {
				if err := ad.UpdateOptions(adapter.WinDivertOptions{QueueLen: newWc.AdapterOpts.QueueLen}); err != nil {
					logError("reload_queue_len_update_failed", "reload: update queue_len failed", err, "queue_len", newWc.AdapterOpts.QueueLen)
				} else {
					curWc.AdapterOpts.QueueLen = newWc.AdapterOpts.QueueLen
				}
			}
			if !reopened && newWc.AdapterOpts.QueueTime != curWc.AdapterOpts.QueueTime {
				if err := ad.UpdateOptions(adapter.WinDivertOptions{QueueTime: newWc.AdapterOpts.QueueTime}); err != nil {
					logError("reload_queue_time_update_failed", "reload: update queue_time_ms failed", err, "queue_time_ms", newWc.AdapterOpts.QueueTime)
				} else {
					curWc.AdapterOpts.QueueTime = newWc.AdapterOpts.QueueTime
				}
			}
			if !reopened && newWc.AdapterOpts.QueueSize != curWc.AdapterOpts.QueueSize {
				if err := ad.UpdateOptions(adapter.WinDivertOptions{QueueSize: newWc.AdapterOpts.QueueSize}); err != nil {
					logError("reload_queue_size_update_failed", "reload: update queue_size_bytes failed", err, "queue_size_bytes", newWc.AdapterOpts.QueueSize)
				} else {
					curWc.AdapterOpts.QueueSize = newWc.AdapterOpts.QueueSize
				}
			}

			applyCfg := newCfg
			if applyCfg.WorkerCount != curCfg.WorkerCount {
				logWarn("reload_restart_required_workers", "reload: workers changed; requires service restart to apply", "from", curCfg.WorkerCount, "to", applyCfg.WorkerCount)
				applyCfg.WorkerCount = curCfg.WorkerCount
			}
			if applyCfg.WorkerQueueSize != curCfg.WorkerQueueSize {
				logWarn("reload_restart_required_worker_queue", "reload: worker_queue_size changed; requires service restart to apply", "from", curCfg.WorkerQueueSize, "to", applyCfg.WorkerQueueSize)
				applyCfg.WorkerQueueSize = curCfg.WorkerQueueSize
			}

			if err := eng.Reload(applyCfg); err != nil {
				logError("reload_engine_apply_failed", "reload: engine config apply failed", err)
				continue
			}
			curCfg = applyCfg
			logInfo("reload_engine_applied", "reload: engine config applied", "split_mode", curCfg.SplitMode, "split_chunk", curCfg.SplitChunk, "collect_timeout", curCfg.CollectTimeout)

		case err := <-errCh:
			if err != nil && !errors.Is(err, context.Canceled) {
				return fmt.Errorf("engine stopped: %w", err)
			}
			return nil
		case <-ctx.Done():
			err := <-errCh
			if err != nil && !errors.Is(err, context.Canceled) {
				return fmt.Errorf("engine stopped: %w", err)
			}
			return nil
		}
	}
}

func logWinDivertReport(report driver.Report) {
	if report.ResolvedDir == "" && report.ServiceName == "" {
		return
	}
	logInfo("windivert_state", "windivert state",
		"dir", report.ResolvedDir,
		"sys", report.ResolvedSysPath,
		"files_present", report.FilesPresent,
		"service", report.ServiceName,
		"exists", report.ServiceExists,
		"running", report.ServiceRunning,
		"created", report.ServiceCreated,
		"reconfigured", report.ServiceReconfigured,
		"started", report.ServiceStarted,
		"bin_path", report.ServiceBinPath,
		"bin_path_exists", report.ServiceBinPathExists,
		"bin_path_matches", report.ServiceBinPathMatchesDesired,
		"cleanup_stop", report.CleanupWillStop,
		"cleanup_delete", report.CleanupWillDelete,
	)
}

func parseSplitMode(value string) (engine.SplitMode, error) {
	switch strings.ToLower(value) {
	case "immediate":
		return engine.SplitModeImmediate, nil
	case "tls-hello":
		return engine.SplitModeTLSHello, nil
	default:
		return engine.SplitModeTLSHello, errors.New("expected tls-hello or immediate")
	}
}

func printWindowsReloadability(w io.Writer) {
	if w == nil {
		return
	}

	fmt.Fprintln(w, "reloadable (in-place):")
	for _, item := range windowsReloadableSettings {
		fmt.Fprintln(w, "- "+item)
	}
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "restart-required:")
	for _, item := range windowsRestartRequiredSettings {
		fmt.Fprintln(w, "- "+item)
	}
}
