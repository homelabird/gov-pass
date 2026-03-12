package engine

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"fk-gov/internal/adapter"
	"fk-gov/internal/flow"
	"fk-gov/internal/packet"
	"fk-gov/internal/reassembly"
	"fk-gov/internal/safecast"
	"fk-gov/internal/tls"
)

const maxIPv4TotalLen = 0xffff

var ErrShutdownFailOpenLimitReached = errors.New("shutdown fail-open packet limit reached")

type worker struct {
	id      int
	cfg     atomic.Pointer[Config]
	adapter adapter.Adapter
	stats   *stats
	in      chan *packet.Packet
	touch   chan flow.Key
	flows   *flow.Table

	heldBytes       int64
	reassemblyBytes int64
}

func newWorker(id int, cfg Config, ad adapter.Adapter, st *stats) *worker {
	if cfg.WorkerQueueSize < 1 {
		cfg.WorkerQueueSize = 1024
	}
	if cfg.GCInterval <= 0 {
		cfg.GCInterval = 5 * time.Second
	}
	if cfg.FlowIdleTimeout <= 0 {
		cfg.FlowIdleTimeout = 30 * time.Second
	}
	if cfg.MaxFlowsPerWorker < 0 {
		cfg.MaxFlowsPerWorker = 0
	}
	if cfg.MaxReassemblyBytesPerWorker < 0 {
		cfg.MaxReassemblyBytesPerWorker = 0
	}
	if cfg.MaxHeldBytesPerWorker < 0 {
		cfg.MaxHeldBytesPerWorker = 0
	}
	w := &worker{
		id:      id,
		adapter: ad,
		stats:   st,
		in:      make(chan *packet.Packet, cfg.WorkerQueueSize),
		touch:   make(chan flow.Key, cfg.WorkerQueueSize),
		flows:   flow.NewTable(),
	}
	cfgCopy := cloneConfig(cfg)
	w.cfg.Store(&cfgCopy)
	return w
}

func (w *worker) enqueue(ctx context.Context, pkt *packet.Packet) error {
	// Once canceled, never enqueue more packets. This avoids leaving captured
	// packets stuck in the queue while workers are shutting down.
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case w.in <- pkt:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *worker) touchFlow(key flow.Key) bool {
	select {
	case w.touch <- key:
		return true
	default:
		return false
	}
}

func (w *worker) setConfig(cfg Config) {
	cfgCopy := cloneConfig(cfg)
	w.cfg.Store(&cfgCopy)
}

func (w *worker) close() {
	close(w.in)
	close(w.touch)
}

func (w *worker) run(ctx context.Context) (err error) {
	interval := 5 * time.Second
	if cfg := w.cfg.Load(); cfg != nil && cfg.GCInterval > 0 {
		interval = cfg.GCInterval
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()

	for {
		select {
		case pkt, ok := <-w.in:
			if !ok {
				return nil
			}
			if err := w.handlePacket(ctx, pkt); err != nil {
				return err
			}
		case key, ok := <-w.touch:
			if !ok {
				w.touch = nil
				continue
			}
			// Fast-path for ACK-only packets: keep existing flows alive without
			// enqueueing the full packet through the worker queue.
			if st, ok := w.flows.Get(key); ok {
				st.LastActive = time.Now()
			}
		case <-timer.C:
			if err := w.gc(ctx); err != nil {
				return err
			}
			next := 5 * time.Second
			if cfg := w.cfg.Load(); cfg != nil && cfg.GCInterval > 0 {
				next = cfg.GCInterval
			}
			timer.Reset(next)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (w *worker) handlePacket(ctx context.Context, pkt *packet.Packet) error {
	cfg := w.cfg.Load()
	if cfg == nil {
		return errors.New("worker config is nil")
	}

	now := time.Now()
	key := flow.KeyFromMeta(pkt.Meta)
	payload := pkt.Payload()
	if st, ok := w.flows.Get(key); ok {
		st.LastActive = now
		plan, resolved := w.resolveFlowPlan(st, pkt.Meta, nil, *cfg)

		// FIN/RST often have no payload; ensure they still clean up flow state
		// promptly even when payloadless packets are fast-pathed.
		if len(payload) == 0 && (pkt.HasFlag(packet.TCPFlagRST) || pkt.HasFlag(packet.TCPFlagFIN)) {
			if st.State == flow.StateCollecting {
				if err := w.failOpenWithReason(ctx, key, st, failOpenReasonControlFlag); err != nil {
					return err
				}
			}
			if err := sendPacket(ctx, w.adapter, pkt); err != nil {
				return err
			}
			w.flows.Delete(key)
			return nil
		}

		if st.State == flow.StateInjected || st.State == flow.StatePassThrough {
			return sendPacket(ctx, w.adapter, pkt)
		}
		if len(payload) == 0 {
			return sendPacket(ctx, w.adapter, pkt)
		}
		if resolved && plan.Skip {
			if st.State == flow.StateCollecting && len(st.HeldPackets) > 0 {
				if err := w.failOpenWithReason(ctx, key, st, failOpenReasonPolicySkip); err != nil {
					return err
				}
			}
			st.State = flow.StatePassThrough
			return sendPacket(ctx, w.adapter, pkt)
		}

		if st.State == flow.StateNew {
			maxBufferBytes, ok := safecast.IntToUint32(cfg.MaxBufferBytes)
			if !ok {
				return errors.New("max-buffer exceeds uint32")
			}
			st.BaseSeq = pkt.Meta.Seq
			st.Reassembler = reassembly.New(st.BaseSeq, maxBufferBytes)
			st.State = flow.StateCollecting
			st.CollectStart = now
			st.FirstPayloadLen = len(payload)
			st.Template = pkt
		} else {
			st.Template = pkt
		}

		if cfg.MaxHeldBytesPerWorker > 0 {
			need := int64(len(pkt.Data))
			limit := int64(cfg.MaxHeldBytesPerWorker)
			if w.heldBytes+need > limit {
				if err := w.failOpenWithReason(ctx, key, st, failOpenReasonHeldBytesLimit); err != nil {
					return err
				}
				return sendPacket(ctx, w.adapter, pkt)
			}
		}
		st.HeldPackets = append(st.HeldPackets, pkt)
		w.heldBytes += int64(len(pkt.Data))
		if len(st.HeldPackets) > cfg.MaxHeldPackets {
			return w.failOpenWithReason(ctx, key, st, failOpenReasonHeldPacketsLimit)
		}
		if now.Sub(st.CollectStart) > cfg.CollectTimeout {
			return w.failOpenWithReason(ctx, key, st, failOpenReasonCollectTimeout)
		}
		if st.Reassembler == nil {
			return w.failOpenWithReason(ctx, key, st, failOpenReasonStateInvalid)
		}
		before := int64(st.Reassembler.TotalBytes())
		err := st.Reassembler.Push(pkt.Meta.Seq, payload)
		after := int64(st.Reassembler.TotalBytes())
		w.reassemblyBytes += after - before
		if w.reassemblyBytes < 0 {
			w.reassemblyBytes = 0
		}
		if err != nil {
			return w.failOpenWithReason(ctx, key, st, failOpenReasonReassemblyError)
		}
		if cfg.MaxReassemblyBytesPerWorker > 0 && w.reassemblyBytes > int64(cfg.MaxReassemblyBytesPerWorker) {
			return w.failOpenWithReason(ctx, key, st, failOpenReasonReassemblyBytesLimit)
		}

		if pkt.HasFlag(packet.TCPFlagSYN) {
			return w.failOpenWithReason(ctx, key, st, failOpenReasonControlFlag)
		}

		if pkt.HasFlag(packet.TCPFlagRST) {
			if err := w.failOpenWithReason(ctx, key, st, failOpenReasonControlFlag); err != nil {
				return err
			}
			w.flows.Delete(key)
			return nil
		}

		if pkt.HasFlag(packet.TCPFlagFIN) {
			if err := w.failOpenWithReason(ctx, key, st, failOpenReasonControlFlag); err != nil {
				return err
			}
			w.flows.Delete(key)
			return nil
		}

		if !resolved {
			return w.trySplitTLSHello(ctx, key, st)
		}

		if plan.SplitMode == SplitModeImmediate {
			return w.trySplitImmediate(ctx, key, st)
		}

		if plan.SplitMode == SplitModeTLSHello {
			return w.trySplitTLSHello(ctx, key, st)
		}

		return nil
	}

	// No existing state: fail-open for payloadless packets (no flow creation).
	if len(payload) == 0 {
		return sendPacket(ctx, w.adapter, pkt)
	}

	// DoS guard: bound the number of tracked flows per worker.
	if cfg.MaxFlowsPerWorker > 0 && w.flows.Len() >= cfg.MaxFlowsPerWorker {
		w.notePressure(pressureReasonFlowLimit)
		return sendPacket(ctx, w.adapter, pkt)
	}

	plan, resolved := cfg.resolvePlan(pkt.Meta, nil)
	if resolved && plan.Skip {
		st := w.flows.GetOrCreate(key, now)
		st.LastActive = now
		w.storeFlowPlan(st, plan)
		st.State = flow.StatePassThrough
		return sendPacket(ctx, w.adapter, pkt)
	}

	// Best-effort budget checks before creating per-flow state.
	if cfg.MaxHeldBytesPerWorker > 0 {
		need := int64(len(pkt.Data))
		limit := int64(cfg.MaxHeldBytesPerWorker)
		if w.heldBytes+need > limit {
			w.notePressure(pressureReasonHeldBytesLimit)
			return sendPacket(ctx, w.adapter, pkt)
		}
	}
	if cfg.MaxReassemblyBytesPerWorker > 0 {
		need := int64(len(payload))
		limit := int64(cfg.MaxReassemblyBytesPerWorker)
		if w.reassemblyBytes+need > limit {
			w.notePressure(pressureReasonReassemblyBytesLimit)
			return sendPacket(ctx, w.adapter, pkt)
		}
	}

	st := w.flows.GetOrCreate(key, now)
	st.LastActive = now
	if resolved {
		w.storeFlowPlan(st, plan)
	}

	if st.State == flow.StateNew {
		maxBufferBytes, ok := safecast.IntToUint32(cfg.MaxBufferBytes)
		if !ok {
			return errors.New("max-buffer exceeds uint32")
		}
		st.BaseSeq = pkt.Meta.Seq
		st.Reassembler = reassembly.New(st.BaseSeq, maxBufferBytes)
		st.State = flow.StateCollecting
		st.CollectStart = now
		st.FirstPayloadLen = len(payload)
		st.Template = pkt
	} else {
		st.Template = pkt
	}

	if cfg.MaxHeldBytesPerWorker > 0 {
		need := int64(len(pkt.Data))
		limit := int64(cfg.MaxHeldBytesPerWorker)
		if w.heldBytes+need > limit {
			if err := w.failOpenWithReason(ctx, key, st, failOpenReasonHeldBytesLimit); err != nil {
				return err
			}
			return sendPacket(ctx, w.adapter, pkt)
		}
	}
	st.HeldPackets = append(st.HeldPackets, pkt)
	w.heldBytes += int64(len(pkt.Data))
	if len(st.HeldPackets) > cfg.MaxHeldPackets {
		return w.failOpenWithReason(ctx, key, st, failOpenReasonHeldPacketsLimit)
	}
	if now.Sub(st.CollectStart) > cfg.CollectTimeout {
		return w.failOpenWithReason(ctx, key, st, failOpenReasonCollectTimeout)
	}
	before := int64(st.Reassembler.TotalBytes())
	err := st.Reassembler.Push(pkt.Meta.Seq, payload)
	after := int64(st.Reassembler.TotalBytes())
	w.reassemblyBytes += after - before
	if w.reassemblyBytes < 0 {
		w.reassemblyBytes = 0
	}
	if err != nil {
		return w.failOpenWithReason(ctx, key, st, failOpenReasonReassemblyError)
	}
	if cfg.MaxReassemblyBytesPerWorker > 0 && w.reassemblyBytes > int64(cfg.MaxReassemblyBytesPerWorker) {
		return w.failOpenWithReason(ctx, key, st, failOpenReasonReassemblyBytesLimit)
	}

	if pkt.HasFlag(packet.TCPFlagSYN) || pkt.HasFlag(packet.TCPFlagRST) {
		if err := w.failOpenWithReason(ctx, key, st, failOpenReasonControlFlag); err != nil {
			return err
		}
		if pkt.HasFlag(packet.TCPFlagRST) {
			w.flows.Delete(key)
		}
		return nil
	}

	if pkt.HasFlag(packet.TCPFlagFIN) {
		if err := w.failOpenWithReason(ctx, key, st, failOpenReasonControlFlag); err != nil {
			return err
		}
		w.flows.Delete(key)
		return nil
	}

	if !resolved {
		return w.trySplitTLSHello(ctx, key, st)
	}

	if plan.SplitMode == SplitModeImmediate {
		return w.trySplitImmediate(ctx, key, st)
	}

	if plan.SplitMode == SplitModeTLSHello {
		return w.trySplitTLSHello(ctx, key, st)
	}

	return nil
}

func (w *worker) trySplitImmediate(ctx context.Context, key flow.Key, st *flow.FlowState) error {
	if st.FirstPayloadLen <= 0 {
		return nil
	}
	if st.Reassembler == nil {
		return w.failOpenWithReason(ctx, key, st, failOpenReasonStateInvalid)
	}
	contig := st.Reassembler.Contiguous()
	if len(contig) < st.FirstPayloadLen {
		return nil
	}
	return w.injectWindow(ctx, key, st, st.FirstPayloadLen)
}

func (w *worker) trySplitTLSHello(ctx context.Context, key flow.Key, st *flow.FlowState) error {
	cfg := w.cfg.Load()
	if cfg == nil {
		return errors.New("worker config is nil")
	}

	if st.Reassembler == nil {
		return w.failOpenWithReason(ctx, key, st, failOpenReasonStateInvalid)
	}
	contig := st.Reassembler.Contiguous()
	recordLen, result := tls.DetectClientHelloRecord(contig)
	if result == tls.ResultNeedMore {
		return nil
	}
	if result == tls.ResultMismatch {
		return w.failOpenWithReason(ctx, key, st, failOpenReasonTLSMismatch)
	}

	need := 5 + int(recordLen)
	if need > cfg.MaxBufferBytes {
		return w.failOpenWithReason(ctx, key, st, failOpenReasonStateInvalid)
	}
	if len(contig) < need {
		return nil
	}

	info, result := tls.ParseClientHello(contig[:need])
	if result == tls.ResultNeedMore {
		return nil
	}
	if result == tls.ResultMismatch {
		return w.failOpenWithReason(ctx, key, st, failOpenReasonTLSMismatch)
	}
	plan, resolved := w.resolveFlowPlan(st, tplMeta(st), &info, *cfg)
	if !resolved {
		return nil
	}
	if plan.Skip {
		return w.failOpenWithReason(ctx, key, st, failOpenReasonPolicySkip)
	}
	if plan.SplitMode == SplitModeImmediate {
		return w.trySplitImmediate(ctx, key, st)
	}
	return w.injectWindow(ctx, key, st, need)
}

func (w *worker) injectWindow(ctx context.Context, key flow.Key, st *flow.FlowState, windowLen int) error {
	cfg := w.cfg.Load()
	if cfg == nil {
		return errors.New("worker config is nil")
	}
	plan, resolved := w.resolveFlowPlan(st, tplMeta(st), nil, *cfg)
	if !resolved {
		return w.failOpenWithReason(ctx, key, st, failOpenReasonStateInvalid)
	}

	if windowLen < 1 {
		return w.failOpenWithReason(ctx, key, st, failOpenReasonStateInvalid)
	}
	contig := st.Reassembler.Contiguous()
	if len(contig) < windowLen {
		return nil
	}
	tpl := st.Template
	if tpl == nil {
		return w.failOpenWithReason(ctx, key, st, failOpenReasonStateInvalid)
	}
	// Short-term safety guard: Linux IPv6 raw reinjection cannot preserve
	// extension headers yet, so bypass splitting rather than emitting a
	// semantically different packet.
	if shouldFailOpenIPv6ExtensionHeaders(tpl) {
		return w.failOpenWithReason(ctx, key, st, failOpenReasonIPv6ExtensionHeaders)
	}
	maxPayload := len(tpl.Payload())
	headerLen := tpl.Meta.IPHeaderLen + tpl.Meta.TCPHeaderLen
	maxPayload = clampSegmentPayload(maxPayload, headerLen, plan.MaxSegmentPayload)
	if maxPayload < 1 {
		return w.failOpenWithReason(ctx, key, st, failOpenReasonStateInvalid)
	}

	window := contig[:windowLen]
	remainder := contig[windowLen:]

	splitSegs := splitFirst(window, plan.SplitChunk, maxPayload)
	if len(splitSegs) < 2 {
		return w.failOpenWithReason(ctx, key, st, failOpenReasonStateInvalid)
	}

	flags := tpl.Meta.Flags
	flagsNoPshFin := flags &^ (packet.TCPFlagPSH | packet.TCPFlagFIN)
	splitLastFlags := flags
	if len(remainder) > 0 {
		splitLastFlags = flagsNoPshFin
	}

	var ipid *uint16
	if tpl.Meta.IPVersion == packet.IPVersion4 {
		nextID := packet.IPv4ID(tpl.Data)
		ipid = &nextID
	}
	held := w.takeHeldPackets(st)
	if len(held) == 0 {
		return w.failOpenWithReason(ctx, key, st, failOpenReasonStateInvalid)
	}

	sentCount, err := w.sendSegments(ctx, tpl, st.BaseSeq, splitSegs, flagsNoPshFin, splitLastFlags, ipid)
	if err != nil {
		return w.handleDetachedInjectError(ctx, st, held, sentCount > 0)
	}
	sentAny := sentCount > 0

	if len(remainder) > 0 {
		windowLen32, ok := safecast.IntToUint32(windowLen)
		if !ok {
			w.noteFailOpenReason(failOpenReasonStateInvalid)
			w.releaseDetachedPackets(held)
			st.State = flow.StatePassThrough
			w.clearCollectingState(st)
			return nil
		}
		if w.canTrimRemainder(st) {
			sentCount, err = w.reinjectTrimmed(ctx, held, st.BaseSeq, windowLen32, ipid)
			if err != nil {
				return w.handleDetachedInjectError(ctx, st, held, sentAny || sentCount > 0)
			}
			sentAny = sentAny || sentCount > 0
		} else {
			remSegs := chunkPayload(remainder, maxPayload)
			sentCount, err = w.sendSegments(ctx, tpl, st.BaseSeq+windowLen32, remSegs, flagsNoPshFin, flags, ipid)
			if err != nil {
				return w.handleDetachedInjectError(ctx, st, held, sentAny || sentCount > 0)
			}
			sentAny = sentAny || sentCount > 0
		}
	}

	if err := w.dropDetachedPackets(ctx, held); err != nil {
		w.noteFailOpenReason(failOpenReasonInjectError)
		w.releaseDetachedPackets(held)
		st.State = flow.StateInjected
		w.clearCollectingState(st)
		st.Processed = true
		return nil
	}
	w.releaseDetachedPackets(held)

	w.noteSplitOK()
	st.State = flow.StateInjected
	w.clearCollectingState(st)
	st.Processed = true
	return nil
}

func (w *worker) resolveFlowPlan(st *flow.FlowState, meta packet.Meta, hello *tls.ClientHelloInfo, cfg Config) (flowPlan, bool) {
	if st != nil && st.PolicyResolved {
		return flowPlan{
			Skip:              st.PolicySkip,
			SplitMode:         SplitMode(st.SplitModeValue),
			SplitChunk:        st.SplitChunk,
			MaxSegmentPayload: st.MaxSegmentPayload,
		}, true
	}
	plan, resolved := cfg.resolvePlan(meta, hello)
	if resolved {
		w.storeFlowPlan(st, plan)
	}
	return plan, resolved
}

func (w *worker) storeFlowPlan(st *flow.FlowState, plan flowPlan) {
	if st == nil {
		return
	}
	st.PolicyResolved = true
	st.PolicySkip = plan.Skip
	st.SplitModeValue = uint8(plan.SplitMode)
	st.SplitChunk = plan.SplitChunk
	st.MaxSegmentPayload = plan.MaxSegmentPayload
}

func tplMeta(st *flow.FlowState) packet.Meta {
	if st == nil || st.Template == nil {
		return packet.Meta{}
	}
	return st.Template.Meta
}

func (w *worker) sendSegments(ctx context.Context, tpl *packet.Packet, baseSeq uint32, segments [][]byte, flags uint8, lastFlags uint8, ipid *uint16) (int, error) {
	offset := 0
	sent := 0
	for i, segPayload := range segments {
		if len(segPayload) == 0 {
			continue
		}
		segFlags := flags
		if i == len(segments)-1 {
			segFlags = lastFlags
		}
		offset32, ok := safecast.IntToUint32(offset)
		if !ok {
			return sent, errors.New("segment offset exceeds uint32")
		}
		newPkt, err := buildPacket(tpl, baseSeq+offset32, segPayload, segFlags, ipid)
		if err != nil {
			return sent, err
		}
		if err := w.adapter.CalcChecksums(newPkt); err != nil {
			return sent, err
		}
		if err := sendPacket(ctx, w.adapter, newPkt); err != nil {
			return sent, err
		}
		sent++
		offset += len(segPayload)
	}
	return sent, nil
}

func buildPacket(tpl *packet.Packet, seq uint32, payload []byte, flags uint8, ipid *uint16) (*packet.Packet, error) {
	if tpl == nil {
		return nil, errors.New("template packet is nil")
	}
	ipHeaderLen := tpl.Meta.IPHeaderLen
	tcpHeaderLen := tpl.Meta.TCPHeaderLen
	headerLen := ipHeaderLen + tcpHeaderLen
	if headerLen <= 0 || headerLen > len(tpl.Data) {
		return nil, errors.New("invalid header length")
	}

	buf := make([]byte, headerLen+len(payload))
	copy(buf, tpl.Data[:headerLen])

	switch tpl.Meta.IPVersion {
	case packet.IPVersion4:
		totalLen, ok := safecast.IntToUint16(len(buf))
		if !ok {
			return nil, errors.New("ipv4 packet exceeds 65535 bytes")
		}
		packet.SetIPv4TotalLength(buf, totalLen)
		if ipid != nil {
			packet.SetIPv4ID(buf, *ipid)
			*ipid = *ipid + 1
		}
	case packet.IPVersion6:
		payloadLen, ok := safecast.IntToUint16(len(buf) - 40)
		if !ok {
			return nil, errors.New("ipv6 payload exceeds 65535 bytes")
		}
		packet.SetIPv6PayloadLength(buf, payloadLen)
	default:
		return nil, errors.New("unsupported ip version")
	}
	packet.SetTCPSeq(buf, ipHeaderLen, seq)
	packet.SetTCPFlags(buf, ipHeaderLen, flags)

	copy(buf[headerLen:], payload)
	packet.SetTCPChecksumZero(buf, ipHeaderLen)
	if tpl.Meta.IPVersion == packet.IPVersion4 {
		packet.SetIPv4ChecksumZero(buf)
	}

	return &packet.Packet{
		Data:    buf,
		Addr:    tpl.Addr,
		Source:  packet.SourceInjected,
		IfIndex: tpl.IfIndex,
	}, nil
}

func shouldFailOpenIPv6ExtensionHeaders(pkt *packet.Packet) bool {
	if pkt == nil {
		return false
	}
	return pkt.Meta.IPVersion == packet.IPVersion6 && pkt.Meta.IPHeaderLen > 40
}

func (w *worker) canTrimRemainder(st *flow.FlowState) bool {
	if st.Reassembler == nil {
		return false
	}
	return !st.Reassembler.HadOutOfOrder() && !st.Reassembler.HadOverlap()
}

func (w *worker) reinjectTrimmed(ctx context.Context, held []*packet.Packet, baseSeq uint32, windowLen uint32, ipid *uint16) (int, error) {
	sent := 0
	for _, pkt := range held {
		payload := pkt.Payload()
		if len(payload) == 0 {
			continue
		}
		offset := pkt.Meta.Seq - baseSeq
		payloadLen, ok := safecast.IntToUint32(len(payload))
		if !ok {
			return sent, errors.New("payload exceeds uint32")
		}
		end := offset + payloadLen
		if end <= windowLen {
			continue
		}

		trim := uint32(0)
		if offset < windowLen {
			trim = windowLen - offset
		}
		if trim >= payloadLen {
			continue
		}

		newPayload := payload[trim:]
		newSeq := pkt.Meta.Seq + trim

		newPkt, err := buildPacket(pkt, newSeq, newPayload, pkt.Meta.Flags, ipid)
		if err != nil {
			return sent, err
		}
		if err := w.adapter.CalcChecksums(newPkt); err != nil {
			return sent, err
		}
		if err := sendPacket(ctx, w.adapter, newPkt); err != nil {
			return sent, err
		}
		sent++
	}
	return sent, nil
}

func splitFirst(payload []byte, firstLen int, maxPayload int) [][]byte {
	if maxPayload < 1 || len(payload) == 0 {
		return nil
	}
	if firstLen < 1 {
		firstLen = 1
	}
	if firstLen > maxPayload {
		firstLen = maxPayload
	}
	if firstLen >= len(payload) {
		return nil
	}

	segments := make([][]byte, 0, 2)
	segments = append(segments, payload[:firstLen])

	offset := firstLen
	for offset < len(payload) {
		chunk := maxPayload
		remaining := len(payload) - offset
		if chunk > remaining {
			chunk = remaining
		}
		segments = append(segments, payload[offset:offset+chunk])
		offset += chunk
	}
	return segments
}

func clampSegmentPayload(payloadLen int, headerLen int, capPayload int) int {
	if payloadLen < 1 {
		return 0
	}
	if headerLen < 1 || headerLen > maxIPv4TotalLen {
		return 0
	}
	maxPayload := payloadLen
	ipMax := maxIPv4TotalLen - headerLen
	if maxPayload > ipMax {
		maxPayload = ipMax
	}
	if capPayload > 0 && maxPayload > capPayload {
		maxPayload = capPayload
	}
	return maxPayload
}

func chunkPayload(payload []byte, maxPayload int) [][]byte {
	if maxPayload < 1 || len(payload) == 0 {
		return nil
	}
	segments := make([][]byte, 0, (len(payload)+maxPayload-1)/maxPayload)
	offset := 0
	for offset < len(payload) {
		chunk := maxPayload
		remaining := len(payload) - offset
		if chunk > remaining {
			chunk = remaining
		}
		segments = append(segments, payload[offset:offset+chunk])
		offset += chunk
	}
	return segments
}

func (w *worker) noteSplitOK() {
	if w == nil || w.stats == nil {
		return
	}
	w.stats.incSplitOK()
}

func (w *worker) notePressure(reason pressureReason) {
	if w == nil || w.stats == nil {
		return
	}
	w.stats.incPressure(reason)
}

func (w *worker) failOpenWithReason(ctx context.Context, key flow.Key, st *flow.FlowState, reason failOpenReason) error {
	w.noteFailOpenReason(reason)
	return w.failOpen(ctx, key, st)
}

func (w *worker) noteFailOpenReason(reason failOpenReason) {
	if w != nil && w.stats != nil {
		w.stats.incFailOpen(reason)
		switch reason {
		case failOpenReasonHeldBytesLimit:
			w.stats.incPressure(pressureReasonHeldBytesLimit)
		case failOpenReasonHeldPacketsLimit:
			w.stats.incPressure(pressureReasonHeldPacketsLimit)
		case failOpenReasonCollectTimeout:
			w.stats.incPressure(pressureReasonCollectTimeout)
		case failOpenReasonReassemblyBytesLimit:
			w.stats.incPressure(pressureReasonReassemblyBytesLimit)
		}
	}
}

func (w *worker) failOpen(ctx context.Context, key flow.Key, st *flow.FlowState) error {
	for i, pkt := range st.HeldPackets {
		if pkt == nil {
			continue
		}
		if err := sendPacket(ctx, w.adapter, pkt); err != nil {
			w.compactHeldPackets(st)
			return err
		}
		w.consumeHeldPacket(st, i)
	}
	st.State = flow.StatePassThrough
	w.clearCollectingState(st)
	return nil
}

func (w *worker) dropHeld(ctx context.Context, st *flow.FlowState) error {
	for i, pkt := range st.HeldPackets {
		if pkt == nil {
			continue
		}
		if err := dropPacket(ctx, w.adapter, pkt); err != nil {
			w.compactHeldPackets(st)
			return err
		}
		w.consumeHeldPacket(st, i)
	}
	return nil
}

func (w *worker) takeHeldPackets(st *flow.FlowState) []*packet.Packet {
	if st == nil || len(st.HeldPackets) == 0 {
		return nil
	}
	held := st.HeldPackets
	for _, pkt := range held {
		if pkt == nil {
			continue
		}
		w.heldBytes -= int64(len(pkt.Data))
	}
	if w.heldBytes < 0 {
		w.heldBytes = 0
	}
	st.HeldPackets = nil
	st.Template = nil
	return held
}

func (w *worker) releaseDetachedPackets(pkts []*packet.Packet) {
	for _, pkt := range pkts {
		if pkt == nil {
			continue
		}
		pkt.Release()
	}
}

func (w *worker) failOpenDetachedPackets(ctx context.Context, pkts []*packet.Packet) error {
	for i, pkt := range pkts {
		if pkt == nil {
			continue
		}
		if err := sendPacket(ctx, w.adapter, pkt); err != nil {
			w.releaseDetachedPackets(pkts[i:])
			return err
		}
	}
	return nil
}

func (w *worker) dropDetachedPackets(ctx context.Context, pkts []*packet.Packet) error {
	for _, pkt := range pkts {
		if pkt == nil {
			continue
		}
		if err := w.adapter.Drop(ctx, pkt); err != nil {
			return err
		}
	}
	return nil
}

func (w *worker) handleDetachedInjectError(ctx context.Context, st *flow.FlowState, held []*packet.Packet, sentAny bool) error {
	w.noteFailOpenReason(failOpenReasonInjectError)
	if !sentAny {
		err := w.failOpenDetachedPackets(ctx, held)
		st.State = flow.StatePassThrough
		w.clearCollectingState(st)
		return err
	}
	w.releaseDetachedPackets(held)
	st.State = flow.StatePassThrough
	w.clearCollectingState(st)
	return nil
}

func (w *worker) consumeHeldPacket(st *flow.FlowState, idx int) {
	if st == nil || idx < 0 || idx >= len(st.HeldPackets) {
		return
	}
	pkt := st.HeldPackets[idx]
	if pkt == nil {
		return
	}
	w.heldBytes -= int64(len(pkt.Data))
	if w.heldBytes < 0 {
		w.heldBytes = 0
	}
	st.HeldPackets[idx] = nil
	if st.Template == pkt {
		st.Template = nil
	}
}

func (w *worker) compactHeldPackets(st *flow.FlowState) {
	if st == nil || len(st.HeldPackets) == 0 {
		return
	}
	kept := st.HeldPackets[:0]
	for _, pkt := range st.HeldPackets {
		if pkt != nil {
			kept = append(kept, pkt)
		}
	}
	st.HeldPackets = kept
	if st.Template == nil && len(kept) > 0 {
		st.Template = kept[len(kept)-1]
	}
}

func (w *worker) clearCollectingState(st *flow.FlowState) {
	if st == nil {
		return
	}
	for _, pkt := range st.HeldPackets {
		if pkt == nil {
			continue
		}
		w.heldBytes -= int64(len(pkt.Data))
	}
	if w.heldBytes < 0 {
		w.heldBytes = 0
	}
	if st.Reassembler != nil {
		w.reassemblyBytes -= int64(st.Reassembler.TotalBytes())
		if w.reassemblyBytes < 0 {
			w.reassemblyBytes = 0
		}
	}
	st.HeldPackets = nil
	st.Template = nil
	st.Reassembler = nil
}

func (w *worker) gc(ctx context.Context) error {
	idle := 30 * time.Second
	collectTimeout := 250 * time.Millisecond
	if cfg := w.cfg.Load(); cfg != nil {
		if cfg.FlowIdleTimeout > 0 {
			idle = cfg.FlowIdleTimeout
		}
		if cfg.CollectTimeout > 0 {
			collectTimeout = cfg.CollectTimeout
		}
	}

	now := time.Now()
	var firstErr error
	w.flows.Range(func(key flow.Key, st *flow.FlowState) {
		if st == nil {
			return
		}
		if st.State == flow.StateCollecting && len(st.HeldPackets) > 0 && !st.CollectStart.IsZero() && now.Sub(st.CollectStart) > collectTimeout {
			if err := w.failOpenWithReason(ctx, key, st, failOpenReasonCollectTimeout); err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			st.LastActive = now
			return
		}
		if now.Sub(st.LastActive) <= idle {
			return
		}
		if st.State == flow.StateCollecting && len(st.HeldPackets) > 0 {
			if err := w.failOpenWithReason(ctx, key, st, failOpenReasonIdleGC); err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return
			}
		}
		w.flows.Delete(key)
	})
	return firstErr
}

func (w *worker) shutdownFailOpen(ctx context.Context) error {
	maxPackets := 200000
	if cfg := w.cfg.Load(); cfg != nil && cfg.ShutdownFailOpenMaxPackets > 0 {
		maxPackets = cfg.ShutdownFailOpenMaxPackets
	}
	flushed := 0
	reinject := func(pkt *packet.Packet) error {
		if pkt == nil {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if maxPackets > 0 && flushed >= maxPackets {
			return ErrShutdownFailOpenLimitReached
		}
		if err := sendPacket(ctx, w.adapter, pkt); err != nil {
			return err
		}
		flushed++
		return nil
	}

	// 1) Release any held packets for flows still collecting (or any flow that
	//    has a non-empty HeldPackets slice).
	stop := false
	var stopErr error
	w.flows.Range(func(key flow.Key, st *flow.FlowState) {
		if stop {
			return
		}
		if st == nil || len(st.HeldPackets) == 0 {
			return
		}
		for i, pkt := range st.HeldPackets {
			if pkt == nil {
				continue
			}
			if err := reinject(pkt); err != nil {
				w.compactHeldPackets(st)
				stop = true
				stopErr = err
				return
			}
			w.consumeHeldPacket(st, i)
		}
		if len(st.HeldPackets) == 0 {
			w.clearCollectingState(st)
		}
	})
	if stopErr != nil {
		return stopErr
	}

	// 2) Drain any queued-but-unprocessed packets and pass them through.
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if maxPackets > 0 && flushed >= maxPackets {
			return ErrShutdownFailOpenLimitReached
		}
		select {
		case pkt, ok := <-w.in:
			if !ok {
				return nil
			}
			if pkt == nil {
				continue
			}
			if err := reinject(pkt); err != nil {
				return err
			}
		default:
			return nil
		}
	}
}
