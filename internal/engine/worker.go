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
	"fk-gov/internal/tls"
)

const maxIPv4TotalLen = 0xffff

var ErrShutdownFailOpenLimitReached = errors.New("shutdown fail-open packet limit reached")

type worker struct {
	id      int
	cfg     atomic.Pointer[Config]
	adapter adapter.Adapter
	in      chan *packet.Packet
	touch   chan flow.Key
	flows   *flow.Table

	heldBytes       int64
	reassemblyBytes int64
}

func newWorker(id int, cfg Config, ad adapter.Adapter) *worker {
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
		in:      make(chan *packet.Packet, cfg.WorkerQueueSize),
		touch:   make(chan flow.Key, cfg.WorkerQueueSize),
		flows:   flow.NewTable(),
	}
	cfgCopy := cfg
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

func (w *worker) touchFlow(key flow.Key) {
	select {
	case w.touch <- key:
	default:
	}
}

func (w *worker) setConfig(cfg Config) {
	cfgCopy := cfg
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

		// FIN/RST often have no payload; ensure they still clean up flow state
		// promptly even when payloadless packets are fast-pathed.
		if len(payload) == 0 && (pkt.HasFlag(packet.TCPFlagRST) || pkt.HasFlag(packet.TCPFlagFIN)) {
			if st.State == flow.StateCollecting {
				if err := w.failOpen(ctx, key, st); err != nil {
					return err
				}
			}
			if err := w.adapter.Send(ctx, pkt); err != nil {
				return err
			}
			w.flows.Delete(key)
			return nil
		}

		if st.State == flow.StateInjected || st.State == flow.StatePassThrough {
			return w.adapter.Send(ctx, pkt)
		}
		if len(payload) == 0 {
			return w.adapter.Send(ctx, pkt)
		}

		if st.State == flow.StateNew {
			st.BaseSeq = pkt.Meta.Seq
			st.Reassembler = reassembly.New(st.BaseSeq, uint32(cfg.MaxBufferBytes))
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
				if err := w.failOpen(ctx, key, st); err != nil {
					return err
				}
				return w.adapter.Send(ctx, pkt)
			}
		}
		st.HeldPackets = append(st.HeldPackets, pkt)
		w.heldBytes += int64(len(pkt.Data))
		if len(st.HeldPackets) >= cfg.MaxHeldPackets {
			return w.failOpen(ctx, key, st)
		}
		if now.Sub(st.CollectStart) > cfg.CollectTimeout {
			return w.failOpen(ctx, key, st)
		}
		if st.Reassembler == nil {
			return w.failOpen(ctx, key, st)
		}
		before := int64(st.Reassembler.TotalBytes())
		err := st.Reassembler.Push(pkt.Meta.Seq, payload)
		after := int64(st.Reassembler.TotalBytes())
		w.reassemblyBytes += after - before
		if w.reassemblyBytes < 0 {
			w.reassemblyBytes = 0
		}
		if err != nil {
			return w.failOpen(ctx, key, st)
		}
		if cfg.MaxReassemblyBytesPerWorker > 0 && w.reassemblyBytes > int64(cfg.MaxReassemblyBytesPerWorker) {
			return w.failOpen(ctx, key, st)
		}

		if pkt.HasFlag(packet.TCPFlagSYN) {
			return w.failOpen(ctx, key, st)
		}

		if pkt.HasFlag(packet.TCPFlagRST) {
			if err := w.failOpen(ctx, key, st); err != nil {
				return err
			}
			w.flows.Delete(key)
			return nil
		}

		if pkt.HasFlag(packet.TCPFlagFIN) {
			if err := w.failOpen(ctx, key, st); err != nil {
				return err
			}
			w.flows.Delete(key)
			return nil
		}

		if cfg.SplitMode == SplitModeImmediate {
			return w.trySplitImmediate(ctx, key, st)
		}

		if cfg.SplitMode == SplitModeTLSHello {
			return w.trySplitTLSHello(ctx, key, st)
		}

		return nil
	}

	// No existing state: fail-open for payloadless packets (no flow creation).
	if len(payload) == 0 {
		return w.adapter.Send(ctx, pkt)
	}

	// DoS guard: bound the number of tracked flows per worker.
	if cfg.MaxFlowsPerWorker > 0 && w.flows.Len() >= cfg.MaxFlowsPerWorker {
		return w.adapter.Send(ctx, pkt)
	}

	// Best-effort budget checks before creating per-flow state.
	if cfg.MaxHeldBytesPerWorker > 0 {
		need := int64(len(pkt.Data))
		limit := int64(cfg.MaxHeldBytesPerWorker)
		if w.heldBytes+need > limit {
			return w.adapter.Send(ctx, pkt)
		}
	}
	if cfg.MaxReassemblyBytesPerWorker > 0 {
		need := int64(len(payload))
		limit := int64(cfg.MaxReassemblyBytesPerWorker)
		if w.reassemblyBytes+need > limit {
			return w.adapter.Send(ctx, pkt)
		}
	}

	st := w.flows.GetOrCreate(key, now)
	st.LastActive = now

	if st.State == flow.StateNew {
		st.BaseSeq = pkt.Meta.Seq
		st.Reassembler = reassembly.New(st.BaseSeq, uint32(cfg.MaxBufferBytes))
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
			if err := w.failOpen(ctx, key, st); err != nil {
				return err
			}
			return w.adapter.Send(ctx, pkt)
		}
	}
	st.HeldPackets = append(st.HeldPackets, pkt)
	w.heldBytes += int64(len(pkt.Data))
	if len(st.HeldPackets) >= cfg.MaxHeldPackets {
		return w.failOpen(ctx, key, st)
	}
	if now.Sub(st.CollectStart) > cfg.CollectTimeout {
		return w.failOpen(ctx, key, st)
	}
	before := int64(st.Reassembler.TotalBytes())
	err := st.Reassembler.Push(pkt.Meta.Seq, payload)
	after := int64(st.Reassembler.TotalBytes())
	w.reassemblyBytes += after - before
	if w.reassemblyBytes < 0 {
		w.reassemblyBytes = 0
	}
	if err != nil {
		return w.failOpen(ctx, key, st)
	}
	if cfg.MaxReassemblyBytesPerWorker > 0 && w.reassemblyBytes > int64(cfg.MaxReassemblyBytesPerWorker) {
		return w.failOpen(ctx, key, st)
	}

	if pkt.HasFlag(packet.TCPFlagSYN) || pkt.HasFlag(packet.TCPFlagRST) {
		if err := w.failOpen(ctx, key, st); err != nil {
			return err
		}
		if pkt.HasFlag(packet.TCPFlagRST) {
			w.flows.Delete(key)
		}
		return nil
	}

	if pkt.HasFlag(packet.TCPFlagFIN) {
		if err := w.failOpen(ctx, key, st); err != nil {
			return err
		}
		w.flows.Delete(key)
		return nil
	}

	if cfg.SplitMode == SplitModeImmediate {
		return w.trySplitImmediate(ctx, key, st)
	}

	if cfg.SplitMode == SplitModeTLSHello {
		return w.trySplitTLSHello(ctx, key, st)
	}

	return nil
}

func (w *worker) trySplitImmediate(ctx context.Context, key flow.Key, st *flow.FlowState) error {
	if st.FirstPayloadLen <= 0 {
		return nil
	}
	if st.Reassembler == nil {
		return w.failOpen(ctx, key, st)
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
		return w.failOpen(ctx, key, st)
	}
	contig := st.Reassembler.Contiguous()
	recordLen, result := tls.DetectClientHelloRecord(contig)
	if result == tls.ResultNeedMore {
		return nil
	}
	if result == tls.ResultMismatch {
		return w.failOpen(ctx, key, st)
	}

	need := 5 + int(recordLen)
	if need > cfg.MaxBufferBytes {
		return w.failOpen(ctx, key, st)
	}
	if len(contig) < need {
		return nil
	}

	return w.injectWindow(ctx, key, st, need)
}

func (w *worker) injectWindow(ctx context.Context, key flow.Key, st *flow.FlowState, windowLen int) error {
	cfg := w.cfg.Load()
	if cfg == nil {
		return errors.New("worker config is nil")
	}

	if windowLen < 1 {
		return w.failOpen(ctx, key, st)
	}
	contig := st.Reassembler.Contiguous()
	if len(contig) < windowLen {
		return nil
	}
	tpl := st.Template
	if tpl == nil {
		return w.failOpen(ctx, key, st)
	}
	maxPayload := len(tpl.Payload())
	headerLen := tpl.Meta.IPHeaderLen + tpl.Meta.TCPHeaderLen
	maxPayload = clampSegmentPayload(maxPayload, headerLen, cfg.MaxSegmentPayload)
	if maxPayload < 1 {
		return w.failOpen(ctx, key, st)
	}

	window := contig[:windowLen]
	remainder := contig[windowLen:]

	splitSegs := splitFirst(window, cfg.SplitChunk, maxPayload)
	if len(splitSegs) < 2 {
		return w.failOpen(ctx, key, st)
	}

	flags := tpl.Meta.Flags
	flagsNoPshFin := flags &^ (packet.TCPFlagPSH | packet.TCPFlagFIN)
	splitLastFlags := flags
	if len(remainder) > 0 {
		splitLastFlags = flagsNoPshFin
	}

	ipid := packet.IPv4ID(tpl.Data)
	if err := w.sendSegments(ctx, tpl, st.BaseSeq, splitSegs, flagsNoPshFin, splitLastFlags, &ipid); err != nil {
		return w.failOpen(ctx, key, st)
	}

	if len(remainder) > 0 {
		if w.canTrimRemainder(st) {
			if err := w.reinjectTrimmed(ctx, st, uint32(windowLen), &ipid); err != nil {
				return w.failOpen(ctx, key, st)
			}
		} else {
			remSegs := chunkPayload(remainder, maxPayload)
			if err := w.sendSegments(ctx, tpl, st.BaseSeq+uint32(windowLen), remSegs, flagsNoPshFin, flags, &ipid); err != nil {
				return w.failOpen(ctx, key, st)
			}
		}
	}

	if err := w.dropHeld(ctx, st); err != nil {
		return err
	}

	st.State = flow.StateInjected
	w.clearCollectingState(st)
	st.Processed = true
	return nil
}

func (w *worker) sendSegments(ctx context.Context, tpl *packet.Packet, baseSeq uint32, segments [][]byte, flags uint8, lastFlags uint8, ipid *uint16) error {
	offset := 0
	for i, segPayload := range segments {
		if len(segPayload) == 0 {
			continue
		}
		segFlags := flags
		if i == len(segments)-1 {
			segFlags = lastFlags
		}
		newPkt, err := buildPacket(tpl, baseSeq+uint32(offset), segPayload, segFlags, ipid)
		if err != nil {
			return err
		}
		if err := w.adapter.CalcChecksums(newPkt); err != nil {
			return err
		}
		if err := w.adapter.Send(ctx, newPkt); err != nil {
			return err
		}
		offset += len(segPayload)
	}
	return nil
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

	packet.SetIPv4TotalLength(buf, uint16(len(buf)))
	if ipid != nil {
		packet.SetIPv4ID(buf, *ipid)
		*ipid++
	}
	packet.SetTCPSeq(buf, ipHeaderLen, seq)
	packet.SetTCPFlags(buf, ipHeaderLen, flags)

	copy(buf[headerLen:], payload)
	packet.SetIPv4ChecksumZero(buf)
	packet.SetTCPChecksumZero(buf, ipHeaderLen)

	return &packet.Packet{
		Data:   buf,
		Addr:   tpl.Addr,
		Source: packet.SourceInjected,
	}, nil
}

func (w *worker) canTrimRemainder(st *flow.FlowState) bool {
	if st.Reassembler == nil {
		return false
	}
	return !st.Reassembler.HadOutOfOrder() && !st.Reassembler.HadOverlap()
}

func (w *worker) reinjectTrimmed(ctx context.Context, st *flow.FlowState, windowLen uint32, ipid *uint16) error {
	for _, pkt := range st.HeldPackets {
		payload := pkt.Payload()
		if len(payload) == 0 {
			continue
		}
		offset := pkt.Meta.Seq - st.BaseSeq
		end := offset + uint32(len(payload))
		if end <= windowLen {
			continue
		}

		trim := uint32(0)
		if offset < windowLen {
			trim = windowLen - offset
		}
		if trim >= uint32(len(payload)) {
			continue
		}

		newPayload := payload[trim:]
		newSeq := pkt.Meta.Seq + trim

		newPkt, err := buildPacket(pkt, newSeq, newPayload, pkt.Meta.Flags, ipid)
		if err != nil {
			return err
		}
		if err := w.adapter.CalcChecksums(newPkt); err != nil {
			return err
		}
		if err := w.adapter.Send(ctx, newPkt); err != nil {
			return err
		}
	}
	return nil
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

func (w *worker) failOpen(ctx context.Context, key flow.Key, st *flow.FlowState) error {
	for _, pkt := range st.HeldPackets {
		if err := w.adapter.Send(ctx, pkt); err != nil {
			return err
		}
	}
	st.State = flow.StatePassThrough
	w.clearCollectingState(st)
	return nil
}

func (w *worker) dropHeld(ctx context.Context, st *flow.FlowState) error {
	for _, pkt := range st.HeldPackets {
		if err := w.adapter.Drop(ctx, pkt); err != nil {
			return err
		}
	}
	return nil
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
	if cfg := w.cfg.Load(); cfg != nil && cfg.FlowIdleTimeout > 0 {
		idle = cfg.FlowIdleTimeout
	}

	now := time.Now()
	var firstErr error
	w.flows.Range(func(key flow.Key, st *flow.FlowState) {
		if now.Sub(st.LastActive) <= idle {
			return
		}
		if st.State == flow.StateCollecting && len(st.HeldPackets) > 0 {
			if err := w.failOpen(ctx, key, st); err != nil {
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
	var firstErr error

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
		if err := w.adapter.Send(ctx, pkt); err != nil {
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
		for _, pkt := range st.HeldPackets {
			if err := reinject(pkt); err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrShutdownFailOpenLimitReached) {
					stop = true
					stopErr = err
					return
				}
				if firstErr == nil {
					firstErr = err
				}
			}
		}
	})
	if stopErr != nil {
		if firstErr != nil {
			return errors.Join(firstErr, stopErr)
		}
		return stopErr
	}

	// 2) Drain any queued-but-unprocessed packets and pass them through.
	for {
		if err := ctx.Err(); err != nil {
			if firstErr != nil {
				return errors.Join(firstErr, err)
			}
			return err
		}
		if maxPackets > 0 && flushed >= maxPackets {
			if firstErr != nil {
				return errors.Join(firstErr, ErrShutdownFailOpenLimitReached)
			}
			return ErrShutdownFailOpenLimitReached
		}
		select {
		case pkt, ok := <-w.in:
			if !ok {
				return firstErr
			}
			if pkt == nil {
				continue
			}
			if err := reinject(pkt); err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrShutdownFailOpenLimitReached) {
					if firstErr != nil {
						return errors.Join(firstErr, err)
					}
					return err
				}
				if firstErr == nil {
					firstErr = err
				}
			}
		default:
			return firstErr
		}
	}
}
