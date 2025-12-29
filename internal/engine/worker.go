package engine

import (
	"context"
	"errors"
	"time"

	"fk-gov/internal/adapter"
	"fk-gov/internal/flow"
	"fk-gov/internal/packet"
	"fk-gov/internal/reassembly"
	"fk-gov/internal/tls"
)

type worker struct {
	id      int
	cfg     Config
	adapter adapter.Adapter
	in      chan *packet.Packet
	flows   *flow.Table
}

func newWorker(id int, cfg Config, ad adapter.Adapter) *worker {
	if cfg.WorkerQueueSize < 1 {
		cfg.WorkerQueueSize = 1024
	}
	return &worker{
		id:      id,
		cfg:     cfg,
		adapter: ad,
		in:      make(chan *packet.Packet, cfg.WorkerQueueSize),
		flows:   flow.NewTable(),
	}
}

func (w *worker) enqueue(ctx context.Context, pkt *packet.Packet) error {
	select {
	case w.in <- pkt:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *worker) close() {
	close(w.in)
}

func (w *worker) run(ctx context.Context) error {
	ticker := time.NewTicker(w.cfg.GCInterval)
	defer ticker.Stop()

	for {
		select {
		case pkt, ok := <-w.in:
			if !ok {
				return nil
			}
			if err := w.handlePacket(ctx, pkt); err != nil {
				return err
			}
		case <-ticker.C:
			if err := w.gc(ctx); err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (w *worker) handlePacket(ctx context.Context, pkt *packet.Packet) error {
	now := time.Now()
	key := flow.KeyFromMeta(pkt.Meta)
	st := w.flows.GetOrCreate(key, now)
	st.LastActive = now

	if st.State == flow.StateInjected || st.State == flow.StatePassThrough {
		return w.adapter.Send(ctx, pkt)
	}

	payload := pkt.Payload()
	if len(payload) == 0 {
		return w.adapter.Send(ctx, pkt)
	}

	if st.State == flow.StateNew {
		st.BaseSeq = pkt.Meta.Seq
		st.Reassembler = reassembly.New(st.BaseSeq, uint32(w.cfg.MaxBufferBytes))
		st.State = flow.StateCollecting
		st.CollectStart = now
		st.FirstPayloadLen = len(payload)
		st.Template = pkt
	} else {
		st.Template = pkt
	}

	st.HeldPackets = append(st.HeldPackets, pkt)
	if len(st.HeldPackets) > w.cfg.MaxHeldPackets {
		return w.failOpen(ctx, key, st)
	}
	if now.Sub(st.CollectStart) > w.cfg.CollectTimeout {
		return w.failOpen(ctx, key, st)
	}
	if err := st.Reassembler.Push(pkt.Meta.Seq, payload); err != nil {
		return w.failOpen(ctx, key, st)
	}

	if pkt.HasFlag(packet.TCPFlagSYN) || pkt.HasFlag(packet.TCPFlagRST) {
		return w.failOpen(ctx, key, st)
	}

	if pkt.HasFlag(packet.TCPFlagFIN) {
		if err := w.failOpen(ctx, key, st); err != nil {
			return err
		}
		w.flows.Delete(key)
		return nil
	}

	if w.cfg.SplitMode == SplitModeImmediate {
		return w.trySplitImmediate(ctx, key, st)
	}

	if w.cfg.SplitMode == SplitModeTLSHello {
		return w.trySplitTLSHello(ctx, key, st)
	}

	return nil
}

func (w *worker) trySplitImmediate(ctx context.Context, key flow.Key, st *flow.FlowState) error {
	if st.FirstPayloadLen <= 0 {
		return nil
	}
	contig := st.Reassembler.Contiguous()
	if len(contig) < st.FirstPayloadLen {
		return nil
	}
	return w.injectWindow(ctx, key, st, st.FirstPayloadLen)
}

func (w *worker) trySplitTLSHello(ctx context.Context, key flow.Key, st *flow.FlowState) error {
	contig := st.Reassembler.Contiguous()
	recordLen, result := tls.DetectClientHelloRecord(contig)
	if result == tls.ResultNeedMore {
		return nil
	}
	if result == tls.ResultMismatch {
		return w.failOpen(ctx, key, st)
	}

	need := 5 + int(recordLen)
	if need > w.cfg.MaxBufferBytes {
		return w.failOpen(ctx, key, st)
	}
	if len(contig) < need {
		return nil
	}

	return w.injectWindow(ctx, key, st, need)
}

func (w *worker) injectWindow(ctx context.Context, key flow.Key, st *flow.FlowState, windowLen int) error {
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
	if maxPayload < 1 {
		return w.failOpen(ctx, key, st)
	}

	window := contig[:windowLen]
	remainder := contig[windowLen:]

	splitSegs := splitFirst(window, w.cfg.SplitChunk, maxPayload)
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
	st.HeldPackets = nil
	st.Template = nil
	st.Reassembler = nil
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
		Data: buf,
		Addr: tpl.Addr,
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
	st.State = flow.StatePassThrough
	for _, pkt := range st.HeldPackets {
		if err := w.adapter.Send(ctx, pkt); err != nil {
			return err
		}
	}
	st.HeldPackets = nil
	st.Template = nil
	st.Reassembler = nil
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

func (w *worker) gc(ctx context.Context) error {
	now := time.Now()
	var firstErr error
	w.flows.Range(func(key flow.Key, st *flow.FlowState) {
		if now.Sub(st.LastActive) <= w.cfg.FlowIdleTimeout {
			return
		}
		if st.State == flow.StateCollecting && len(st.HeldPackets) > 0 {
			if err := w.failOpen(ctx, key, st); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		w.flows.Delete(key)
	})
	return firstErr
}
