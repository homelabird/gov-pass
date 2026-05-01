//go:build linux

package main

import (
	"bufio"
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
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"fk-gov/internal/adapter"
	"fk-gov/internal/engine"
)

const maxUint32Value = int64(^uint32(0))

var trustedLinuxCommandDirs = []string{
	"/usr/local/sbin",
	"/usr/local/bin",
	"/usr/sbin",
	"/usr/bin",
	"/sbin",
	"/bin",
	"/run/current-system/sw/bin",
	"/nix/var/nix/profiles/default/bin",
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg := engine.DefaultConfig()
	policies := cfg.Policies
	const (
		defaultQueueNum    = 100
		defaultQueueMaxLen = 4096
		defaultCopyRange   = 0xffff
		defaultRecvBuffer  = 0
		defaultMark        = 1
	)

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
	queueNum := flag.Int("queue-num", defaultQueueNum, "NFQUEUE number")
	queueMaxLen := flag.Int("queue-maxlen", defaultQueueMaxLen, "NFQUEUE maxlen (0=kernel default)")
	copyRange := flag.Int("copy-range", defaultCopyRange, "NFQUEUE copy range in bytes (0=full packet)")
	recvBuffer := flag.Int("recv-buffer", defaultRecvBuffer, "internal NFQUEUE recv buffer capacity (0=derive from queue-maxlen)")
	mark := flag.Int("mark", defaultMark, "SO_MARK for reinjected packets")
	autoRules := flag.Bool("auto-rules", true, "auto install/uninstall NFQUEUE rules (nft or iptables)")
	autoOffload := flag.Bool("auto-offload", true, "auto disable GRO/GSO/TSO (ethtool)")
	autoOffloadRestore := flag.Bool("auto-offload-restore", true, "restore GRO/GSO/TSO settings on exit when auto-offload is enabled")
	autoInstallTools := flag.Bool("auto-install-tools", true, "auto install missing system tools (nft/iptables/ip/ethtool) when auto helpers are enabled")
	iface := flag.String("iface", "", "egress interface for offload disable (default: auto-detect)")
	noLoopback := flag.Bool("no-loopback", false, "do not exclude loopback from NFQUEUE rules")
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
		refs := &linuxFlagRefs{
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
			QueueNum:                   queueNum,
			QueueMaxLen:                queueMaxLen,
			CopyRange:                  copyRange,
			RecvBuffer:                 recvBuffer,
			Mark:                       mark,
			AutoRules:                  autoRules,
			AutoOffload:                autoOffload,
			AutoOffloadRestore:         autoOffloadRestore,
			AutoInstallTools:           autoInstallTools,
			Iface:                      iface,
			NoLoopback:                 noLoopback,
		}
		if err := applyLinuxJSONConfig(path, setFlags, refs); err != nil {
			return fmt.Errorf("apply config failed: %w", err)
		}
	}

	mode, err := parseSplitMode(*splitMode)
	if err != nil {
		return fmt.Errorf("invalid split-mode: %w", err)
	}
	if *splitChunk < 1 {
		return errors.New("split-chunk must be >= 1")
	}
	if *maxBuffer < 1 {
		return errors.New("max-buffer must be >= 1")
	}
	if *maxHeld < 1 {
		return errors.New("max-held-pkts must be >= 1")
	}
	if *maxSegPayload < 0 {
		return errors.New("max-seg-payload must be >= 0")
	}
	if *workers < 1 {
		return errors.New("workers must be >= 1")
	}
	if *collectTimeout < 1*time.Millisecond {
		return errors.New("collect-timeout must be >= 1ms")
	}
	if *flowTimeout < 1*time.Millisecond {
		return errors.New("flow-timeout must be >= 1ms")
	}
	if *gcInterval < 1*time.Millisecond {
		return errors.New("gc-interval must be >= 1ms")
	}
	if *maxFlows < 0 {
		return errors.New("max-flows-per-worker must be >= 0")
	}
	if *maxReassembly < 0 {
		return errors.New("max-reassembly-bytes-per-worker must be >= 0")
	}
	if *maxHeldBytes < 0 {
		return errors.New("max-held-bytes-per-worker must be >= 0")
	}
	if *shutdownFailOpenTimeout < 0 {
		return errors.New("shutdown-fail-open-timeout must be >= 0")
	}
	if *shutdownFailOpenMaxPkts < 0 {
		return errors.New("shutdown-fail-open-max-pkts must be >= 0")
	}
	if *adapterFlushTimeout < 0 {
		return errors.New("adapter-flush-timeout must be >= 0")
	}
	if *statsInterval < 0 {
		return errors.New("stats-interval must be >= 0")
	}
	if *queueNum < 0 || *queueNum > 65535 {
		return errors.New("queue-num must be in 0..65535")
	}
	if *queueMaxLen < 0 {
		return errors.New("queue-maxlen must be >= 0")
	}
	if int64(*queueMaxLen) > maxUint32Value {
		return errors.New("queue-maxlen must be <= 4294967295")
	}
	if *copyRange < 0 {
		return errors.New("copy-range must be >= 0")
	}
	if int64(*copyRange) > maxUint32Value {
		return errors.New("copy-range must be <= 4294967295")
	}
	if *recvBuffer < 0 {
		return errors.New("recv-buffer must be >= 0")
	}
	if *recvBuffer > adapter.MaxNFQueueRecvBuffer {
		return fmt.Errorf("recv-buffer must be <= %d", adapter.MaxNFQueueRecvBuffer)
	}
	if *mark < 0 {
		return errors.New("mark must be >= 0")
	}
	if int64(*mark) > maxUint32Value {
		return errors.New("mark must be <= 4294967295")
	}
	if *autoRules && *mark == 0 {
		return errors.New("auto-rules requires mark > 0 for reinjection bypass; set --mark or disable --auto-rules")
	}
	if *autoOffload && strings.TrimSpace(*iface) != "" {
		normalizedIface, err := normalizeLinuxIfaceName(*iface)
		if err != nil {
			return err
		}
		*iface = normalizedIface
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
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	toolNeeds := linuxToolNeeds{
		AutoRules:   *autoRules,
		AutoOffload: *autoOffload,
		NeedIP:      *autoOffload && strings.TrimSpace(*iface) == "",
	}
	if *checkMode {
		return runLinuxPreflight(*autoInstallTools, toolNeeds, *autoRules || *autoOffload, *checkJSON)
	}

	if *autoRules || *autoOffload {
		if os.Geteuid() != 0 {
			return errors.New("auto-rules/auto-offload require root; run as root or set --auto-rules=false --auto-offload=false")
		}
	}

	if err := ensureLinuxExternalTools(*autoInstallTools, toolNeeds); err != nil {
		return err
	}

	var rulesCleanup func() error
	if *autoRules {
		opts := ruleOptions{
			QueueNum:        uint16(*queueNum),
			Mark:            uint32(*mark),
			ExcludeLoopback: !*noLoopback,
		}
		cleanup, backend, err := installRules(opts)
		if err != nil {
			return fmt.Errorf("auto rule install failed: %w", err)
		}
		rulesCleanup = cleanup
		logInfo("linux_rules_installed", "auto rules installed", "backend", backend)
		defer func() {
			if rulesCleanup == nil {
				return
			}
			if err := rulesCleanup(); err != nil {
				logError("linux_rules_cleanup_failed", "auto rule uninstall failed", err)
			}
		}()
	}

	if *autoOffload {
		ifaceName := strings.TrimSpace(*iface)
		if ifaceName == "" {
			detected, err := detectEgressInterface()
			if err != nil {
				if rulesCleanup != nil {
					_ = rulesCleanup()
				}
				return fmt.Errorf("auto offload failed: %w", err)
			}
			ifaceName = detected
		}

		var restore *offloadState
		if *autoOffloadRestore {
			st, err := readOffloadState(ifaceName)
			if err != nil {
				logWarn("linux_offload_state_unavailable", "warning: could not read offload state; restore disabled", "iface", ifaceName, "err", err)
			} else {
				restore = &st
			}
		}

		if restore != nil {
			st := *restore
			defer func() {
				if err := applyOffloadState(ifaceName, st); err != nil {
					logError("linux_offload_restore_failed", "offload restore failed", err, "iface", ifaceName)
				} else {
					logInfo("linux_offload_restored", "offload restored", "iface", ifaceName, "gro", st.gro, "gso", st.gso, "tso", st.tso)
				}
			}()
		}

		if err := disableOffload(ifaceName); err != nil {
			if rulesCleanup != nil {
				_ = rulesCleanup()
			}
			return fmt.Errorf("disable offload failed: %w", err)
		}
		logInfo("linux_offload_disabled", "offload disabled", "iface", ifaceName, "gro", false, "gso", false, "tso", false)
	}

	if *mark == 0 {
		logWarn("linux_mark_zero", "warning: mark=0; ensure NFQUEUE bypass rules prevent reinjection loops")
	}

	opts := adapter.NFQueueOptions{
		QueueNum:       uint16(*queueNum),
		QueueMaxLen:    uint32(*queueMaxLen),
		CopyRange:      uint32(*copyRange),
		RecvBufferSize: uint32(*recvBuffer),
		Mark:           uint32(*mark),
	}
	ad, err := adapter.NewNFQueue(opts)
	if err != nil {
		return fmt.Errorf("NFQUEUE open failed: %w", err)
	}
	eng, err := engine.NewChecked(cfg, ad)
	if err != nil {
		return fmt.Errorf("invalid engine config: %w", err)
	}

	logInfo("engine_started", "engine started",
		"workers", cfg.WorkerCount,
		"split_mode", cfg.SplitMode,
		"split_chunk", cfg.SplitChunk,
		"stats_interval", *statsInterval,
		"queue_num", opts.QueueNum,
		"queue_maxlen", opts.QueueMaxLen,
		"copy_range", opts.CopyRange,
		"recv_buffer", opts.RecvBufferSize,
		"effective_recv_buffer", adapter.NFQueueRecvBufferCapacity(opts),
		"mark", opts.Mark,
	)
	stopStats := startEngineStatsLogger(ctx, eng, *statsInterval)
	defer stopStats()
	err = eng.Run(ctx)
	logInfo("engine_stats", "engine stats", "stats", eng.Stats())
	if err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("engine stopped: %w", err)
	}
	return nil
}

type linuxJSONConfig struct {
	Engine *linuxEngineJSONConfig  `json:"engine,omitempty"`
	Linux  *linuxRuntimeJSONConfig `json:"linux,omitempty"`
}

type linuxEngineJSONConfig = engineJSONConfig

type linuxRuntimeJSONConfig struct {
	QueueNum        *int    `json:"queue_num,omitempty"`
	QueueMaxLen     *int    `json:"queue_maxlen,omitempty"`
	CopyRange       *int    `json:"copy_range,omitempty"`
	RecvBuffer      *int    `json:"recv_buffer,omitempty"`
	Mark            *int    `json:"mark,omitempty"`
	AutoRules       *bool   `json:"auto_rules,omitempty"`
	AutoOffload     *bool   `json:"auto_offload,omitempty"`
	AutoOffloadRest *bool   `json:"auto_offload_restore,omitempty"`
	AutoInstallTool *bool   `json:"auto_install_tools,omitempty"`
	Iface           *string `json:"iface,omitempty"`
	NoLoopback      *bool   `json:"no_loopback,omitempty"`
}

type linuxFlagRefs struct {
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

	QueueNum           *int
	QueueMaxLen        *int
	CopyRange          *int
	RecvBuffer         *int
	Mark               *int
	AutoRules          *bool
	AutoOffload        *bool
	AutoOffloadRestore *bool
	AutoInstallTools   *bool
	Iface              *string
	NoLoopback         *bool
}

func linuxEngineFlagRefs(refs *linuxFlagRefs) engineFlagRefs {
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

func applyLinuxJSONConfig(path string, setFlags map[string]bool, refs *linuxFlagRefs) error {
	if refs == nil {
		return errors.New("nil refs")
	}

	b, err := readLinuxConfigFile(path)
	if err != nil {
		return err
	}

	var cfg linuxJSONConfig
	if err := decodeStrictJSONConfig(b, &cfg); err != nil {
		return err
	}

	if err := applyEngineJSONConfigToFlags(cfg.Engine, setFlags, linuxEngineFlagRefs(refs)); err != nil {
		return err
	}

	if cfg.Linux != nil {
		if cfg.Linux.QueueNum != nil && !setFlags["queue-num"] {
			*refs.QueueNum = *cfg.Linux.QueueNum
		}
		if cfg.Linux.QueueMaxLen != nil && !setFlags["queue-maxlen"] {
			*refs.QueueMaxLen = *cfg.Linux.QueueMaxLen
		}
		if cfg.Linux.CopyRange != nil && !setFlags["copy-range"] {
			*refs.CopyRange = *cfg.Linux.CopyRange
		}
		if cfg.Linux.RecvBuffer != nil && !setFlags["recv-buffer"] {
			*refs.RecvBuffer = *cfg.Linux.RecvBuffer
		}
		if cfg.Linux.Mark != nil && !setFlags["mark"] {
			*refs.Mark = *cfg.Linux.Mark
		}
		if cfg.Linux.AutoRules != nil && !setFlags["auto-rules"] {
			*refs.AutoRules = *cfg.Linux.AutoRules
		}
		if cfg.Linux.AutoOffload != nil && !setFlags["auto-offload"] {
			*refs.AutoOffload = *cfg.Linux.AutoOffload
		}
		if cfg.Linux.AutoOffloadRest != nil && !setFlags["auto-offload-restore"] {
			*refs.AutoOffloadRestore = *cfg.Linux.AutoOffloadRest
		}
		if cfg.Linux.AutoInstallTool != nil && !setFlags["auto-install-tools"] {
			*refs.AutoInstallTools = *cfg.Linux.AutoInstallTool
		}
		if cfg.Linux.Iface != nil && !setFlags["iface"] {
			*refs.Iface = strings.TrimSpace(*cfg.Linux.Iface)
		}
		if cfg.Linux.NoLoopback != nil && !setFlags["no-loopback"] {
			*refs.NoLoopback = *cfg.Linux.NoLoopback
		}
	}

	return nil
}

type preflightCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

type preflightReport struct {
	OK     bool             `json:"ok"`
	Checks []preflightCheck `json:"checks"`
}

func runLinuxPreflight(autoInstall bool, needs linuxToolNeeds, requireRoot bool, jsonOut bool) error {
	checks := make([]preflightCheck, 0, 6)
	add := func(name string, ok bool, detail string) {
		checks = append(checks, preflightCheck{Name: name, OK: ok, Detail: detail})
	}

	if requireRoot {
		rootOK := os.Geteuid() == 0
		detail := "required when auto-rules/auto-offload is enabled"
		if !rootOK {
			detail = "run as root or disable auto helpers"
		}
		add("root", rootOK, detail)
	}

	if needs.AutoRules {
		_, hasNft := linuxLookPath("nft")
		_, hasIpt := linuxLookPath("iptables")
		_, hasIpt6 := linuxLookPath("ip6tables")
		ok := hasNft || (hasIpt && hasIpt6)
		var detail string
		if hasNft {
			detail = "nft found"
		} else if hasIpt && hasIpt6 {
			detail = "iptables and ip6tables found"
		} else if autoInstall {
			if kind, _, found := linuxDetectPackageManager(); found {
				ok = true
				detail = "missing now; auto-install available via " + kind
			} else {
				detail = "missing now; no supported package manager found"
			}
		} else {
			detail = "missing; install nftables or both iptables and ip6tables"
		}
		add("nft_or_iptables", ok, detail)
	}

	if needs.AutoOffload {
		_, hasEthtool := linuxLookPath("ethtool")
		ok := hasEthtool
		var detail string
		if hasEthtool {
			detail = "ethtool found"
		} else if autoInstall {
			if kind, _, found := linuxDetectPackageManager(); found {
				ok = true
				detail = "missing now; auto-install available via " + kind
			} else {
				detail = "missing now; no supported package manager found"
			}
		} else {
			detail = "missing; install ethtool"
		}
		add("ethtool", ok, detail)

		if needs.NeedIP {
			_, hasIP := linuxLookPath("ip")
			okIP := hasIP
			var detailIP string
			if hasIP {
				detailIP = "ip found"
			} else if autoInstall {
				if kind, _, found := linuxDetectPackageManager(); found {
					okIP = true
					detailIP = "missing now; auto-install available via " + kind
				} else {
					detailIP = "missing now; no supported package manager found"
				}
			} else {
				detailIP = "missing; install iproute2/iproute"
			}
			add("ip", okIP, detailIP)
		}
	}

	report := preflightReport{
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
			fmt.Fprintf(os.Stdout, "[%s] %s: %s\n", state, c.Name, c.Detail)
		}
	}

	if !report.OK {
		return errors.New("preflight failed")
	}
	return nil
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

type ruleOptions struct {
	QueueNum        uint16
	Mark            uint32
	ExcludeLoopback bool
}

func installRules(opts ruleOptions) (func() error, string, error) {
	if path, ok := linuxLookPath("nft"); ok {
		if err := installNftRules(path, opts); err != nil {
			return nil, "", err
		}
		return func() error { return uninstallNftRules(path) }, "nft", nil
	}
	iptablesPath, hasIptables := linuxLookPath("iptables")
	ip6tablesPath, hasIP6Tables := linuxLookPath("ip6tables")
	if hasIptables && hasIP6Tables {
		if err := installIptablesRules(iptablesPath, ip6tablesPath, opts); err != nil {
			return nil, "", err
		}
		return func() error { return uninstallIptablesRules(iptablesPath, ip6tablesPath, opts) }, "iptables/ip6tables", nil
	}
	return nil, "", errors.New("nft or both iptables and ip6tables not found in PATH")
}

func installNftRules(path string, opts ruleOptions) error {
	const (
		table = "gov_pass"
		chain = "output"
		tag   = "gov-pass"
	)

	if _, err := runCommand(path, "list", "table", "inet", table); err != nil {
		if _, err := runCommand(path, "add", "table", "inet", table); err != nil {
			return fmt.Errorf("nft add table failed: %w", err)
		}
	}

	if _, err := runCommand(path, "list", "chain", "inet", table, chain); err != nil {
		args := []string{
			"add", "chain", "inet", table, chain,
			"{", "type", "filter", "hook", "output", "priority", "mangle", ";", "policy", "accept", ";", "}",
		}
		if _, err := runCommand(path, args...); err != nil {
			return fmt.Errorf("nft add chain failed: %w", err)
		}
	}

	if err := deleteTaggedNftRules(path, table, chain, tag); err != nil {
		return fmt.Errorf("nft delete old rules failed: %w", err)
	}

	if opts.Mark != 0 {
		mark := fmt.Sprintf("%d", opts.Mark)
		args := []string{"add", "rule", "inet", table, chain, "meta", "mark", "&", mark, "==", mark, "return", "comment", tag}
		if _, err := runCommand(path, args...); err != nil {
			return fmt.Errorf("nft add mark bypass failed: %w", err)
		}
	}

	if opts.ExcludeLoopback {
		args := []string{"add", "rule", "inet", table, chain, "oifname", "lo", "return", "comment", tag}
		if _, err := runCommand(path, args...); err != nil {
			return fmt.Errorf("nft add loopback bypass failed: %w", err)
		}
	}

	queue := fmt.Sprintf("%d", opts.QueueNum)
	for _, family := range []string{"ipv4", "ipv6"} {
		args := []string{"add", "rule", "inet", table, chain, "meta", "nfproto", family, "tcp", "dport", "443", "queue", "num", queue, "bypass", "comment", tag}
		if _, err := runCommand(path, args...); err != nil {
			return fmt.Errorf("nft add %s queue rule failed: %w", family, err)
		}
	}

	return nil
}

func uninstallNftRules(path string) error {
	const (
		table = "gov_pass"
		chain = "output"
		tag   = "gov-pass"
	)
	if _, err := runCommand(path, "list", "table", "inet", table); err != nil {
		return nil
	}
	if err := deleteTaggedNftRules(path, table, chain, tag); err != nil {
		return fmt.Errorf("nft delete rules failed: %w", err)
	}
	return nil
}

func deleteTaggedNftRules(path string, table string, chain string, tag string) error {
	out, err := runCommand(path, "-a", "list", "chain", "inet", table, chain)
	if err != nil {
		// Best-effort: table/chain may not exist.
		lower := strings.ToLower(err.Error())
		if strings.Contains(lower, "no such file") || strings.Contains(lower, "does not exist") {
			return nil
		}
		return err
	}

	want := fmt.Sprintf("comment \"%s\"", tag)
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, want) {
			continue
		}
		handle, ok := parseNftHandle(line)
		if !ok {
			continue
		}
		if _, err := runCommand(path, "delete", "rule", "inet", table, chain, "handle", strconv.Itoa(handle)); err != nil {
			lower := strings.ToLower(err.Error())
			if strings.Contains(lower, "no such file") || strings.Contains(lower, "does not exist") {
				continue
			}
			return err
		}
	}
	return scanner.Err()
}

func parseNftHandle(line string) (int, bool) {
	fields := strings.Fields(line)
	for i := 0; i+1 < len(fields); i++ {
		if fields[i] == "handle" {
			v, err := strconv.Atoi(fields[i+1])
			if err != nil {
				return 0, false
			}
			if v <= 0 {
				return 0, false
			}
			return v, true
		}
	}
	return 0, false
}

func installIptablesRules(iptablesPath string, ip6tablesPath string, opts ruleOptions) error {
	if err := installOneIptablesFamily(iptablesPath, "GOVPASS_OUTPUT", opts); err != nil {
		return err
	}
	if err := installOneIptablesFamily(ip6tablesPath, "GOVPASS_OUTPUT6", opts); err != nil {
		_ = uninstallOneIptablesFamily(iptablesPath, "GOVPASS_OUTPUT", opts)
		return err
	}
	return nil
}

func installOneIptablesFamily(path string, chain string, opts ruleOptions) error {
	const (
		table  = "mangle"
		parent = "OUTPUT"
	)

	if err := ensureIptablesChain(path, table, chain); err != nil {
		return fmt.Errorf("iptables create chain failed: %w", err)
	}
	if _, err := runCommand(path, "-t", table, "-F", chain); err != nil {
		return fmt.Errorf("iptables flush chain failed: %w", err)
	}

	// Jump early from OUTPUT to our dedicated chain so we don't pollute OUTPUT
	// with multiple rules and can cleanly uninstall later.
	checkJump := []string{"-t", table, "-C", parent, "-j", chain}
	addJump := []string{"-t", table, "-I", parent, "1", "-j", chain}
	if err := ensureIptablesRule(path, checkJump, addJump); err != nil {
		return fmt.Errorf("iptables install jump failed: %w", err)
	}

	if opts.Mark != 0 {
		mark := fmt.Sprintf("%d/%d", opts.Mark, opts.Mark)
		if _, err := runCommand(path, "-t", table, "-A", chain, "-m", "mark", "--mark", mark, "-j", "RETURN"); err != nil {
			return fmt.Errorf("iptables mark bypass failed: %w", err)
		}
	}

	if opts.ExcludeLoopback {
		if _, err := runCommand(path, "-t", table, "-A", chain, "-o", "lo", "-j", "RETURN"); err != nil {
			return fmt.Errorf("iptables loopback bypass failed: %w", err)
		}
	}

	queue := fmt.Sprintf("%d", opts.QueueNum)
	if _, err := runCommand(path, "-t", table, "-A", chain, "-p", "tcp", "--dport", "443", "-j", "NFQUEUE", "--queue-num", queue, "--queue-bypass"); err != nil {
		return fmt.Errorf("iptables queue rule failed: %w", err)
	}

	return nil
}

func uninstallIptablesRules(iptablesPath string, ip6tablesPath string, opts ruleOptions) error {
	var errs []error
	if err := uninstallOneIptablesFamily(iptablesPath, "GOVPASS_OUTPUT", opts); err != nil {
		errs = append(errs, err)
	}
	if err := uninstallOneIptablesFamily(ip6tablesPath, "GOVPASS_OUTPUT6", opts); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func uninstallOneIptablesFamily(path string, chain string, opts ruleOptions) error {
	const (
		table  = "mangle"
		parent = "OUTPUT"
	)
	_ = opts

	// Remove jump(s) from OUTPUT (best-effort).
	for {
		if _, err := runCommand(path, "-t", table, "-D", parent, "-j", chain); err != nil {
			break
		}
	}

	_, _ = runCommand(path, "-t", table, "-F", chain)
	_, _ = runCommand(path, "-t", table, "-X", chain)
	return nil
}

func ensureIptablesRule(path string, check []string, add []string) error {
	if _, err := runCommand(path, check...); err == nil {
		return nil
	}
	if _, err := runCommand(path, add...); err != nil {
		return err
	}
	return nil
}

func ensureIptablesChain(path string, table string, chain string) error {
	if _, err := runCommand(path, "-t", table, "-N", chain); err != nil {
		// iptables errors vary between legacy/nft variants; treat "exists" as success.
		if strings.Contains(strings.ToLower(err.Error()), "exists") {
			return nil
		}
		return err
	}
	return nil
}

type offloadState struct {
	gro bool
	gso bool
	tso bool
}

func readOffloadState(iface string) (offloadState, error) {
	iface, err := normalizeLinuxIfaceName(iface)
	if err != nil {
		return offloadState{}, err
	}
	path, ok := linuxLookPath("ethtool")
	if !ok {
		return offloadState{}, errors.New("ethtool not found in PATH")
	}
	out, err := runCommand(path, "-k", iface)
	if err != nil {
		return offloadState{}, err
	}

	var st offloadState
	foundGro := false
	foundGso := false
	foundTso := false

	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case strings.HasPrefix(line, "generic-receive-offload:"):
			if v, ok := parseEthtoolOnOff(line); ok {
				st.gro = v
				foundGro = true
			}
		case strings.HasPrefix(line, "generic-segmentation-offload:"):
			if v, ok := parseEthtoolOnOff(line); ok {
				st.gso = v
				foundGso = true
			}
		case strings.HasPrefix(line, "tcp-segmentation-offload:"):
			if v, ok := parseEthtoolOnOff(line); ok {
				st.tso = v
				foundTso = true
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return offloadState{}, err
	}
	if !foundGro || !foundGso || !foundTso {
		return offloadState{}, errors.New("could not parse ethtool offload state")
	}
	return st, nil
}

func parseEthtoolOnOff(line string) (bool, bool) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return false, false
	}
	rest := strings.TrimSpace(line[idx+1:])
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return false, false
	}
	switch strings.ToLower(fields[0]) {
	case "on":
		return true, true
	case "off":
		return false, true
	default:
		return false, false
	}
}

func applyOffloadState(iface string, st offloadState) error {
	iface, err := normalizeLinuxIfaceName(iface)
	if err != nil {
		return err
	}
	path, ok := linuxLookPath("ethtool")
	if !ok {
		return errors.New("ethtool not found in PATH")
	}

	gro := "off"
	if st.gro {
		gro = "on"
	}
	gso := "off"
	if st.gso {
		gso = "on"
	}
	tso := "off"
	if st.tso {
		tso = "on"
	}

	if _, err := runCommand(path, "-K", iface, "gro", gro, "gso", gso, "tso", tso); err != nil {
		return err
	}
	return nil
}

func disableOffload(iface string) error {
	iface, err := normalizeLinuxIfaceName(iface)
	if err != nil {
		return err
	}
	path, ok := linuxLookPath("ethtool")
	if !ok {
		return errors.New("ethtool not found in PATH")
	}
	if _, err := runCommand(path, "-K", iface, "gro", "off", "gso", "off", "tso", "off"); err != nil {
		return err
	}
	return nil
}

func detectEgressInterface() (string, error) {
	path, ok := linuxLookPath("ip")
	if !ok {
		return "", errors.New("ip command not found in PATH; use --iface")
	}
	return detectEgressInterfaceWith(path, runCommand)
}

type linuxCommandRunner func(name string, args ...string) (string, error)

func detectEgressInterfaceWith(path string, runner linuxCommandRunner) (string, error) {
	probes := []struct {
		args []string
	}{
		{args: []string{"-o", "-4", "route", "get", "1.1.1.1"}},
		{args: []string{"-o", "-6", "route", "get", "2606:4700:4700::1111"}},
		{args: []string{"-o", "-4", "route", "show", "default"}},
		{args: []string{"-o", "-6", "route", "show", "default"}},
	}

	var candidates []string
	seen := make(map[string]struct{})
	for _, probe := range probes {
		out, err := runner(path, probe.args...)
		if err != nil {
			continue
		}
		for _, iface := range parseRouteDevs(out) {
			if _, ok := seen[iface]; ok {
				continue
			}
			seen[iface] = struct{}{}
			candidates = append(candidates, iface)
		}
	}

	switch len(candidates) {
	case 0:
		return "", errors.New("could not detect egress interface from IPv4/IPv6 route lookups; use --iface")
	case 1:
		return candidates[0], nil
	default:
		return "", fmt.Errorf("detected multiple candidate egress interfaces (%s); use --iface", strings.Join(candidates, ", "))
	}
}

func parseRouteDev(output string) string {
	devs := parseRouteDevs(output)
	if len(devs) == 0 {
		return ""
	}
	return devs[0]
}

func parseRouteDevs(output string) []string {
	scanner := bufio.NewScanner(strings.NewReader(output))
	seen := make(map[string]struct{})
	var devs []string
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		for i := 0; i+1 < len(fields); i++ {
			if fields[i] != "dev" {
				continue
			}
			iface := fields[i+1]
			if _, err := normalizeLinuxIfaceName(iface); err != nil {
				break
			}
			if _, ok := seen[iface]; ok {
				break
			}
			seen[iface] = struct{}{}
			devs = append(devs, iface)
			break
		}
	}
	return devs
}

func normalizeLinuxIfaceName(iface string) (string, error) {
	iface = strings.TrimSpace(iface)
	if iface == "" {
		return "", errors.New("iface is empty")
	}
	if len(iface) > 15 {
		return "", fmt.Errorf("iface %q exceeds 15 characters", iface)
	}
	if strings.HasPrefix(iface, "-") || strings.ContainsAny(iface, `/\`) {
		return "", fmt.Errorf("iface %q contains unsupported characters", iface)
	}
	for _, r := range iface {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '.' || r == ':' || r == '@' || r == '-' {
			continue
		}
		return "", fmt.Errorf("iface %q contains unsupported characters", iface)
	}
	return iface, nil
}

func runCommand(name string, args ...string) (string, error) {
	if !filepath.IsAbs(name) {
		return "", fmt.Errorf("command path must be absolute: %s", name)
	}
	// #nosec G204,G702 -- name is restricted to trusted absolute system paths and exec does not invoke a shell.
	cmd := exec.Command(name, args...)
	cmd.Env = sanitizedLinuxCommandEnv(nil)
	out, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(out))
		if trimmed != "" {
			return string(out), fmt.Errorf("%s %s failed: %w: %s", name, strings.Join(args, " "), err, trimmed)
		}
		return string(out), fmt.Errorf("%s %s failed: %w", name, strings.Join(args, " "), err)
	}
	return string(out), nil
}

func lookPath(name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", false
	}
	if filepath.IsAbs(name) {
		if path, ok := canonicalTrustedLinuxCommandPath(name); ok {
			return path, true
		}
		return "", false
	}
	if !isBareTrustedCommandName(name) {
		return "", false
	}
	for _, dir := range trustedLinuxCommandDirs {
		candidate := filepath.Join(dir, name)
		if path, ok := canonicalTrustedLinuxCommandPath(candidate); ok {
			return path, true
		}
	}
	return "", false
}

var linuxLookPath = lookPath
var linuxDetectPackageManager = detectLinuxPackageManager

func isTrustedLinuxCommandPath(path string) bool {
	_, ok := canonicalTrustedLinuxCommandPath(path)
	return ok
}

func canonicalTrustedLinuxCommandPath(path string) (string, bool) {
	clean := filepath.Clean(strings.TrimSpace(path))
	if !filepath.IsAbs(clean) {
		return "", false
	}

	resolved, err := filepath.EvalSymlinks(clean)
	if err != nil {
		return "", false
	}
	resolved = filepath.Clean(resolved)
	if !filepath.IsAbs(resolved) || !isExecutableFile(resolved) {
		return "", false
	}

	for _, dir := range trustedLinuxCommandDirs {
		trustedDir := filepath.Clean(strings.TrimSpace(dir))
		if trustedDir == "" {
			continue
		}
		if canonicalDir, err := filepath.EvalSymlinks(trustedDir); err == nil {
			trustedDir = filepath.Clean(canonicalDir)
		}
		if filepath.Dir(resolved) == trustedDir {
			return resolved, true
		}
	}
	return "", false
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode()&0o111 != 0
}

type linuxToolNeeds struct {
	AutoRules   bool
	AutoOffload bool
	NeedIP      bool
}

func ensureLinuxExternalTools(autoInstall bool, needs linuxToolNeeds) error {
	if !needs.AutoRules && !needs.AutoOffload {
		return nil
	}

	missing := make([]string, 0, 4)
	wantPkgs := make(map[string]struct{})

	if needs.AutoRules {
		if _, ok := linuxLookPath("nft"); !ok {
			_, hasIpt := linuxLookPath("iptables")
			_, hasIpt6 := linuxLookPath("ip6tables")
			if !hasIpt || !hasIpt6 {
				missing = append(missing, "nft or iptables+ip6tables")
				// Install both to maximize compatibility across distros/setups.
				wantPkgs["nftables"] = struct{}{}
				wantPkgs["iptables"] = struct{}{}
			}
		}
	}

	if needs.AutoOffload {
		if _, ok := linuxLookPath("ethtool"); !ok {
			missing = append(missing, "ethtool")
			wantPkgs["ethtool"] = struct{}{}
		}
		if needs.NeedIP {
			if _, ok := linuxLookPath("ip"); !ok {
				missing = append(missing, "ip")
				// Package name differs by distro; map per package manager.
				wantPkgs["__iproute__"] = struct{}{}
			}
		}
	}

	if len(missing) == 0 {
		return nil
	}

	if !autoInstall {
		return fmt.Errorf("missing required external tools: %s (install them or set --auto-install-tools=true)", strings.Join(missing, ", "))
	}

	mgrKind, mgrPath, ok := linuxDetectPackageManager()
	if !ok {
		return fmt.Errorf("missing required external tools: %s (no supported package manager found; install tools manually)", strings.Join(missing, ", "))
	}

	pkgs := make([]string, 0, len(wantPkgs))
	for p := range wantPkgs {
		if p == "__iproute__" {
			pkgs = append(pkgs, iproutePackageName(mgrKind))
			continue
		}
		pkgs = append(pkgs, p)
	}
	sort.Strings(pkgs)
	if len(pkgs) == 0 {
		return fmt.Errorf("missing required external tools: %s", strings.Join(missing, ", "))
	}

	logInfo("linux_install_tools", "installing missing tools", "manager", mgrKind, "packages", strings.Join(pkgs, ","))
	if err := installLinuxPackages(mgrKind, mgrPath, pkgs); err != nil {
		return fmt.Errorf("auto-install-tools failed: %w", err)
	}

	// Re-check after install.
	if needs.AutoRules {
		if _, ok := linuxLookPath("nft"); !ok {
			_, hasIpt := linuxLookPath("iptables")
			_, hasIpt6 := linuxLookPath("ip6tables")
			if !hasIpt || !hasIpt6 {
				return errors.New("auto-install-tools completed, but nft or iptables+ip6tables is still missing")
			}
		}
	}
	if needs.AutoOffload {
		if _, ok := linuxLookPath("ethtool"); !ok {
			return errors.New("auto-install-tools completed, but ethtool still missing")
		}
		if needs.NeedIP {
			if _, ok := linuxLookPath("ip"); !ok {
				return errors.New("auto-install-tools completed, but ip still missing")
			}
		}
	}

	return nil
}

func detectLinuxPackageManager() (kind string, path string, ok bool) {
	for _, name := range []string{"apt-get", "dnf", "yum", "pacman", "apk", "zypper"} {
		if p, ok := linuxLookPath(name); ok {
			return name, p, true
		}
	}
	return "", "", false
}

func iproutePackageName(mgrKind string) string {
	switch mgrKind {
	case "dnf", "yum":
		return "iproute"
	default:
		return "iproute2"
	}
}

func installLinuxPackages(mgrKind string, mgrPath string, pkgs []string) error {
	switch mgrKind {
	case "apt-get":
		env := []string{"DEBIAN_FRONTEND=noninteractive"}
		if _, err := runCommandEnv(env, mgrPath, "update"); err != nil {
			return err
		}
		args := append([]string{"install", "-y", "--no-install-recommends"}, pkgs...)
		_, err := runCommandEnv(env, mgrPath, args...)
		return err
	case "dnf":
		args := append([]string{"install", "-y"}, pkgs...)
		_, err := runCommandEnv(nil, mgrPath, args...)
		return err
	case "yum":
		args := append([]string{"install", "-y"}, pkgs...)
		_, err := runCommandEnv(nil, mgrPath, args...)
		return err
	case "pacman":
		args := append([]string{"-Sy", "--noconfirm", "--needed"}, pkgs...)
		_, err := runCommandEnv(nil, mgrPath, args...)
		return err
	case "apk":
		args := append([]string{"add", "--no-cache"}, pkgs...)
		_, err := runCommandEnv(nil, mgrPath, args...)
		return err
	case "zypper":
		args := append([]string{"--non-interactive", "install", "-y"}, pkgs...)
		_, err := runCommandEnv(nil, mgrPath, args...)
		return err
	default:
		return fmt.Errorf("unsupported package manager: %s", mgrKind)
	}
}

func runCommandEnv(env []string, name string, args ...string) (string, error) {
	if !filepath.IsAbs(name) {
		return "", fmt.Errorf("command path must be absolute: %s", name)
	}
	// #nosec G204 -- name is restricted to trusted absolute system paths and exec does not invoke a shell.
	cmd := exec.Command(name, args...)
	cmd.Env = sanitizedLinuxCommandEnv(env)
	out, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(out))
		if trimmed != "" {
			return string(out), fmt.Errorf("%s %s failed: %w: %s", name, strings.Join(args, " "), err, trimmed)
		}
		return string(out), fmt.Errorf("%s %s failed: %w", name, strings.Join(args, " "), err)
	}
	return string(out), nil
}

func sanitizedLinuxCommandEnv(extra []string) []string {
	env := []string{
		"PATH=" + strings.Join(trustedLinuxCommandDirs, string(os.PathListSeparator)),
		"HOME=/root",
		"LANG=C",
		"LC_ALL=C",
	}
	return append(env, extra...)
}

func readLinuxConfigFile(path string) ([]byte, error) {
	return readJSONConfigFile(path)
}
