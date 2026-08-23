package engine

import (
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
)

type failOpenReason string

const (
	failOpenReasonCollectTimeout       failOpenReason = "collect_timeout"
	failOpenReasonControlFlag          failOpenReason = "tcp_control"
	failOpenReasonHeldBytesLimit       failOpenReason = "held_bytes_limit"
	failOpenReasonHeldPacketsLimit     failOpenReason = "held_packets_limit"
	failOpenReasonIdleGC               failOpenReason = "idle_gc"
	failOpenReasonInjectError          failOpenReason = "inject_error"
	failOpenReasonIPv6ExtensionHeaders failOpenReason = "ipv6_extension_headers"
	failOpenReasonPolicySkip           failOpenReason = "policy_skip"
	failOpenReasonReassemblyBytesLimit failOpenReason = "reassembly_bytes_limit"
	failOpenReasonReassemblyError      failOpenReason = "reassembly_error"
	failOpenReasonStateInvalid         failOpenReason = "state_invalid"
	failOpenReasonTLSMismatch          failOpenReason = "tls_mismatch"
)

type pressureReason string

const (
	pressureReasonACKTouchOverflow     pressureReason = "ack_touch_overflow"
	pressureReasonCollectTimeout       pressureReason = "collect_timeout"
	pressureReasonFlowLimit            pressureReason = "flow_limit"
	pressureReasonHeldBytesLimit       pressureReason = "held_bytes_limit"
	pressureReasonHeldPacketsLimit     pressureReason = "held_packets_limit"
	pressureReasonReassemblyBytesLimit pressureReason = "reassembly_bytes_limit"
	pressureReasonWorkerQueueWait      pressureReason = "worker_queue_wait"
)

type StatsSnapshot struct {
	SplitsOK      uint64
	FailOpenTotal uint64
	FailOpen      map[string]uint64
	PressureTotal uint64
	Pressure      map[string]uint64
}

type stats struct {
	splitsOK                 atomic.Uint64
	failOpenTotal            atomic.Uint64
	pressureTotal            atomic.Uint64
	pressureACKTouchOverflow atomic.Uint64

	failOpenCollectTimeout       atomic.Uint64
	failOpenControlFlag          atomic.Uint64
	failOpenHeldBytesLimit       atomic.Uint64
	failOpenHeldPacketsLimit     atomic.Uint64
	failOpenIdleGC               atomic.Uint64
	failOpenInjectError          atomic.Uint64
	failOpenIPv6ExtensionHeaders atomic.Uint64
	failOpenPolicySkip           atomic.Uint64
	failOpenReassemblyBytesLimit atomic.Uint64
	failOpenReassemblyError      atomic.Uint64
	failOpenStateInvalid         atomic.Uint64
	failOpenTLSMismatch          atomic.Uint64

	pressureCollectTimeout       atomic.Uint64
	pressureFlowLimit            atomic.Uint64
	pressureHeldBytesLimit       atomic.Uint64
	pressureHeldPacketsLimit     atomic.Uint64
	pressureReassemblyBytesLimit atomic.Uint64
	pressureWorkerQueueWait      atomic.Uint64
}

func newStats() *stats {
	return &stats{}
}

func (s *stats) incSplitOK() {
	if s == nil {
		return
	}
	s.splitsOK.Add(1)
}

func (s *stats) incFailOpen(reason failOpenReason) {
	if s == nil {
		return
	}
	s.failOpenTotal.Add(1)
	switch reason {
	case failOpenReasonCollectTimeout:
		s.failOpenCollectTimeout.Add(1)
	case failOpenReasonControlFlag:
		s.failOpenControlFlag.Add(1)
	case failOpenReasonHeldBytesLimit:
		s.failOpenHeldBytesLimit.Add(1)
	case failOpenReasonHeldPacketsLimit:
		s.failOpenHeldPacketsLimit.Add(1)
	case failOpenReasonIdleGC:
		s.failOpenIdleGC.Add(1)
	case failOpenReasonInjectError:
		s.failOpenInjectError.Add(1)
	case failOpenReasonIPv6ExtensionHeaders:
		s.failOpenIPv6ExtensionHeaders.Add(1)
	case failOpenReasonPolicySkip:
		s.failOpenPolicySkip.Add(1)
	case failOpenReasonReassemblyBytesLimit:
		s.failOpenReassemblyBytesLimit.Add(1)
	case failOpenReasonReassemblyError:
		s.failOpenReassemblyError.Add(1)
	case failOpenReasonStateInvalid:
		s.failOpenStateInvalid.Add(1)
	case failOpenReasonTLSMismatch:
		s.failOpenTLSMismatch.Add(1)
	}
}

func (s *stats) incPressure(reason pressureReason) {
	if s == nil {
		return
	}
	s.pressureTotal.Add(1)
	switch reason {
	case pressureReasonACKTouchOverflow:
		s.pressureACKTouchOverflow.Add(1)
	case pressureReasonCollectTimeout:
		s.pressureCollectTimeout.Add(1)
	case pressureReasonFlowLimit:
		s.pressureFlowLimit.Add(1)
	case pressureReasonHeldBytesLimit:
		s.pressureHeldBytesLimit.Add(1)
	case pressureReasonHeldPacketsLimit:
		s.pressureHeldPacketsLimit.Add(1)
	case pressureReasonReassemblyBytesLimit:
		s.pressureReassemblyBytesLimit.Add(1)
	case pressureReasonWorkerQueueWait:
		s.pressureWorkerQueueWait.Add(1)
	}
}

func (s *stats) snapshot() StatsSnapshot {
	snap := StatsSnapshot{
		FailOpen: make(map[string]uint64),
		Pressure: make(map[string]uint64),
	}
	if s == nil {
		return snap
	}

	snap.SplitsOK = s.splitsOK.Load()
	snap.FailOpenTotal = s.failOpenTotal.Load()
	snap.PressureTotal = s.pressureTotal.Load()

	loadCounter(snap.FailOpen, string(failOpenReasonCollectTimeout), &s.failOpenCollectTimeout)
	loadCounter(snap.FailOpen, string(failOpenReasonControlFlag), &s.failOpenControlFlag)
	loadCounter(snap.FailOpen, string(failOpenReasonHeldBytesLimit), &s.failOpenHeldBytesLimit)
	loadCounter(snap.FailOpen, string(failOpenReasonHeldPacketsLimit), &s.failOpenHeldPacketsLimit)
	loadCounter(snap.FailOpen, string(failOpenReasonIdleGC), &s.failOpenIdleGC)
	loadCounter(snap.FailOpen, string(failOpenReasonInjectError), &s.failOpenInjectError)
	loadCounter(snap.FailOpen, string(failOpenReasonIPv6ExtensionHeaders), &s.failOpenIPv6ExtensionHeaders)
	loadCounter(snap.FailOpen, string(failOpenReasonPolicySkip), &s.failOpenPolicySkip)
	loadCounter(snap.FailOpen, string(failOpenReasonReassemblyBytesLimit), &s.failOpenReassemblyBytesLimit)
	loadCounter(snap.FailOpen, string(failOpenReasonReassemblyError), &s.failOpenReassemblyError)
	loadCounter(snap.FailOpen, string(failOpenReasonStateInvalid), &s.failOpenStateInvalid)
	loadCounter(snap.FailOpen, string(failOpenReasonTLSMismatch), &s.failOpenTLSMismatch)

	loadCounter(snap.Pressure, string(pressureReasonACKTouchOverflow), &s.pressureACKTouchOverflow)
	loadCounter(snap.Pressure, string(pressureReasonCollectTimeout), &s.pressureCollectTimeout)
	loadCounter(snap.Pressure, string(pressureReasonFlowLimit), &s.pressureFlowLimit)
	loadCounter(snap.Pressure, string(pressureReasonHeldBytesLimit), &s.pressureHeldBytesLimit)
	loadCounter(snap.Pressure, string(pressureReasonHeldPacketsLimit), &s.pressureHeldPacketsLimit)
	loadCounter(snap.Pressure, string(pressureReasonReassemblyBytesLimit), &s.pressureReassemblyBytesLimit)
	loadCounter(snap.Pressure, string(pressureReasonWorkerQueueWait), &s.pressureWorkerQueueWait)

	return snap
}

func loadCounter(dst map[string]uint64, name string, counter *atomic.Uint64) {
	if v := counter.Load(); v > 0 {
		dst[name] = v
	}
}

func (s StatsSnapshot) String() string {
	return fmt.Sprintf(
		"splits_ok=%d fail_open=%s pressure=%s",
		s.SplitsOK,
		formatCounterMap(s.FailOpen),
		formatCounterMap(s.Pressure),
	)
}

func formatCounterMap(counts map[string]uint64) string {
	if len(counts) == 0 {
		return "{}"
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s:%d", k, counts[k]))
	}
	return "{" + strings.Join(parts, ",") + "}"
}
