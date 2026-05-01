//go:build freebsd

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fk-gov/internal/engine"
)

func writeFreeBSDTempJSON(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cfg.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func freeBSDExampleConfigRefs() *freebsdFlagRefs {
	cfg := engine.DefaultConfig()
	splitMode := "tls-hello"
	splitChunk := cfg.SplitChunk
	collectTimeout := cfg.CollectTimeout
	maxBuffer := cfg.MaxBufferBytes
	maxHeld := cfg.MaxHeldPackets
	maxSegPayload := cfg.MaxSegmentPayload
	workers := cfg.WorkerCount
	flowTimeout := cfg.FlowIdleTimeout
	gcInterval := cfg.GCInterval
	maxFlows := cfg.MaxFlowsPerWorker
	maxReassembly := cfg.MaxReassemblyBytesPerWorker
	maxHeldBytes := cfg.MaxHeldBytesPerWorker
	shutdownFailOpenTimeout := cfg.ShutdownFailOpenTimeout
	shutdownFailOpenMaxPkts := cfg.ShutdownFailOpenMaxPackets
	adapterFlushTimeout := cfg.AdapterFlushTimeout
	statsInterval := defaultStatsInterval
	policies := cfg.Policies
	divertPort := 12345

	return &freebsdFlagRefs{
		SplitMode:                  &splitMode,
		SplitChunk:                 &splitChunk,
		CollectTimeout:             &collectTimeout,
		MaxBuffer:                  &maxBuffer,
		MaxHeld:                    &maxHeld,
		MaxSegPayload:              &maxSegPayload,
		Workers:                    &workers,
		FlowTimeout:                &flowTimeout,
		GCInterval:                 &gcInterval,
		MaxFlows:                   &maxFlows,
		MaxReassembly:              &maxReassembly,
		MaxHeldBytes:               &maxHeldBytes,
		ShutdownFailOpenTimeout:    &shutdownFailOpenTimeout,
		ShutdownFailOpenMaxPackets: &shutdownFailOpenMaxPkts,
		AdapterFlushTimeout:        &adapterFlushTimeout,
		StatsInterval:              &statsInterval,
		Policies:                   &policies,
		DivertPort:                 &divertPort,
	}
}

func TestApplyFreeBSDJSONConfig_EngineStatsInterval(t *testing.T) {
	splitChunk := 5
	statsInterval := defaultStatsInterval
	divertPort := 10000
	refs := &freebsdFlagRefs{
		SplitChunk:    &splitChunk,
		StatsInterval: &statsInterval,
		DivertPort:    &divertPort,
	}

	path := writeFreeBSDTempJSON(t, `{
  "engine": {
    "split_chunk": 9,
    "stats_interval": "2m"
  },
  "freebsd": {
    "divert_port": 12000
  }
}`)

	if err := applyFreeBSDJSONConfig(path, map[string]bool{}, refs); err != nil {
		t.Fatalf("apply config: %v", err)
	}

	if splitChunk != 9 {
		t.Fatalf("splitChunk = %d, want 9", splitChunk)
	}
	if statsInterval != 2*time.Minute {
		t.Fatalf("statsInterval = %s, want 2m", statsInterval)
	}
	if divertPort != 12000 {
		t.Fatalf("divertPort = %d, want 12000", divertPort)
	}
}

func TestApplyFreeBSDJSONConfig_ExplicitStatsIntervalFlagOverridesConfig(t *testing.T) {
	statsInterval := defaultStatsInterval
	refs := &freebsdFlagRefs{StatsInterval: &statsInterval}
	path := writeFreeBSDTempJSON(t, `{
  "engine": {
    "stats_interval": "2m"
  }
}`)

	if err := applyFreeBSDJSONConfig(path, map[string]bool{"stats-interval": true}, refs); err != nil {
		t.Fatalf("apply config: %v", err)
	}

	if statsInterval != defaultStatsInterval {
		t.Fatalf("statsInterval = %s, want default", statsInterval)
	}
}

func TestFreeBSDExampleConfigApplies(t *testing.T) {
	path := filepath.Join("..", "..", "docs", "examples", "splitter.freebsd.json")
	if err := applyFreeBSDJSONConfig(path, map[string]bool{}, freeBSDExampleConfigRefs()); err != nil {
		t.Fatalf("apply example config: %v", err)
	}
}

func TestApplyFreeBSDJSONConfigRejectsUnknownFields(t *testing.T) {
	path := writeFreeBSDTempJSON(t, `{
  "freebsd": {
    "divert_port": 12000,
    "divert_socket": "bad"
  }
}`)

	err := applyFreeBSDJSONConfig(path, map[string]bool{}, freeBSDExampleConfigRefs())
	if err == nil || !strings.Contains(err.Error(), `unknown field "divert_socket"`) {
		t.Fatalf("expected unknown field error, got %v", err)
	}
}
