//go:build windows

package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"fk-gov/internal/adapter"
	"fk-gov/internal/driver"
	"fk-gov/internal/engine"
)

func main() {
	cfg := engine.DefaultConfig()
	const (
		defaultQueueLen    = 4096
		defaultQueueTimeMs = 2000
		defaultQueueSize   = 32 * 1024 * 1024
		defaultServiceName = "WinDivert"
	)
	splitMode := flag.String("split-mode", "tls-hello", "split trigger: tls-hello or immediate")
	splitChunk := flag.Int("split-chunk", cfg.SplitChunk, "first split size in bytes")
	collectTimeout := flag.Duration("collect-timeout", cfg.CollectTimeout, "reassembly collect timeout")
	maxBuffer := flag.Int("max-buffer", cfg.MaxBufferBytes, "max reassembly buffer size in bytes")
	maxHeld := flag.Int("max-held-pkts", cfg.MaxHeldPackets, "max held packets per flow")
	workers := flag.Int("workers", cfg.WorkerCount, "worker count for sharded processing")
	flowTimeout := flag.Duration("flow-timeout", cfg.FlowIdleTimeout, "idle timeout for flow cleanup")
	gcInterval := flag.Duration("gc-interval", cfg.GCInterval, "flow GC interval")
	filter := flag.String("filter", "outbound and ip and tcp.DstPort == 443", "WinDivert filter")
	queueLen := flag.Uint("queue-len", defaultQueueLen, "WinDivert queue length (0=driver default)")
	queueTime := flag.Uint("queue-time", defaultQueueTimeMs, "WinDivert queue time in ms (0=driver default)")
	queueSize := flag.Uint("queue-size", defaultQueueSize, "WinDivert queue size in bytes (0=driver default)")
	driverDir := flag.String("windivert-dir", "", "directory containing WinDivert.dll/.sys/.cat (default: exe dir)")
	driverSys := flag.String("windivert-sys", "", "driver sys filename (default: WinDivert64.sys or WinDivert.sys)")
	autoInstall := flag.Bool("auto-install", true, "auto install/start WinDivert driver")
	autoUninstall := flag.Bool("auto-uninstall", true, "auto uninstall if installed by this run")
	flag.Parse()

	mode, err := parseSplitMode(*splitMode)
	if err != nil {
		log.Fatalf("invalid split-mode: %v", err)
	}
	if *splitChunk < 1 {
		log.Fatal("split-chunk must be >= 1")
	}
	if *maxBuffer < 1 {
		log.Fatal("max-buffer must be >= 1")
	}
	if *maxHeld < 1 {
		log.Fatal("max-held-pkts must be >= 1")
	}
	if *workers < 1 {
		log.Fatal("workers must be >= 1")
	}
	if *collectTimeout < 1*time.Millisecond {
		log.Fatal("collect-timeout must be >= 1ms")
	}

	cfg.SplitMode = mode
	cfg.SplitChunk = *splitChunk
	cfg.CollectTimeout = *collectTimeout
	cfg.MaxBufferBytes = *maxBuffer
	cfg.MaxHeldPackets = *maxHeld
	cfg.WorkerCount = *workers
	cfg.FlowIdleTimeout = *flowTimeout
	cfg.GCInterval = *gcInterval

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	exeDir := ""
	if *driverDir == "" {
		if exe, err := os.Executable(); err == nil {
			exeDir = filepath.Dir(exe)
		}
	} else {
		exeDir = *driverDir
	}
	if exeDir != "" {
		if err := driver.PrependPath(exeDir); err != nil {
			log.Fatalf("set PATH failed: %v", err)
		}
	}

	cleanup, err := driver.Ensure(ctx, driver.Config{
		Dir:           exeDir,
		SysName:       *driverSys,
		ServiceName:   defaultServiceName,
		AutoInstall:   *autoInstall,
		AutoUninstall: *autoUninstall,
		AutoStop:      true,
	})
	if err != nil {
		log.Fatalf("driver ensure failed: %v", err)
	}
	if cleanup != nil {
		defer func() {
			if err := cleanup(); err != nil {
				log.Printf("driver cleanup failed: %v", err)
			}
		}()
	}

	opts := adapter.WinDivertOptions{
		QueueLen:  uint64(*queueLen),
		QueueTime: uint64(*queueTime),
		QueueSize: uint64(*queueSize),
	}
	ad, err := adapter.NewWinDivert(*filter, opts)
	if err != nil {
		log.Fatalf("WinDivert open failed: %v", err)
	}
	eng := engine.New(cfg, ad)

	if err := eng.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("engine stopped: %v", err)
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
