package engine

import (
	"fmt"
	"runtime"
	"time"
)

type SplitMode uint8

const (
	SplitModeImmediate SplitMode = iota
	SplitModeTLSHello
)

func (m SplitMode) String() string {
	switch m {
	case SplitModeImmediate:
		return "immediate"
	case SplitModeTLSHello:
		return "tls-hello"
	default:
		return fmt.Sprintf("SplitMode(%d)", uint8(m))
	}
}

type Config struct {
	SplitMode                   SplitMode
	SplitChunk                  int
	CollectTimeout              time.Duration
	MaxBufferBytes              int
	MaxHeldPackets              int
	MaxSegmentPayload           int
	MaxFlowsPerWorker           int
	MaxReassemblyBytesPerWorker int
	MaxHeldBytesPerWorker       int
	WorkerCount                 int
	WorkerQueueSize             int
	FlowIdleTimeout             time.Duration
	GCInterval                  time.Duration

	// ShutdownFailOpenTimeout bounds the time spent per worker trying to
	// fail-open and drain held/queued packets during shutdown.
	ShutdownFailOpenTimeout time.Duration
	// ShutdownFailOpenMaxPackets bounds the number of packets per worker that
	// will be reinjected during shutdown fail-open. 0 means use a safe default.
	ShutdownFailOpenMaxPackets int
	// AdapterFlushTimeout bounds the time spent draining adapter-level pending
	// packets on shutdown.
	AdapterFlushTimeout time.Duration
}

func DefaultConfig() Config {
	return Config{
		SplitMode:                   SplitModeTLSHello,
		SplitChunk:                  5,
		CollectTimeout:              250 * time.Millisecond,
		MaxBufferBytes:              64 * 1024,
		MaxHeldPackets:              32,
		MaxSegmentPayload:           1460,
		MaxFlowsPerWorker:           4096,
		MaxReassemblyBytesPerWorker: 64 * 1024 * 1024,
		MaxHeldBytesPerWorker:       64 * 1024 * 1024,
		WorkerCount:                 runtime.NumCPU(),
		WorkerQueueSize:             1024,
		FlowIdleTimeout:             30 * time.Second,
		GCInterval:                  5 * time.Second,

		ShutdownFailOpenTimeout:    5 * time.Second,
		ShutdownFailOpenMaxPackets: 200000,
		AdapterFlushTimeout:        2 * time.Second,
	}
}
