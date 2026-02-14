//go:build windows

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
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
	defaultWinDivertFilter             = "outbound and ip and tcp.DstPort == 443"
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
	flag.Parse()

	setFlags := make(map[string]bool)
	flag.CommandLine.Visit(func(f *flag.Flag) {
		setFlags[f.Name] = true
	})

	if !*asService && isWindowsServiceProcess() {
		*asService = true
	}

	args := windowsCLIArgs{
		SplitMode:      *splitMode,
		SplitChunk:     *splitChunk,
		CollectTimeout: *collectTimeout,
		MaxBufferBytes: *maxBuffer,
		MaxHeldPackets: *maxHeld,
		MaxSegPayload:  *maxSegPayload,
		Workers:        *workers,
		FlowTimeout:    *flowTimeout,
		GCInterval:     *gcInterval,
		MaxFlows:       *maxFlows,
		MaxReassembly:  *maxReassembly,
		MaxHeldBytes:   *maxHeldBytes,

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
		return runService(*serviceName, *serviceLog, func(ctx context.Context, reload <-chan struct{}) error {
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

	AutoInstallDriver   bool
	AutoUninstallDriver bool
	AutoDownloadFiles   bool
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

	if driverDir != "" {
		if err := driver.PrependPath(driverDir); err != nil {
			return fmt.Errorf("set PATH failed: %w", err)
		}
	}

	cleanup, err := driver.Ensure(ctx, driver.Config{
		Dir:           driverDir,
		SysName:       wc.WinDivertSys,
		ServiceName:   wc.WinDivertSvcName,
		AutoInstall:   wc.AutoInstallDriver,
		AutoUninstall: wc.AutoUninstallDriver,
		AutoStop:      true,
	})
	if err != nil {
		return fmt.Errorf("driver ensure failed: %w", err)
	}
	if cleanup != nil {
		defer func() {
			if err := cleanup(); err != nil {
				log.Printf("driver cleanup failed: %v", err)
			}
		}()
	}

	ad, err := adapter.NewWinDivert(wc.Filter, wc.AdapterOpts)
	if err != nil {
		return fmt.Errorf("WinDivert open failed: %w", err)
	}
	eng := engine.New(cfg, ad)

	if err := eng.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
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

	if driverDir != "" {
		if err := driver.PrependPath(driverDir); err != nil {
			return fmt.Errorf("set PATH failed: %w", err)
		}
	}

	// Service mode: do not stop/uninstall the global WinDivert driver on shutdown.
	cleanup, err := driver.Ensure(ctx, driver.Config{
		Dir:           driverDir,
		SysName:       wc.WinDivertSys,
		ServiceName:   wc.WinDivertSvcName,
		AutoInstall:   wc.AutoInstallDriver,
		AutoUninstall: wc.AutoUninstallDriver,
		AutoStop:      false,
	})
	if err != nil {
		return fmt.Errorf("driver ensure failed: %w", err)
	}
	if cleanup != nil {
		defer func() {
			if err := cleanup(); err != nil {
				log.Printf("driver cleanup failed: %v", err)
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
		errCh <- eng.Run(ctx)
	}()

	curCfg := cfg
	curWc := wc
	log.Printf("engine started (workers=%d)", curCfg.WorkerCount)

	for {
		select {
		case <-reload:
			newCfg, newWc, err := effectiveWindowsConfig(args, setFlags, true)
			if err != nil {
				log.Printf("reload failed: %v", err)
				continue
			}

			if newWc.Filter != curWc.Filter {
				log.Printf("reload: windivert.filter changed; requires service restart to apply")
			}
			if strings.TrimSpace(newWc.WinDivertDir) != strings.TrimSpace(curWc.WinDivertDir) ||
				strings.TrimSpace(newWc.WinDivertSys) != strings.TrimSpace(curWc.WinDivertSys) {
				log.Printf("reload: windivert_dir/sys changed; requires service restart to apply")
			}

			// Best-effort update of queue parameters in-place. "0" means "use driver
			// default", which cannot be applied without re-opening the handle.
			if newWc.AdapterOpts.QueueLen != curWc.AdapterOpts.QueueLen {
				if newWc.AdapterOpts.QueueLen == 0 {
					log.Printf("reload: queue_len=0 requires service restart to apply (revert to driver default)")
				} else if err := ad.UpdateOptions(adapter.WinDivertOptions{QueueLen: newWc.AdapterOpts.QueueLen}); err != nil {
					log.Printf("reload: update queue_len failed: %v", err)
				} else {
					curWc.AdapterOpts.QueueLen = newWc.AdapterOpts.QueueLen
				}
			}
			if newWc.AdapterOpts.QueueTime != curWc.AdapterOpts.QueueTime {
				if newWc.AdapterOpts.QueueTime == 0 {
					log.Printf("reload: queue_time_ms=0 requires service restart to apply (revert to driver default)")
				} else if err := ad.UpdateOptions(adapter.WinDivertOptions{QueueTime: newWc.AdapterOpts.QueueTime}); err != nil {
					log.Printf("reload: update queue_time_ms failed: %v", err)
				} else {
					curWc.AdapterOpts.QueueTime = newWc.AdapterOpts.QueueTime
				}
			}
			if newWc.AdapterOpts.QueueSize != curWc.AdapterOpts.QueueSize {
				if newWc.AdapterOpts.QueueSize == 0 {
					log.Printf("reload: queue_size_bytes=0 requires service restart to apply (revert to driver default)")
				} else if err := ad.UpdateOptions(adapter.WinDivertOptions{QueueSize: newWc.AdapterOpts.QueueSize}); err != nil {
					log.Printf("reload: update queue_size_bytes failed: %v", err)
				} else {
					curWc.AdapterOpts.QueueSize = newWc.AdapterOpts.QueueSize
				}
			}

			applyCfg := newCfg
			if applyCfg.WorkerCount != curCfg.WorkerCount {
				log.Printf("reload: workers changed; requires service restart to apply (%d -> %d)", curCfg.WorkerCount, applyCfg.WorkerCount)
				applyCfg.WorkerCount = curCfg.WorkerCount
			}
			if applyCfg.WorkerQueueSize != curCfg.WorkerQueueSize {
				log.Printf("reload: worker_queue_size changed; requires service restart to apply (%d -> %d)", curCfg.WorkerQueueSize, applyCfg.WorkerQueueSize)
				applyCfg.WorkerQueueSize = curCfg.WorkerQueueSize
			}

			if err := eng.Reload(applyCfg); err != nil {
				log.Printf("reload: engine config apply failed: %v", err)
				continue
			}
			curCfg = applyCfg
			log.Printf("reload: engine config applied (split_mode=%v split_chunk=%d collect_timeout=%s)", curCfg.SplitMode, curCfg.SplitChunk, curCfg.CollectTimeout)

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
