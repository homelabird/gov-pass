package engine

import (
	"fmt"
	"net/netip"
	"strings"
	"testing"
	"time"

	"fk-gov/internal/adapter"
)

func TestValidateConfig_AcceptsDefault(t *testing.T) {
	if err := ValidateConfig(DefaultConfig()); err != nil {
		t.Fatalf("ValidateConfig(DefaultConfig()) failed: %v", err)
	}
}

func TestValidateConfig_RejectsInvalidCoreFields(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*Config)
		wantError string
	}{
		{
			name: "split mode",
			mutate: func(cfg *Config) {
				cfg.SplitMode = SplitMode(99)
			},
			wantError: "split-mode must be tls-hello or immediate",
		},
		{
			name: "split chunk",
			mutate: func(cfg *Config) {
				cfg.SplitChunk = 0
			},
			wantError: "split-chunk must be >= 1",
		},
		{
			name: "collect timeout",
			mutate: func(cfg *Config) {
				cfg.CollectTimeout = time.Nanosecond
			},
			wantError: "collect-timeout must be >= 1ms",
		},
		{
			name: "max buffer",
			mutate: func(cfg *Config) {
				cfg.MaxBufferBytes = 0
			},
			wantError: "max-buffer must be >= 1",
		},
		{
			name: "max held packets",
			mutate: func(cfg *Config) {
				cfg.MaxHeldPackets = 0
			},
			wantError: "max-held-pkts must be >= 1",
		},
		{
			name: "max segment payload",
			mutate: func(cfg *Config) {
				cfg.MaxSegmentPayload = -1
			},
			wantError: "max-seg-payload must be >= 0",
		},
		{
			name: "workers",
			mutate: func(cfg *Config) {
				cfg.WorkerCount = 0
			},
			wantError: "workers must be >= 1",
		},
		{
			name: "worker queue size",
			mutate: func(cfg *Config) {
				cfg.WorkerQueueSize = 0
			},
			wantError: "worker-queue-size must be >= 1",
		},
		{
			name: "flow timeout",
			mutate: func(cfg *Config) {
				cfg.FlowIdleTimeout = time.Nanosecond
			},
			wantError: "flow-timeout must be >= 1ms",
		},
		{
			name: "gc interval",
			mutate: func(cfg *Config) {
				cfg.GCInterval = time.Nanosecond
			},
			wantError: "gc-interval must be >= 1ms",
		},
		{
			name: "max flows",
			mutate: func(cfg *Config) {
				cfg.MaxFlowsPerWorker = -1
			},
			wantError: "max-flows-per-worker must be >= 0",
		},
		{
			name: "max reassembly bytes",
			mutate: func(cfg *Config) {
				cfg.MaxReassemblyBytesPerWorker = -1
			},
			wantError: "max-reassembly-bytes-per-worker must be >= 0",
		},
		{
			name: "max held bytes",
			mutate: func(cfg *Config) {
				cfg.MaxHeldBytesPerWorker = -1
			},
			wantError: "max-held-bytes-per-worker must be >= 0",
		},
		{
			name: "shutdown fail-open timeout",
			mutate: func(cfg *Config) {
				cfg.ShutdownFailOpenTimeout = -time.Nanosecond
			},
			wantError: "shutdown-fail-open-timeout must be >= 0",
		},
		{
			name: "shutdown fail-open max packets",
			mutate: func(cfg *Config) {
				cfg.ShutdownFailOpenMaxPackets = -1
			},
			wantError: "shutdown-fail-open-max-pkts must be >= 0",
		},
		{
			name: "adapter flush timeout",
			mutate: func(cfg *Config) {
				cfg.AdapterFlushTimeout = -time.Nanosecond
			},
			wantError: "adapter-flush-timeout must be >= 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.mutate(&cfg)
			err := ValidateConfig(cfg)
			if err == nil {
				t.Fatalf("expected validation error containing %q", tt.wantError)
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("unexpected error: got %q want substring %q", err, tt.wantError)
			}
		})
	}
}

func TestValidatePolicy_RejectsInvalidSplitMode(t *testing.T) {
	err := ValidatePolicy(Policy{
		DstPrefixes:  []netip.Prefix{netip.MustParsePrefix("1.1.1.0/24")},
		HasSplitMode: true,
		SplitMode:    SplitMode(99),
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "split_mode must be tls-hello or immediate") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidatePolicy_RejectsSkipWithSplitOverrides(t *testing.T) {
	err := ValidatePolicy(Policy{
		DstPrefixes:   []netip.Prefix{netip.MustParsePrefix("1.1.1.0/24")},
		Skip:          true,
		HasSplitChunk: true,
		SplitChunk:    7,
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "skip=true cannot be combined with split overrides") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidatePolicy_RejectsInvalidSNISuffixes(t *testing.T) {
	tests := []string{
		"bad_suffix.example",
		"*.example.com",
		"example..com",
		"-example.com",
		"example.com/",
		"192.0.2.1",
	}
	for _, suffix := range tests {
		t.Run(suffix, func(t *testing.T) {
			err := ValidatePolicy(Policy{
				SNISuffixes: []string{suffix},
				Skip:        true,
			})
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), "sni_suffixes") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidatePolicy_AcceptsNormalizedSNISuffix(t *testing.T) {
	err := ValidatePolicy(Policy{
		SNISuffixes: []string{"www.example-1.com"},
		Skip:        true,
	})
	if err != nil {
		t.Fatalf("ValidatePolicy failed: %v", err)
	}
}

func TestNew_PanicsOnInvalidConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.WorkerCount = 0

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic")
		}
		if !strings.Contains(fmt.Sprint(r), "invalid engine config: workers must be >= 1") {
			t.Fatalf("unexpected panic: %v", r)
		}
	}()

	_ = New(cfg, adapter.NewStub())
}

func TestNewChecked_ReturnsInvalidConfigError(t *testing.T) {
	cfg := DefaultConfig()
	cfg.WorkerCount = 0

	eng, err := NewChecked(cfg, adapter.NewStub())
	if err == nil {
		t.Fatal("NewChecked returned nil error for invalid config")
	}
	if eng != nil {
		t.Fatalf("NewChecked returned engine for invalid config: %#v", eng)
	}
	if !strings.Contains(err.Error(), "workers must be >= 1") {
		t.Fatalf("unexpected error: %v", err)
	}
}
