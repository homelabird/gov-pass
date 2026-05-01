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
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"fk-gov/internal/adapter"
	"fk-gov/internal/engine"
)

const (
	defaultFreeBSDPFAnchorName   = "gov-pass"
	defaultFreeBSDPFConfPath     = "/etc/pf.conf"
	defaultFreeBSDPFAnchorSource = "/usr/local/etc/gov-pass/pf.anchor.conf"
	defaultFreeBSDPFAnchorPath   = "/etc/pf.anchors/gov-pass"
)

var (
	freebsdLookPath  = resolveTrustedFreeBSDSplitterCommand
	freebsdStat      = os.Stat
	freebsdCmdOutput = func(name string, args ...string) ([]byte, error) {
		if !filepath.IsAbs(name) {
			return nil, fmt.Errorf("command path must be absolute: %s", name)
		}
		cmd := exec.Command(name, args...)
		cmd.Env = sanitizedFreeBSDSplitterCommandEnv()
		return cmd.CombinedOutput()
	}
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
	statsInterval := flag.Duration("stats-interval", defaultStatsInterval, "periodic engine stats log interval (0=disabled)")
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
			StatsInterval:              statsInterval,
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
	if *statsInterval < 0 {
		log.Fatal("stats-interval must be >= 0")
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
	eng, err := engine.NewChecked(cfg, ad)
	if err != nil {
		log.Fatalf("invalid engine config: %v", err)
	}

	logInfo("engine_started", "engine started",
		"workers", cfg.WorkerCount,
		"split_mode", cfg.SplitMode,
		"split_chunk", cfg.SplitChunk,
		"stats_interval", *statsInterval,
		"divert_port", opts.Port,
	)
	stopStats := startEngineStatsLogger(ctx, eng, *statsInterval)
	defer stopStats()
	err = eng.Run(ctx)
	logInfo("engine_stats", "engine stats", "stats", eng.Stats())
	if err != nil && !errors.Is(err, context.Canceled) {
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

type freebsdEngineJSONConfig = engineJSONConfig

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
	StatsInterval              *time.Duration
	Policies                   *[]engine.Policy
	DivertPort                 *int
}

func freebsdEngineFlagRefs(refs *freebsdFlagRefs) engineFlagRefs {
	if refs == nil {
		return engineFlagRefs{}
	}
	return engineFlagRefs{
		SplitMode:                  refs.SplitMode,
		SplitChunk:                 refs.SplitChunk,
		CollectTimeout:             refs.CollectTimeout,
		MaxBuffer:                  refs.MaxBuffer,
		MaxHeld:                    refs.MaxHeld,
		MaxSegPayload:              refs.MaxSegPayload,
		Workers:                    refs.Workers,
		FlowTimeout:                refs.FlowTimeout,
		GCInterval:                 refs.GCInterval,
		MaxFlows:                   refs.MaxFlows,
		MaxReassembly:              refs.MaxReassembly,
		MaxHeldBytes:               refs.MaxHeldBytes,
		ShutdownFailOpenTimeout:    refs.ShutdownFailOpenTimeout,
		ShutdownFailOpenMaxPackets: refs.ShutdownFailOpenMaxPackets,
		AdapterFlushTimeout:        refs.AdapterFlushTimeout,
		StatsInterval:              refs.StatsInterval,
		Policies:                   refs.Policies,
	}
}

func applyFreeBSDJSONConfig(path string, setFlags map[string]bool, refs *freebsdFlagRefs) error {
	if refs == nil {
		return errors.New("nil refs")
	}
	b, err := readJSONConfigFile(path)
	if err != nil {
		return err
	}

	var cfg freebsdJSONConfig
	if err := decodeStrictJSONConfig(b, &cfg); err != nil {
		return err
	}

	if err := applyEngineJSONConfigToFlags(cfg.Engine, setFlags, freebsdEngineFlagRefs(refs)); err != nil {
		return err
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
	Notes  []string                `json:"notes,omitempty"`
}

func runFreeBSDPreflight(jsonOut bool) error {
	checks := make([]freebsdPreflightCheck, 0, 9)
	add := func(name string, ok bool, detail string) {
		checks = append(checks, freebsdPreflightCheck{Name: name, OK: ok, Detail: detail})
	}

	add("root", os.Geteuid() == 0, "required to open pf divert socket")
	cmdPaths := make(map[string]string)
	for _, cmd := range []string{"pfctl", "service", "sysrc"} {
		path, err := freebsdLookPath(cmd)
		if err != nil {
			add(cmd, false, "required for the documented FreeBSD operator workflow")
			continue
		}
		cmdPaths[cmd] = path
		add(cmd, true, path)
	}

	addFreeBSDFileCheck(checksAppendFunc(add), "pf_conf", defaultFreeBSDPFConfPath)
	addFreeBSDFileCheck(checksAppendFunc(add), "pf_anchor_source", defaultFreeBSDPFAnchorSource)
	addFreeBSDFileCheck(checksAppendFunc(add), "pf_anchor_installed", defaultFreeBSDPFAnchorPath)

	if pfctlPath := cmdPaths["pfctl"]; pfctlPath != "" {
		out, err := freebsdCmdOutput(pfctlPath, "-s", "info")
		if err != nil {
			add("pf_status", false, freeBSDCommandFailureDetail(out, err))
		} else if freeBSDPFStatusEnabled(string(out)) {
			add("pf_status", true, "pf is enabled")
		} else {
			add("pf_status", false, "pf is disabled or status could not be parsed")
		}

		out, err = freebsdCmdOutput(pfctlPath, "-a", defaultFreeBSDPFAnchorName, "-s", "rules")
		if err != nil {
			add("pf_anchor_rules", false, freeBSDCommandFailureDetail(out, err))
		} else if freeBSDPFAnchorRulesLoaded(string(out)) {
			add("pf_anchor_rules", true, "live anchor has rules")
		} else {
			add("pf_anchor_rules", false, "live anchor has no rules; run install_pf_anchor.sh")
		}
	} else {
		add("pf_status", false, "pfctl unavailable")
		add("pf_anchor_rules", false, "pfctl unavailable")
	}

	notes := []string{
		"reload is not supported on FreeBSD; use restart",
		"pf policy remains operator-managed; install and apply the gov-pass anchor before starting splitter",
		"pf_status and pf_anchor_rules inspect current pf state but do not prove interface selectors match the deployment",
		"the current divert socket path is IPv4-focused; treat IPv6 divert handling as unsupported",
	}

	report := freebsdPreflightReport{
		OK:     true,
		Checks: checks,
		Notes:  notes,
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
		for _, note := range report.Notes {
			fmt.Printf("[INFO] %s\n", note)
		}
	}

	if !report.OK {
		return errors.New("preflight failed")
	}
	return nil
}

type checksAppendFunc func(name string, ok bool, detail string)

func addFreeBSDFileCheck(add checksAppendFunc, name string, path string) {
	if _, err := freebsdStat(path); err == nil {
		add(name, true, path)
	} else {
		add(name, false, fmt.Sprintf("%s: %v", path, err))
	}
}

func freeBSDCommandFailureDetail(out []byte, err error) string {
	text := strings.TrimSpace(string(out))
	if text == "" {
		return err.Error()
	}
	return fmt.Sprintf("%s: %v", text, err)
}
