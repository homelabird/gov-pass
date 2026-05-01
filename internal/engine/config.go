package engine

import (
	"fmt"
	"net/netip"
	"runtime"
	"strings"
	"time"

	"fk-gov/internal/packet"
	itls "fk-gov/internal/tls"
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

	Policies []Policy
}

const maxUint32Int64 = int64(^uint32(0))

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

type Policy struct {
	Name                 string
	DstPrefixes          []netip.Prefix
	SNISuffixes          []string
	Skip                 bool
	HasSplitMode         bool
	SplitMode            SplitMode
	HasSplitChunk        bool
	SplitChunk           int
	HasMaxSegmentPayload bool
	MaxSegmentPayload    int
}

type flowPlan struct {
	Skip              bool
	SplitMode         SplitMode
	SplitChunk        int
	MaxSegmentPayload int
}

type planMatchStatus uint8

const (
	planMatchNo planMatchStatus = iota
	planMatchPending
	planMatchYes
)

func (cfg Config) resolvePlan(meta packet.Meta, hello *itls.ClientHelloInfo) (flowPlan, bool) {
	plan := flowPlan{
		SplitMode:         cfg.SplitMode,
		SplitChunk:        cfg.SplitChunk,
		MaxSegmentPayload: cfg.MaxSegmentPayload,
	}
	for _, policy := range cfg.Policies {
		switch policy.match(meta, hello) {
		case planMatchPending:
			return flowPlan{}, false
		case planMatchYes:
			if policy.Skip {
				plan.Skip = true
			}
			if policy.HasSplitMode {
				plan.SplitMode = policy.SplitMode
			}
			if policy.HasSplitChunk {
				plan.SplitChunk = policy.SplitChunk
			}
			if policy.HasMaxSegmentPayload {
				plan.MaxSegmentPayload = policy.MaxSegmentPayload
			}
			return plan, true
		}
	}
	return plan, true
}

func (p Policy) match(meta packet.Meta, hello *itls.ClientHelloInfo) planMatchStatus {
	if len(p.DstPrefixes) > 0 {
		addr := meta.DstAddr()
		matched := false
		for _, prefix := range p.DstPrefixes {
			if prefix.Contains(addr) {
				matched = true
				break
			}
		}
		if !matched {
			return planMatchNo
		}
	}
	if len(p.SNISuffixes) == 0 {
		return planMatchYes
	}
	if hello == nil {
		return planMatchPending
	}
	serverName := normalizeHostname(hello.ServerName)
	if serverName == "" {
		return planMatchNo
	}
	for _, suffix := range p.SNISuffixes {
		suffix = normalizeHostname(suffix)
		if serverName == suffix || strings.HasSuffix(serverName, "."+suffix) {
			return planMatchYes
		}
	}
	return planMatchNo
}

func ValidatePolicy(policy Policy) error {
	if len(policy.DstPrefixes) == 0 && len(policy.SNISuffixes) == 0 {
		return fmt.Errorf("policy must define dst_cidrs or sni_suffixes")
	}
	if !policy.Skip && !policy.HasSplitMode && !policy.HasSplitChunk && !policy.HasMaxSegmentPayload {
		return fmt.Errorf("policy must change at least one setting or set skip=true")
	}
	if policy.Skip && (policy.HasSplitMode || policy.HasSplitChunk || policy.HasMaxSegmentPayload) {
		return fmt.Errorf("skip=true cannot be combined with split overrides")
	}
	if policy.HasSplitMode && !isValidSplitMode(policy.SplitMode) {
		return fmt.Errorf("split_mode must be tls-hello or immediate")
	}
	if len(policy.SNISuffixes) > 0 && policy.HasSplitMode && policy.SplitMode != SplitModeTLSHello {
		return fmt.Errorf("sni_suffixes policies only support split_mode=tls-hello")
	}
	for _, suffix := range policy.SNISuffixes {
		if err := validateSNISuffix(suffix); err != nil {
			return fmt.Errorf("sni_suffixes: %w", err)
		}
	}
	if policy.HasSplitChunk && policy.SplitChunk < 1 {
		return fmt.Errorf("split_chunk must be >= 1")
	}
	if policy.HasMaxSegmentPayload && policy.MaxSegmentPayload < 0 {
		return fmt.Errorf("max_segment_payload must be >= 0")
	}
	return nil
}

func normalizeHostname(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.TrimPrefix(value, ".")
	value = strings.TrimSuffix(value, ".")
	return value
}

func validateSNISuffix(value string) error {
	value = normalizeHostname(value)
	if value == "" {
		return fmt.Errorf("empty value")
	}
	if len(value) > 253 {
		return fmt.Errorf("%q exceeds 253 characters", value)
	}
	if _, err := netip.ParseAddr(value); err == nil {
		return fmt.Errorf("%q must be a hostname suffix, not an IP address", value)
	}
	labels := strings.Split(value, ".")
	for _, label := range labels {
		if label == "" {
			return fmt.Errorf("%q contains an empty label", value)
		}
		if len(label) > 63 {
			return fmt.Errorf("%q contains a label longer than 63 characters", value)
		}
		if strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return fmt.Errorf("%q contains a label with leading or trailing hyphen", value)
		}
		for _, r := range label {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
				continue
			}
			return fmt.Errorf("%q contains unsupported hostname character %q", value, r)
		}
	}
	return nil
}

func ValidatePolicies(policies []Policy) error {
	for i, policy := range policies {
		if err := ValidatePolicy(policy); err != nil {
			return fmt.Errorf("policies[%d]: %w", i, err)
		}
	}
	return nil
}

func ValidateConfig(cfg Config) error {
	if !isValidSplitMode(cfg.SplitMode) {
		return fmt.Errorf("split-mode must be tls-hello or immediate")
	}
	if cfg.SplitChunk < 1 {
		return fmt.Errorf("split-chunk must be >= 1")
	}
	if cfg.CollectTimeout < time.Millisecond {
		return fmt.Errorf("collect-timeout must be >= 1ms")
	}
	if cfg.MaxBufferBytes < 1 {
		return fmt.Errorf("max-buffer must be >= 1")
	}
	if int64(cfg.MaxBufferBytes) > maxUint32Int64 {
		return fmt.Errorf("max-buffer must be <= %d", maxUint32Int64)
	}
	if cfg.MaxHeldPackets < 1 {
		return fmt.Errorf("max-held-pkts must be >= 1")
	}
	if cfg.MaxSegmentPayload < 0 {
		return fmt.Errorf("max-seg-payload must be >= 0")
	}
	if cfg.WorkerCount < 1 {
		return fmt.Errorf("workers must be >= 1")
	}
	if cfg.WorkerQueueSize < 1 {
		return fmt.Errorf("worker-queue-size must be >= 1")
	}
	if cfg.FlowIdleTimeout < time.Millisecond {
		return fmt.Errorf("flow-timeout must be >= 1ms")
	}
	if cfg.GCInterval < time.Millisecond {
		return fmt.Errorf("gc-interval must be >= 1ms")
	}
	if cfg.MaxFlowsPerWorker < 0 {
		return fmt.Errorf("max-flows-per-worker must be >= 0")
	}
	if cfg.MaxReassemblyBytesPerWorker < 0 {
		return fmt.Errorf("max-reassembly-bytes-per-worker must be >= 0")
	}
	if cfg.MaxHeldBytesPerWorker < 0 {
		return fmt.Errorf("max-held-bytes-per-worker must be >= 0")
	}
	if cfg.ShutdownFailOpenTimeout < 0 {
		return fmt.Errorf("shutdown-fail-open-timeout must be >= 0")
	}
	if cfg.ShutdownFailOpenMaxPackets < 0 {
		return fmt.Errorf("shutdown-fail-open-max-pkts must be >= 0")
	}
	if cfg.AdapterFlushTimeout < 0 {
		return fmt.Errorf("adapter-flush-timeout must be >= 0")
	}
	if err := ValidatePolicies(cfg.Policies); err != nil {
		return err
	}
	for i, policy := range cfg.Policies {
		if len(policy.SNISuffixes) == 0 {
			continue
		}
		mode := cfg.SplitMode
		if policy.HasSplitMode {
			mode = policy.SplitMode
		}
		if mode != SplitModeTLSHello {
			return fmt.Errorf("policies[%d]: sni_suffixes policies require effective split_mode=tls-hello", i)
		}
	}
	return nil
}

func isValidSplitMode(mode SplitMode) bool {
	switch mode {
	case SplitModeImmediate, SplitModeTLSHello:
		return true
	default:
		return false
	}
}

func cloneConfig(cfg Config) Config {
	cfgCopy := cfg
	if len(cfg.Policies) > 0 {
		cfgCopy.Policies = make([]Policy, len(cfg.Policies))
		copy(cfgCopy.Policies, cfg.Policies)
	}
	return cfgCopy
}
