package engine

import (
	"runtime"
	"time"
)

type SplitMode uint8

const (
	SplitModeImmediate SplitMode = iota
	SplitModeTLSHello
)

type Config struct {
	SplitMode       SplitMode
	SplitChunk      int
	CollectTimeout  time.Duration
	MaxBufferBytes  int
	MaxHeldPackets  int
	WorkerCount     int
	WorkerQueueSize int
	FlowIdleTimeout time.Duration
	GCInterval      time.Duration
}

func DefaultConfig() Config {
	return Config{
		SplitMode:       SplitModeTLSHello,
		SplitChunk:      5,
		CollectTimeout:  250 * time.Millisecond,
		MaxBufferBytes:  64 * 1024,
		MaxHeldPackets:  32,
		WorkerCount:     runtime.NumCPU(),
		WorkerQueueSize: 1024,
		FlowIdleTimeout: 30 * time.Second,
		GCInterval:      5 * time.Second,
	}
}
