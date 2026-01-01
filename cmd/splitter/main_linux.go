//go:build linux

package main

import (
	"context"
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

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg := engine.DefaultConfig()
	const (
		defaultQueueNum    = 100
		defaultQueueMaxLen = 4096
		defaultCopyRange   = 0xffff
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
	queueNum := flag.Int("queue-num", defaultQueueNum, "NFQUEUE number")
	queueMaxLen := flag.Int("queue-maxlen", defaultQueueMaxLen, "NFQUEUE maxlen (0=kernel default)")
	copyRange := flag.Int("copy-range", defaultCopyRange, "NFQUEUE copy range in bytes (0=full packet)")
	mark := flag.Int("mark", defaultMark, "SO_MARK for reinjected packets")
	autoRules := flag.Bool("auto-rules", true, "auto install/uninstall NFQUEUE rules (nft or iptables)")
	autoOffload := flag.Bool("auto-offload", true, "auto disable GRO/GSO/TSO (ethtool)")
	iface := flag.String("iface", "", "egress interface for offload disable (default: auto-detect)")
	noLoopback := flag.Bool("no-loopback", false, "do not exclude loopback from NFQUEUE rules")
	flag.Parse()

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
	if *queueNum < 0 || *queueNum > 65535 {
		return errors.New("queue-num must be in 0..65535")
	}
	if *queueMaxLen < 0 {
		return errors.New("queue-maxlen must be >= 0")
	}
	if *copyRange < 0 {
		return errors.New("copy-range must be >= 0")
	}
	if *mark < 0 {
		return errors.New("mark must be >= 0")
	}
	if *autoRules && *mark == 0 {
		return errors.New("auto-rules requires mark > 0 for reinjection bypass; set --mark or disable --auto-rules")
	}

	cfg.SplitMode = mode
	cfg.SplitChunk = *splitChunk
	cfg.CollectTimeout = *collectTimeout
	cfg.MaxBufferBytes = *maxBuffer
	cfg.MaxHeldPackets = *maxHeld
	cfg.MaxSegmentPayload = *maxSegPayload
	cfg.WorkerCount = *workers
	cfg.FlowIdleTimeout = *flowTimeout
	cfg.GCInterval = *gcInterval

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *autoRules || *autoOffload {
		if os.Geteuid() != 0 {
			return errors.New("auto-rules/auto-offload require root; run as root or set --auto-rules=false --auto-offload=false")
		}
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
		log.Printf("auto rules installed via %s", backend)
		defer func() {
			if rulesCleanup == nil {
				return
			}
			if err := rulesCleanup(); err != nil {
				log.Printf("auto rule uninstall failed: %v", err)
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
		if err := disableOffload(ifaceName); err != nil {
			if rulesCleanup != nil {
				_ = rulesCleanup()
			}
			return fmt.Errorf("disable offload failed: %w", err)
		}
		log.Printf("offload disabled on %s (gro/gso/tso)", ifaceName)
	}

	if *mark == 0 {
		log.Printf("warning: mark=0; ensure NFQUEUE bypass rules prevent reinjection loops")
	}

	opts := adapter.NFQueueOptions{
		QueueNum:    uint16(*queueNum),
		QueueMaxLen: uint32(*queueMaxLen),
		CopyRange:   uint32(*copyRange),
		Mark:        uint32(*mark),
	}
	ad, err := adapter.NewNFQueue(opts)
	if err != nil {
		return fmt.Errorf("NFQUEUE open failed: %w", err)
	}
	eng := engine.New(cfg, ad)

	if err := eng.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("engine stopped: %w", err)
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
	if path, ok := lookPath("nft"); ok {
		if err := installNftRules(path, opts); err != nil {
			return nil, "", err
		}
		return func() error { return uninstallNftRules(path) }, "nft", nil
	}
	if path, ok := lookPath("iptables"); ok {
		if err := installIptablesRules(path, opts); err != nil {
			return nil, "", err
		}
		return func() error { return uninstallIptablesRules(path, opts) }, "iptables", nil
	}
	return nil, "", errors.New("nft or iptables not found in PATH")
}

func installNftRules(path string, opts ruleOptions) error {
	const (
		table = "gov_pass"
		chain = "output"
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

	if _, err := runCommand(path, "flush", "chain", "inet", table, chain); err != nil {
		return fmt.Errorf("nft flush chain failed: %w", err)
	}

	if opts.Mark != 0 {
		mark := fmt.Sprintf("%d", opts.Mark)
		args := []string{"add", "rule", "inet", table, chain, "meta", "mark", "&", mark, "==", mark, "return"}
		if _, err := runCommand(path, args...); err != nil {
			return fmt.Errorf("nft add mark bypass failed: %w", err)
		}
	}

	if opts.ExcludeLoopback {
		args := []string{"add", "rule", "inet", table, chain, "oifname", "lo", "return"}
		if _, err := runCommand(path, args...); err != nil {
			return fmt.Errorf("nft add loopback bypass failed: %w", err)
		}
	}

	queue := fmt.Sprintf("%d", opts.QueueNum)
	args := []string{"add", "rule", "inet", table, chain, "tcp", "dport", "443", "queue", "num", queue, "bypass"}
	if _, err := runCommand(path, args...); err != nil {
		return fmt.Errorf("nft add queue rule failed: %w", err)
	}

	return nil
}

func uninstallNftRules(path string) error {
	const table = "gov_pass"
	if _, err := runCommand(path, "list", "table", "inet", table); err != nil {
		return nil
	}
	if _, err := runCommand(path, "delete", "table", "inet", table); err != nil {
		return fmt.Errorf("nft delete table failed: %w", err)
	}
	return nil
}

func installIptablesRules(path string, opts ruleOptions) error {
	if opts.Mark != 0 {
		mark := fmt.Sprintf("%d/%d", opts.Mark, opts.Mark)
		check := []string{"-t", "mangle", "-C", "OUTPUT", "-m", "mark", "--mark", mark, "-j", "RETURN"}
		add := []string{"-t", "mangle", "-A", "OUTPUT", "-m", "mark", "--mark", mark, "-j", "RETURN"}
		if err := ensureIptablesRule(path, check, add); err != nil {
			return fmt.Errorf("iptables mark bypass failed: %w", err)
		}
	}

	if opts.ExcludeLoopback {
		check := []string{"-t", "mangle", "-C", "OUTPUT", "-o", "lo", "-j", "RETURN"}
		add := []string{"-t", "mangle", "-A", "OUTPUT", "-o", "lo", "-j", "RETURN"}
		if err := ensureIptablesRule(path, check, add); err != nil {
			return fmt.Errorf("iptables loopback bypass failed: %w", err)
		}
	}

	queue := fmt.Sprintf("%d", opts.QueueNum)
	check := []string{"-t", "mangle", "-C", "OUTPUT", "-p", "tcp", "--dport", "443", "-j", "NFQUEUE", "--queue-num", queue, "--queue-bypass"}
	add := []string{"-t", "mangle", "-A", "OUTPUT", "-p", "tcp", "--dport", "443", "-j", "NFQUEUE", "--queue-num", queue, "--queue-bypass"}
	if err := ensureIptablesRule(path, check, add); err != nil {
		return fmt.Errorf("iptables queue rule failed: %w", err)
	}

	return nil
}

func uninstallIptablesRules(path string, opts ruleOptions) error {
	queue := fmt.Sprintf("%d", opts.QueueNum)
	if _, err := runCommand(path, "-t", "mangle", "-D", "OUTPUT", "-p", "tcp", "--dport", "443", "-j", "NFQUEUE", "--queue-num", queue, "--queue-bypass"); err != nil {
		// ignore
	}
	if opts.ExcludeLoopback {
		if _, err := runCommand(path, "-t", "mangle", "-D", "OUTPUT", "-o", "lo", "-j", "RETURN"); err != nil {
			// ignore
		}
	}
	if opts.Mark != 0 {
		mark := fmt.Sprintf("%d/%d", opts.Mark, opts.Mark)
		if _, err := runCommand(path, "-t", "mangle", "-D", "OUTPUT", "-m", "mark", "--mark", mark, "-j", "RETURN"); err != nil {
			// ignore
		}
	}
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

func disableOffload(iface string) error {
	iface = strings.TrimSpace(iface)
	if iface == "" {
		return errors.New("iface is empty")
	}
	path, ok := lookPath("ethtool")
	if !ok {
		return errors.New("ethtool not found in PATH")
	}
	if _, err := runCommand(path, "-K", iface, "gro", "off", "gso", "off", "tso", "off"); err != nil {
		return err
	}
	return nil
}

func detectEgressInterface() (string, error) {
	path, ok := lookPath("ip")
	if !ok {
		return "", errors.New("ip command not found in PATH; use --iface")
	}

	out, err := runCommand(path, "-4", "route", "get", "1.1.1.1")
	if err == nil {
		if iface := parseRouteDev(out); iface != "" {
			return iface, nil
		}
	}

	out, err = runCommand(path, "-4", "route", "show", "default")
	if err != nil {
		return "", fmt.Errorf("ip route lookup failed: %w", err)
	}
	if iface := parseRouteDev(out); iface != "" {
		return iface, nil
	}
	return "", errors.New("could not detect egress interface; use --iface")
}

func parseRouteDev(output string) string {
	fields := strings.Fields(output)
	for i := 0; i+1 < len(fields); i++ {
		if fields[i] == "dev" {
			return fields[i+1]
		}
	}
	return ""
}

func runCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
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
	path, err := exec.LookPath(name)
	if err == nil {
		return path, true
	}
	for _, dir := range []string{"/usr/sbin", "/sbin"} {
		candidate := filepath.Join(dir, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}
	}
	return "", false
}
