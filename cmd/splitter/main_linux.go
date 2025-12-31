//go:build linux

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
	const (
		defaultQueueNum    = 100
		defaultQueueMaxLen = 4096
		defaultCopyRange   = 0xffff
		defaultMark        = 1
	)

	splitMode := flag.String("split-mode", "tls-hello", "split trigger: tls-hello or immediate")
	splitChunk := flag.Int("split-chunk", cfg.SplitChunk, "first split size in bytes")
	collectTimeout := flag.Duration("collect-timeout", cfg.CollectTimeout, "reassembly collect timeout")
	maxBuffer := flag.Int("max-buffer", cfg.MaxBufferBytes, "max reassembly buffer size in bytes")
	maxHeld := flag.Int("max-held-pkts", cfg.MaxHeldPackets, "max held packets per flow")
	maxSegPayload := flag.Int("max-seg-payload", cfg.MaxSegmentPayload, "max segment payload size (0=unlimited)")
	workers := flag.Int("workers", cfg.WorkerCount, "worker count for sharded processing")
	flowTimeout := flag.Duration("flow-timeout", cfg.FlowIdleTimeout, "idle timeout for flow cleanup")
	gcInterval := flag.Duration("gc-interval", cfg.GCInterval, "flow GC interval")
	queueNum := flag.Int("queue-num", defaultQueueNum, "NFQUEUE number")
	queueMaxLen := flag.Int("queue-maxlen", defaultQueueMaxLen, "NFQUEUE maxlen (0=kernel default)")
	copyRange := flag.Int("copy-range", defaultCopyRange, "NFQUEUE copy range in bytes (0=full packet)")
	mark := flag.Int("mark", defaultMark, "SO_MARK for reinjected packets")
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
	if *queueNum < 0 || *queueNum > 65535 {
		log.Fatal("queue-num must be in 0..65535")
	}
	if *queueMaxLen < 0 {
		log.Fatal("queue-maxlen must be >= 0")
	}
	if *copyRange < 0 {
		log.Fatal("copy-range must be >= 0")
	}
	if *mark < 0 {
		log.Fatal("mark must be >= 0")
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

	if *mark == 0 {
		log.Printf("warning: mark=0; ensure NFQUEUE bypass rules prevent reinjection loops")
	}

	opts := adapter.NFQueueOptions{
		QueueNum:    uint16(*queueNum),
		QueueMaxLen: uint32(*queueMaxLen),
		CopyRange:   uint32(*copyRange),
		Mark:        uint32(*mark),
	}
	ad, err := adapter.NewNFQueue(opts)
	if err != nil {
		log.Fatalf("NFQUEUE open failed: %v", err)
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
