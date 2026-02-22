package engine

import (
	"testing"

	"fk-gov/internal/adapter"
)

func TestEngineReload_RejectsWorkerCountChange(t *testing.T) {
	cfg := DefaultConfig()
	cfg.WorkerCount = 2

	eng := New(cfg, adapter.NewStub())

	next := cfg
	next.WorkerCount = 3

	if err := eng.Reload(next); err == nil {
		t.Fatalf("expected reload to fail when workers change")
	}
}

func TestEngineReload_RejectsWorkerQueueSizeChange(t *testing.T) {
	cfg := DefaultConfig()
	cfg.WorkerCount = 2

	eng := New(cfg, adapter.NewStub())

	next := cfg
	next.WorkerQueueSize = cfg.WorkerQueueSize + 1

	if err := eng.Reload(next); err == nil {
		t.Fatalf("expected reload to fail when worker queue size changes")
	}
}

func TestEngineReload_UpdatesWorkerConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.WorkerCount = 2

	eng := New(cfg, adapter.NewStub())

	next := cfg
	next.SplitChunk = cfg.SplitChunk + 1
	next.MaxFlowsPerWorker = cfg.MaxFlowsPerWorker + 1
	next.MaxHeldPackets = cfg.MaxHeldPackets + 1

	if err := eng.Reload(next); err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	if eng.cfg.SplitChunk != next.SplitChunk {
		t.Fatalf("engine cfg not updated: got %d want %d", eng.cfg.SplitChunk, next.SplitChunk)
	}
	for i, w := range eng.workers {
		wcfg := w.cfg.Load()
		if wcfg == nil {
			t.Fatalf("worker %d cfg is nil", i)
		}
		if wcfg.SplitChunk != next.SplitChunk {
			t.Fatalf("worker %d SplitChunk: got %d want %d", i, wcfg.SplitChunk, next.SplitChunk)
		}
		if wcfg.MaxFlowsPerWorker != next.MaxFlowsPerWorker {
			t.Fatalf("worker %d MaxFlowsPerWorker: got %d want %d", i, wcfg.MaxFlowsPerWorker, next.MaxFlowsPerWorker)
		}
		if wcfg.MaxHeldPackets != next.MaxHeldPackets {
			t.Fatalf("worker %d MaxHeldPackets: got %d want %d", i, wcfg.MaxHeldPackets, next.MaxHeldPackets)
		}
	}
}

