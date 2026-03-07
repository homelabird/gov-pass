package engine

import (
	"net/netip"
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
	next.Policies = []Policy{{
		DstPrefixes: []netip.Prefix{netip.MustParsePrefix("1.1.1.0/24")},
		Skip:        true,
	}}

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
		if len(wcfg.Policies) != 1 || !wcfg.Policies[0].Skip {
			t.Fatalf("worker %d Policies not updated: %+v", i, wcfg.Policies)
		}
	}
}

func TestEngineReload_RejectsSNIPolicyWithEffectiveImmediateMode(t *testing.T) {
	cfg := DefaultConfig()
	cfg.WorkerCount = 2

	eng := New(cfg, adapter.NewStub())

	next := cfg
	next.SplitMode = SplitModeImmediate
	next.Policies = []Policy{{
		SNISuffixes:   []string{"example.com"},
		HasSplitChunk: true,
		SplitChunk:    9,
	}}

	if err := eng.Reload(next); err == nil {
		t.Fatal("expected reload to fail")
	}
}
