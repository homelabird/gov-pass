package main

import (
	"context"
	"sync"
	"time"

	"fk-gov/internal/engine"
)

const defaultStatsInterval = 60 * time.Second

func startEngineStatsLogger(ctx context.Context, eng *engine.Engine, interval time.Duration) func() {
	if eng == nil || interval <= 0 {
		return func() {}
	}
	done := make(chan struct{})
	var once sync.Once
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				logInfo("engine_stats_periodic", "engine stats", "stats", eng.Stats())
			case <-ctx.Done():
				return
			case <-done:
				return
			}
		}
	}()
	return func() {
		once.Do(func() {
			close(done)
		})
	}
}
