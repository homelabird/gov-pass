//go:build windows

package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"fk-gov/internal/engine"
)

func applyEngineJSONConfig(dst *engine.Config, cfg *engineJSONConfig) error {
	if dst == nil || cfg == nil {
		return nil
	}
	if cfg.SplitMode != nil {
		value := strings.TrimSpace(*cfg.SplitMode)
		if value != "" {
			mode, err := parseSplitMode(value)
			if err != nil {
				return fmt.Errorf("engine.split_mode: %w", err)
			}
			dst.SplitMode = mode
		}
	}
	if cfg.SplitChunk != nil {
		dst.SplitChunk = *cfg.SplitChunk
	}
	if cfg.CollectTimeout != nil {
		d, err := parseOptionalDuration("engine.collect_timeout", *cfg.CollectTimeout)
		if err != nil {
			return err
		}
		if d != nil {
			dst.CollectTimeout = *d
		}
	}
	if cfg.MaxBufferBytes != nil {
		dst.MaxBufferBytes = *cfg.MaxBufferBytes
	}
	if cfg.MaxHeldPackets != nil {
		dst.MaxHeldPackets = *cfg.MaxHeldPackets
	}
	if cfg.MaxSegmentPayload != nil {
		dst.MaxSegmentPayload = *cfg.MaxSegmentPayload
	}
	if cfg.Workers != nil {
		dst.WorkerCount = *cfg.Workers
	}
	if cfg.FlowIdleTimeout != nil {
		d, err := parseOptionalDuration("engine.flow_idle_timeout", *cfg.FlowIdleTimeout)
		if err != nil {
			return err
		}
		if d != nil {
			dst.FlowIdleTimeout = *d
		}
	}
	if cfg.GCInterval != nil {
		d, err := parseOptionalDuration("engine.gc_interval", *cfg.GCInterval)
		if err != nil {
			return err
		}
		if d != nil {
			dst.GCInterval = *d
		}
	}
	if cfg.MaxFlowsPerWorker != nil {
		dst.MaxFlowsPerWorker = *cfg.MaxFlowsPerWorker
	}
	if cfg.MaxReassemblyBytesPerWorker != nil {
		dst.MaxReassemblyBytesPerWorker = *cfg.MaxReassemblyBytesPerWorker
	}
	if cfg.MaxHeldBytesPerWorker != nil {
		dst.MaxHeldBytesPerWorker = *cfg.MaxHeldBytesPerWorker
	}
	if cfg.ShutdownFailOpenTimeout != nil {
		d, err := parseOptionalDuration("engine.shutdown_fail_open_timeout", *cfg.ShutdownFailOpenTimeout)
		if err != nil {
			return err
		}
		if d != nil {
			dst.ShutdownFailOpenTimeout = *d
		}
	}
	if cfg.ShutdownFailOpenMaxPackets != nil {
		dst.ShutdownFailOpenMaxPackets = *cfg.ShutdownFailOpenMaxPackets
	}
	if cfg.AdapterFlushTimeout != nil {
		d, err := parseOptionalDuration("engine.adapter_flush_timeout", *cfg.AdapterFlushTimeout)
		if err != nil {
			return err
		}
		if d != nil {
			dst.AdapterFlushTimeout = *d
		}
	}
	if cfg.Policies != nil {
		policies, err := parseEnginePolicies(cfg.Policies)
		if err != nil {
			return err
		}
		dst.Policies = policies
	}
	return nil
}

func parseEngineStatsIntervalConfig(cfg *engineJSONConfig) (*time.Duration, error) {
	if cfg == nil || cfg.StatsInterval == nil {
		return nil, nil
	}
	return parseOptionalDuration("engine.stats_interval", *cfg.StatsInterval)
}

func validateEngineConfig(cfg engine.Config) error {
	if cfg.SplitChunk < 1 {
		return errors.New("split-chunk must be >= 1")
	}
	if cfg.MaxBufferBytes < 1 {
		return errors.New("max-buffer must be >= 1")
	}
	if cfg.MaxHeldPackets < 1 {
		return errors.New("max-held-pkts must be >= 1")
	}
	if cfg.MaxSegmentPayload < 0 {
		return errors.New("max-seg-payload must be >= 0")
	}
	if cfg.WorkerCount < 1 {
		return errors.New("workers must be >= 1")
	}
	if cfg.CollectTimeout < time.Millisecond {
		return errors.New("collect-timeout must be >= 1ms")
	}
	if cfg.FlowIdleTimeout < time.Millisecond {
		return errors.New("flow-timeout must be >= 1ms")
	}
	if cfg.GCInterval < time.Millisecond {
		return errors.New("gc-interval must be >= 1ms")
	}
	if cfg.MaxFlowsPerWorker < 0 {
		return errors.New("max-flows-per-worker must be >= 0")
	}
	if cfg.MaxReassemblyBytesPerWorker < 0 {
		return errors.New("max-reassembly-bytes-per-worker must be >= 0")
	}
	if cfg.MaxHeldBytesPerWorker < 0 {
		return errors.New("max-held-bytes-per-worker must be >= 0")
	}
	if cfg.ShutdownFailOpenTimeout < 0 {
		return errors.New("shutdown-fail-open-timeout must be >= 0")
	}
	if cfg.ShutdownFailOpenMaxPackets < 0 {
		return errors.New("shutdown-fail-open-max-pkts must be >= 0")
	}
	if cfg.AdapterFlushTimeout < 0 {
		return errors.New("adapter-flush-timeout must be >= 0")
	}
	return engine.ValidateConfig(cfg)
}
