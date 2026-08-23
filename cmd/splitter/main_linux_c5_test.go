//go:build linux

package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fk-gov/internal/engine"
)

func writeTempJSON(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "cfg.json")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp json: %v", err)
	}
	return p
}

func linuxExampleConfigRefs() *linuxFlagRefs {
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
	queueNum := 100
	queueMaxLen := 4096
	copyRange := 0xffff
	recvBuffer := 0
	mark := 1
	autoRules := true
	autoOffload := true
	autoOffloadRestore := true
	autoInstallTools := true
	iface := ""
	noLoopback := false

	return &linuxFlagRefs{
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
		QueueNum:                   &queueNum,
		QueueMaxLen:                &queueMaxLen,
		CopyRange:                  &copyRange,
		RecvBuffer:                 &recvBuffer,
		Mark:                       &mark,
		AutoRules:                  &autoRules,
		AutoOffload:                &autoOffload,
		AutoOffloadRestore:         &autoOffloadRestore,
		AutoInstallTools:           &autoInstallTools,
		Iface:                      &iface,
		NoLoopback:                 &noLoopback,
	}
}

func withCapturedStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	runErr := fn()
	_ = w.Close()
	os.Stdout = orig
	out, _ := io.ReadAll(r)
	_ = r.Close()
	return string(out), runErr
}

func hasCheck(report preflightReport, name string) bool {
	for i := range report.Checks {
		if report.Checks[i].Name == name {
			return true
		}
	}
	return false
}

func TestApplyLinuxJSONConfig_MergeOrder(t *testing.T) {
	splitMode := "tls-hello"
	splitChunk := 5
	collectTimeout := 250 * time.Millisecond
	statsInterval := defaultStatsInterval
	queueNum := 100
	recvBuffer := 0
	autoRules := true
	iface := ""
	noLoopback := false

	refs := &linuxFlagRefs{
		SplitMode:      &splitMode,
		SplitChunk:     &splitChunk,
		CollectTimeout: &collectTimeout,
		StatsInterval:  &statsInterval,
		QueueNum:       &queueNum,
		RecvBuffer:     &recvBuffer,
		AutoRules:      &autoRules,
		Iface:          &iface,
		NoLoopback:     &noLoopback,
	}

	cfgPath := writeTempJSON(t, `{
  "engine": {
    "split_mode": "immediate",
    "split_chunk": 9,
    "collect_timeout": "1s",
    "stats_interval": "2m"
  },
	  "linux": {
	    "queue_num": 321,
	    "recv_buffer": 2048,
	    "auto_rules": false,
	    "iface": "eth9",
    "no_loopback": true
  }
}`)

	if err := applyLinuxJSONConfig(cfgPath, map[string]bool{}, refs); err != nil {
		t.Fatalf("apply config: %v", err)
	}

	if splitMode != "immediate" {
		t.Fatalf("splitMode mismatch: %q", splitMode)
	}
	if splitChunk != 9 {
		t.Fatalf("splitChunk mismatch: %d", splitChunk)
	}
	if collectTimeout != time.Second {
		t.Fatalf("collectTimeout mismatch: %s", collectTimeout)
	}
	if statsInterval != 2*time.Minute {
		t.Fatalf("statsInterval mismatch: %s", statsInterval)
	}
	if queueNum != 321 {
		t.Fatalf("queueNum mismatch: %d", queueNum)
	}
	if recvBuffer != 2048 {
		t.Fatalf("recvBuffer mismatch: %d", recvBuffer)
	}
	if autoRules {
		t.Fatalf("autoRules should be false")
	}
	if iface != "eth9" {
		t.Fatalf("iface mismatch: %q", iface)
	}
	if !noLoopback {
		t.Fatalf("noLoopback should be true")
	}
}

func TestApplyLinuxJSONConfig_ExplicitFlagsOverride(t *testing.T) {
	splitChunk := 5
	statsInterval := defaultStatsInterval
	queueNum := 100
	recvBuffer := 0
	autoRules := true
	iface := "eth0"

	refs := &linuxFlagRefs{
		SplitChunk:    &splitChunk,
		StatsInterval: &statsInterval,
		QueueNum:      &queueNum,
		RecvBuffer:    &recvBuffer,
		AutoRules:     &autoRules,
		Iface:         &iface,
	}

	cfgPath := writeTempJSON(t, `{
  "engine": { "split_chunk": 9, "stats_interval": "2m" },
  "linux": {
	    "queue_num": 321,
	    "recv_buffer": 2048,
	    "auto_rules": false,
    "iface": "eth9"
  }
}`)

	setFlags := map[string]bool{
		"split-chunk":    true,
		"stats-interval": true,
		"queue-num":      true,
		"recv-buffer":    true,
		"auto-rules":     true,
		"iface":          true,
	}
	if err := applyLinuxJSONConfig(cfgPath, setFlags, refs); err != nil {
		t.Fatalf("apply config: %v", err)
	}

	if splitChunk != 5 {
		t.Fatalf("splitChunk should remain 5, got %d", splitChunk)
	}
	if statsInterval != defaultStatsInterval {
		t.Fatalf("statsInterval should remain default, got %s", statsInterval)
	}
	if queueNum != 100 {
		t.Fatalf("queueNum should remain 100, got %d", queueNum)
	}
	if recvBuffer != 0 {
		t.Fatalf("recvBuffer should remain 0, got %d", recvBuffer)
	}
	if !autoRules {
		t.Fatalf("autoRules should remain true")
	}
	if iface != "eth0" {
		t.Fatalf("iface should remain eth0, got %q", iface)
	}
}

func TestApplyLinuxJSONConfig_InvalidDuration(t *testing.T) {
	collectTimeout := 250 * time.Millisecond
	refs := &linuxFlagRefs{CollectTimeout: &collectTimeout}
	cfgPath := writeTempJSON(t, `{
  "engine": { "collect_timeout": "not-a-duration" }
}`)

	err := applyLinuxJSONConfig(cfgPath, map[string]bool{}, refs)
	if err == nil || !strings.Contains(err.Error(), "engine.collect_timeout") {
		t.Fatalf("expected collect_timeout error, got %v", err)
	}
}

func TestApplyLinuxJSONConfig_Policies(t *testing.T) {
	var policies []engine.Policy
	refs := &linuxFlagRefs{
		Policies: &policies,
	}
	cfgPath := writeTempJSON(t, `{
  "engine": {
    "policies": [
      {
        "name": "cloudflare-skip",
        "dst_cidrs": ["1.1.1.0/24"],
        "skip": true
      },
      {
        "name": "example-sni",
        "sni_suffixes": ["example.com"],
        "split_chunk": 9
      }
    ]
  }
}`)

	if err := applyLinuxJSONConfig(cfgPath, map[string]bool{}, refs); err != nil {
		t.Fatalf("apply config: %v", err)
	}
	if len(policies) != 2 {
		t.Fatalf("expected 2 policies, got %d", len(policies))
	}
	if !policies[0].Skip {
		t.Fatal("expected skip policy")
	}
	if len(policies[1].SNISuffixes) != 1 || policies[1].SNISuffixes[0] != "example.com" {
		t.Fatalf("unexpected SNI policy: %+v", policies[1])
	}
}

func TestLinuxExampleConfigApplies(t *testing.T) {
	path := filepath.Join("..", "..", "docs", "examples", "splitter.linux.json")
	if err := applyLinuxJSONConfig(path, map[string]bool{}, linuxExampleConfigRefs()); err != nil {
		t.Fatalf("apply example config: %v", err)
	}
}

func TestApplyLinuxJSONConfigRejectsUnknownFields(t *testing.T) {
	refs := linuxExampleConfigRefs()
	cfgPath := writeTempJSON(t, `{
  "engine": {
    "split_chunk": 9,
    "split_chonk": 10
  }
}`)

	err := applyLinuxJSONConfig(cfgPath, map[string]bool{}, refs)
	if err == nil || !strings.Contains(err.Error(), `unknown field "split_chonk"`) {
		t.Fatalf("expected unknown field error, got %v", err)
	}
}

func TestRunLinuxPreflight_JSONSuccessWithAutoInstallFallback(t *testing.T) {
	origLookPath := linuxLookPath
	origDetect := linuxDetectPackageManager
	defer func() {
		linuxLookPath = origLookPath
		linuxDetectPackageManager = origDetect
	}()

	linuxLookPath = func(name string) (string, bool) { return "", false }
	linuxDetectPackageManager = func() (string, string, bool) {
		return "apt-get", "/usr/bin/apt-get", true
	}

	out, err := withCapturedStdout(t, func() error {
		return runLinuxPreflight(true, linuxToolNeeds{
			AutoRules:   true,
			AutoOffload: true,
			NeedIP:      true,
		}, false, true)
	})
	if err != nil {
		t.Fatalf("preflight should succeed, got %v", err)
	}

	var report preflightReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("json parse failed: %v, out=%q", err, out)
	}
	if !report.OK {
		t.Fatalf("report should be OK: %+v", report)
	}
	if !hasCheck(report, "nft_or_iptables") || !hasCheck(report, "ethtool") || !hasCheck(report, "ip") {
		t.Fatalf("missing expected checks: %+v", report.Checks)
	}
}

func TestRunLinuxPreflight_FailWhenMissingToolsAndNoAutoInstall(t *testing.T) {
	origLookPath := linuxLookPath
	origDetect := linuxDetectPackageManager
	defer func() {
		linuxLookPath = origLookPath
		linuxDetectPackageManager = origDetect
	}()

	linuxLookPath = func(name string) (string, bool) { return "", false }
	linuxDetectPackageManager = func() (string, string, bool) { return "", "", false }

	_, err := withCapturedStdout(t, func() error {
		return runLinuxPreflight(false, linuxToolNeeds{AutoRules: true}, false, false)
	})
	if err == nil {
		t.Fatal("expected preflight failure")
	}
}

func TestRunLinuxPreflight_RootCheckJSON(t *testing.T) {
	origLookPath := linuxLookPath
	origDetect := linuxDetectPackageManager
	defer func() {
		linuxLookPath = origLookPath
		linuxDetectPackageManager = origDetect
	}()

	linuxLookPath = func(name string) (string, bool) { return "/bin/true", true }
	linuxDetectPackageManager = func() (string, string, bool) { return "", "", false }

	out, err := withCapturedStdout(t, func() error {
		return runLinuxPreflight(false, linuxToolNeeds{}, true, true)
	})

	var report preflightReport
	if jerr := json.Unmarshal([]byte(out), &report); jerr != nil {
		t.Fatalf("json parse failed: %v, out=%q", jerr, out)
	}

	if os.Geteuid() == 0 {
		if err != nil || !report.OK {
			t.Fatalf("root preflight should pass: err=%v report=%+v", err, report)
		}
	} else {
		if err == nil || report.OK {
			t.Fatalf("non-root preflight should fail: err=%v report=%+v", err, report)
		}
	}
	if !hasCheck(report, "root") {
		t.Fatalf("root check missing: %+v", report.Checks)
	}
}
