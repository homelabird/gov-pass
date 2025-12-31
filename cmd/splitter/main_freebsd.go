//go:build freebsd

package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"fk-gov/internal/adapter"
	"fk-gov/internal/engine"
)

func main() {
	cfg := engine.DefaultConfig()
	const defaultDivertPort = 10000

	splitMode := flag.String("split-mode", "tls-hello", "split trigger: tls-hello or immediate")
	splitChunk := flag.Int("split-chunk", cfg.SplitChunk, "first split size in bytes")
	collectTimeout := flag.Duration("collect-timeout", cfg.CollectTimeout, "reassembly collect timeout")
	maxBuffer := flag.Int("max-buffer", cfg.MaxBufferBytes, "max reassembly buffer size in bytes")
	maxHeld := flag.Int("max-held-pkts", cfg.MaxHeldPackets, "max held packets per flow")
	maxSegPayload := flag.Int("max-seg-payload", cfg.MaxSegmentPayload, "max segment payload size (0=unlimited)")
	workers := flag.Int("workers", cfg.WorkerCount, "worker count for sharded processing")
	flowTimeout := flag.Duration("flow-timeout", cfg.FlowIdleTimeout, "idle timeout for flow cleanup")
	gcInterval := flag.Duration("gc-interval", cfg.GCInterval, "flow GC interval")
	divertPort := flag.Int("divert-port", defaultDivertPort, "pf divert-to port")
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
	if *maxSegPayload < 0 {
		log.Fatal("max-seg-payload must be >= 0")
	}
	if *workers < 1 {
		log.Fatal("workers must be >= 1")
	}
	if *collectTimeout < 1*time.Millisecond {
		log.Fatal("collect-timeout must be >= 1ms")
	}
	if *divertPort < 1 || *divertPort > 65535 {
		log.Fatal("divert-port must be in 1..65535")
	}

	cfg.SplitMode = mode
	cfg.SplitChunk = *splitChunk
	cfg.CollectTimeout = *collectTimeout
	cfg.MaxBufferBytes = *maxBuffer
	cfg.MaxHeldPackets = *maxHeld
	cfg.MaxSegmentPayload = *maxSegPayload
	cfg.WorkerCount = *workers
	cfg.FlowIdleTimeout = *flowTimeout
	cfg.GCInterval = *gcInterval

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	opts := adapter.DivertOptions{
		Port: uint16(*divertPort),
	}
	ad, err := adapter.NewDivert(opts)
	if err != nil {
		log.Fatalf("divert open failed: %v", err)
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
