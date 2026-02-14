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
	maxFlows := flag.Int("max-flows-per-worker", cfg.MaxFlowsPerWorker, "max tracked flows per worker (0=unlimited)")
	maxReassembly := flag.Int("max-reassembly-bytes-per-worker", cfg.MaxReassemblyBytesPerWorker, "max total reassembly bytes per worker (0=unlimited)")
	maxHeldBytes := flag.Int("max-held-bytes-per-worker", cfg.MaxHeldBytesPerWorker, "max total held packet bytes per worker (0=unlimited)")
	shutdownFailOpenTimeout := flag.Duration("shutdown-fail-open-timeout", cfg.ShutdownFailOpenTimeout, "shutdown fail-open drain timeout per worker (0=use default)")
	shutdownFailOpenMaxPkts := flag.Int("shutdown-fail-open-max-pkts", cfg.ShutdownFailOpenMaxPackets, "shutdown fail-open max packets per worker (0=use default)")
	adapterFlushTimeout := flag.Duration("adapter-flush-timeout", cfg.AdapterFlushTimeout, "adapter flush timeout on shutdown (0=use default)")
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
	if *flowTimeout < 1*time.Millisecond {
		log.Fatal("flow-timeout must be >= 1ms")
	}
	if *gcInterval < 1*time.Millisecond {
		log.Fatal("gc-interval must be >= 1ms")
	}
	if *maxFlows < 0 {
		log.Fatal("max-flows-per-worker must be >= 0")
	}
	if *maxReassembly < 0 {
		log.Fatal("max-reassembly-bytes-per-worker must be >= 0")
	}
	if *maxHeldBytes < 0 {
		log.Fatal("max-held-bytes-per-worker must be >= 0")
	}
	if *shutdownFailOpenTimeout < 0 {
		log.Fatal("shutdown-fail-open-timeout must be >= 0")
	}
	if *shutdownFailOpenMaxPkts < 0 {
		log.Fatal("shutdown-fail-open-max-pkts must be >= 0")
	}
	if *adapterFlushTimeout < 0 {
		log.Fatal("adapter-flush-timeout must be >= 0")
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
	cfg.MaxFlowsPerWorker = *maxFlows
	cfg.MaxReassemblyBytesPerWorker = *maxReassembly
	cfg.MaxHeldBytesPerWorker = *maxHeldBytes
	cfg.ShutdownFailOpenTimeout = *shutdownFailOpenTimeout
	cfg.ShutdownFailOpenMaxPackets = *shutdownFailOpenMaxPkts
	cfg.AdapterFlushTimeout = *adapterFlushTimeout
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
