//go:build freebsd

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
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
	policies := cfg.Policies
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
	configPath := flag.String("config", "", "path to config json")
	checkMode := flag.Bool("check", false, "run preflight checks and exit")
	checkJSON := flag.Bool("check-json", false, "run preflight checks and print JSON")
	flag.Parse()

	setFlags := make(map[string]bool)
	flag.CommandLine.Visit(func(f *flag.Flag) {
		setFlags[f.Name] = true
	})

	if *checkJSON {
		*checkMode = true
	}

	if path := strings.TrimSpace(*configPath); path != "" {
		refs := &freebsdFlagRefs{
			SplitMode:                  splitMode,
			SplitChunk:                 splitChunk,
			CollectTimeout:             collectTimeout,
			MaxBuffer:                  maxBuffer,
			MaxHeld:                    maxHeld,
			MaxSegPayload:              maxSegPayload,
			Workers:                    workers,
			FlowTimeout:                flowTimeout,
			GCInterval:                 gcInterval,
			MaxFlows:                   maxFlows,
			MaxReassembly:              maxReassembly,
			MaxHeldBytes:               maxHeldBytes,
			ShutdownFailOpenTimeout:    shutdownFailOpenTimeout,
			ShutdownFailOpenMaxPackets: shutdownFailOpenMaxPkts,
			AdapterFlushTimeout:        adapterFlushTimeout,
			Policies:                   &policies,
			DivertPort:                 divertPort,
		}
		if err := applyFreeBSDJSONConfig(path, setFlags, refs); err != nil {
			log.Fatalf("apply config failed: %v", err)
		}
	}

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
	cfg.Policies = policies
	if err := engine.ValidateConfig(cfg); err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *checkMode {
		if err := runFreeBSDPreflight(*checkJSON); err != nil {
			log.Fatal(err)
		}
		return
	}

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

type freebsdJSONConfig struct {
	Engine  *freebsdEngineJSONConfig  `json:"engine,omitempty"`
	FreeBSD *freebsdRuntimeJSONConfig `json:"freebsd,omitempty"`
}

type freebsdEngineJSONConfig struct {
	SplitMode                   *string                  `json:"split_mode,omitempty"`
	SplitChunk                  *int                     `json:"split_chunk,omitempty"`
	CollectTimeout              *string                  `json:"collect_timeout,omitempty"`
	MaxBufferBytes              *int                     `json:"max_buffer_bytes,omitempty"`
	MaxHeldPackets              *int                     `json:"max_held_packets,omitempty"`
	MaxSegmentPayload           *int                     `json:"max_segment_payload,omitempty"`
	Workers                     *int                     `json:"workers,omitempty"`
	FlowIdleTimeout             *string                  `json:"flow_idle_timeout,omitempty"`
	GCInterval                  *string                  `json:"gc_interval,omitempty"`
	MaxFlowsPerWorker           *int                     `json:"max_flows_per_worker,omitempty"`
	MaxReassemblyBytesPerWorker *int                     `json:"max_reassembly_bytes_per_worker,omitempty"`
	MaxHeldBytesPerWorker       *int                     `json:"max_held_bytes_per_worker,omitempty"`
	ShutdownFailOpenTimeout     *string                  `json:"shutdown_fail_open_timeout,omitempty"`
	ShutdownFailOpenMaxPackets  *int                     `json:"shutdown_fail_open_max_packets,omitempty"`
	AdapterFlushTimeout         *string                  `json:"adapter_flush_timeout,omitempty"`
	Policies                    []enginePolicyJSONConfig `json:"policies,omitempty"`
}

type freebsdRuntimeJSONConfig struct {
	DivertPort *int `json:"divert_port,omitempty"`
}

type freebsdFlagRefs struct {
	SplitMode                  *string
	SplitChunk                 *int
	CollectTimeout             *time.Duration
	MaxBuffer                  *int
	MaxHeld                    *int
	MaxSegPayload              *int
	Workers                    *int
	FlowTimeout                *time.Duration
	GCInterval                 *time.Duration
	MaxFlows                   *int
	MaxReassembly              *int
	MaxHeldBytes               *int
	ShutdownFailOpenTimeout    *time.Duration
	ShutdownFailOpenMaxPackets *int
	AdapterFlushTimeout        *time.Duration
	Policies                   *[]engine.Policy
	DivertPort                 *int
}

func applyFreeBSDJSONConfig(path string, setFlags map[string]bool, refs *freebsdFlagRefs) error {
	if refs == nil {
		return errors.New("nil refs")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var cfg freebsdJSONConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		return err
	}

	if cfg.Engine != nil {
		if cfg.Engine.SplitMode != nil && !setFlags["split-mode"] {
			v := strings.TrimSpace(*cfg.Engine.SplitMode)
			if v != "" {
				*refs.SplitMode = v
			}
		}
		if cfg.Engine.SplitChunk != nil && !setFlags["split-chunk"] {
			*refs.SplitChunk = *cfg.Engine.SplitChunk
		}
		if cfg.Engine.CollectTimeout != nil && !setFlags["collect-timeout"] {
			v := strings.TrimSpace(*cfg.Engine.CollectTimeout)
			if v != "" {
				d, err := time.ParseDuration(v)
				if err != nil {
					return fmt.Errorf("engine.collect_timeout: %w", err)
				}
				*refs.CollectTimeout = d
			}
		}
		if cfg.Engine.MaxBufferBytes != nil && !setFlags["max-buffer"] {
			*refs.MaxBuffer = *cfg.Engine.MaxBufferBytes
		}
		if cfg.Engine.MaxHeldPackets != nil && !setFlags["max-held-pkts"] {
			*refs.MaxHeld = *cfg.Engine.MaxHeldPackets
		}
		if cfg.Engine.MaxSegmentPayload != nil && !setFlags["max-seg-payload"] {
			*refs.MaxSegPayload = *cfg.Engine.MaxSegmentPayload
		}
		if cfg.Engine.Workers != nil && !setFlags["workers"] {
			*refs.Workers = *cfg.Engine.Workers
		}
		if cfg.Engine.FlowIdleTimeout != nil && !setFlags["flow-timeout"] {
			v := strings.TrimSpace(*cfg.Engine.FlowIdleTimeout)
			if v != "" {
				d, err := time.ParseDuration(v)
				if err != nil {
					return fmt.Errorf("engine.flow_idle_timeout: %w", err)
				}
				*refs.FlowTimeout = d
			}
		}
		if cfg.Engine.GCInterval != nil && !setFlags["gc-interval"] {
			v := strings.TrimSpace(*cfg.Engine.GCInterval)
			if v != "" {
				d, err := time.ParseDuration(v)
				if err != nil {
					return fmt.Errorf("engine.gc_interval: %w", err)
				}
				*refs.GCInterval = d
			}
		}
		if cfg.Engine.MaxFlowsPerWorker != nil && !setFlags["max-flows-per-worker"] {
			*refs.MaxFlows = *cfg.Engine.MaxFlowsPerWorker
		}
		if cfg.Engine.MaxReassemblyBytesPerWorker != nil && !setFlags["max-reassembly-bytes-per-worker"] {
			*refs.MaxReassembly = *cfg.Engine.MaxReassemblyBytesPerWorker
		}
		if cfg.Engine.MaxHeldBytesPerWorker != nil && !setFlags["max-held-bytes-per-worker"] {
			*refs.MaxHeldBytes = *cfg.Engine.MaxHeldBytesPerWorker
		}
		if cfg.Engine.ShutdownFailOpenTimeout != nil && !setFlags["shutdown-fail-open-timeout"] {
			v := strings.TrimSpace(*cfg.Engine.ShutdownFailOpenTimeout)
			if v != "" {
				d, err := time.ParseDuration(v)
				if err != nil {
					return fmt.Errorf("engine.shutdown_fail_open_timeout: %w", err)
				}
				*refs.ShutdownFailOpenTimeout = d
			}
		}
		if cfg.Engine.ShutdownFailOpenMaxPackets != nil && !setFlags["shutdown-fail-open-max-pkts"] {
			*refs.ShutdownFailOpenMaxPackets = *cfg.Engine.ShutdownFailOpenMaxPackets
		}
		if cfg.Engine.AdapterFlushTimeout != nil && !setFlags["adapter-flush-timeout"] {
			v := strings.TrimSpace(*cfg.Engine.AdapterFlushTimeout)
			if v != "" {
				d, err := time.ParseDuration(v)
				if err != nil {
					return fmt.Errorf("engine.adapter_flush_timeout: %w", err)
				}
				*refs.AdapterFlushTimeout = d
			}
		}
		if cfg.Engine.Policies != nil && refs.Policies != nil {
			policies, err := parseEnginePolicies(cfg.Engine.Policies)
			if err != nil {
				return err
			}
			*refs.Policies = policies
		}
	}

	if cfg.FreeBSD != nil {
		if cfg.FreeBSD.DivertPort != nil && !setFlags["divert-port"] {
			*refs.DivertPort = *cfg.FreeBSD.DivertPort
		}
	}

	return nil
}

type freebsdPreflightCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

type freebsdPreflightReport struct {
	OK     bool                    `json:"ok"`
	Checks []freebsdPreflightCheck `json:"checks"`
}

func runFreeBSDPreflight(jsonOut bool) error {
	checks := []freebsdPreflightCheck{
		{
			Name:   "root",
			OK:     os.Geteuid() == 0,
			Detail: "required to open pf divert socket",
		},
	}

	report := freebsdPreflightReport{
		OK:     true,
		Checks: checks,
	}
	for i := range report.Checks {
		if !report.Checks[i].OK {
			report.OK = false
			break
		}
	}

	if jsonOut {
		b, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, string(b))
	} else {
		for _, c := range report.Checks {
			state := "OK"
			if !c.OK {
				state = "FAIL"
			}
			fmt.Printf("[%s] %s: %s\n", state, c.Name, c.Detail)
		}
	}

	if !report.OK {
		return errors.New("preflight failed")
	}
	return nil
}
