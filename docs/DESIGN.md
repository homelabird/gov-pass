# Detailed Design - WinDivert Split-Only TLS ClientHello (Go)

This document focuses on WinDivert-specific implementation details. Shared behavior is documented in
[DESIGN_COMMON.md](DESIGN_COMMON.md).

## Platform-specific notes

- WinDivert filter defaults and queue tuning.
- Packet receive/send path (WinDivert API behavior).
- ACK-only fast path and checksum/header handling details.

## Defaults (current)

- split-mode: tls-hello
- tls-completion: first TLS record only
- split-chunk: 5
- collect-timeout: 250ms
- max-buffer: 64KB
- max-held-pkts: 32
- max-seg-payload: 1460 (0 = unlimited)
- workers: NumCPU
- worker-queue-size: 1024
- max-flows-per-worker: 4096
- max-reassembly-bytes-per-worker: 64MB
- max-held-bytes-per-worker: 64MB
- shutdown-fail-open-timeout: 5s
- shutdown-fail-open-max-pkts: 200000
- adapter-flush-timeout: 2s

## Architecture overview

WinDivert -> Decoder -> Flow Manager -> Reassembler -> TLS Inspector
-> Split Plan -> Injector -> WinDivert send

Non-target and completed flows bypass reassembly and are sent as-is.
ACK-only packets are fast-pathed and are not enqueued into worker queues.
FIN/RST packets go through workers so flow state is cleaned up promptly.

## WinDivert adapter

- Filter: outbound and ip and tcp and tcp.DstPort == 443
- Open with queue parameters (MaxLen/MaxTime/MaxSize) tuned for latency.
- Receive loop reads packet bytes + address.
- Send uses the original address with updated headers and checksums.
- Do not use WINDIVERT_FLAG_SNIFF.

## Flow manager

### Key

FlowKey = (srcIP, dstIP, srcPort, dstPort, proto)

### State

NEW -> COLLECTING -> SPLIT_READY -> INJECTED -> PASS_THROUGH -> CLOSED

### Transitions

- NEW -> COLLECTING: first payload observed
- COLLECTING -> SPLIT_READY: tls-hello criteria met
- COLLECTING -> PASS_THROUGH: timeout, parse failure, buffer overflow
- SPLIT_READY -> INJECTED: split segments sent
- INJECTED -> PASS_THROUGH: always after injection
- Any -> CLOSED: FIN/RST or idle timeout

### Held packets

While in NEW/COLLECTING, hold original packets in a bounded queue:
- On split: drop held packets and inject split segments instead.
- On fail-open: reinject held packets in original order.

## Reassembly

We only need contiguous bytes from the start of the first payload range.
The reassembler:
- Tracks seq ranges and de-duplicates overlap.
- Maintains a contiguous "frontier" from baseSeq.
- Caps total buffered bytes at max-buffer.

Suggested structure:
- map[seq]payload for fragments
- list of merged ranges (start, end)
- contiguousLen computed from baseSeq

Sequence math must handle 32-bit wrap-around.

## TLS detection (tls-hello)

Trigger once we have 6 contiguous bytes:

1. contentType == 0x16
2. version in 0x0301..0x0304
3. handshakeType == 0x01

Then read recordLen (2 bytes) and wait until:
contiguousLen >= 5 + recordLen

If recordLen > max-buffer or collect-timeout expires, fail-open.

Pseudo:

```text
if contiguousLen >= 6:
  if type!=0x16 or version not ok or hsType!=0x01:
    fail-open
  recordLen = u16(payload[3:5])
  if recordLen > maxBuffer: fail-open
  if contiguousLen >= 5 + recordLen: split-ready
```

## Split plan

Window to split:
- tls-hello: first TLS record (bytes 0..5+recordLen)
- immediate: first payload packet only

Split segments:
- First segment size = split-chunk (default 5)
- Remaining bytes in one segment (or multiple if needed)
- Cap segment payload size to max-seg-payload and IPv4 total length

Segment build rules:
- seq = baseSeq + offset
- Copy TCP options from template packet
- Preserve ack, window, and flags; set PSH/FIN only on last segment
- Update IP total length and TCP data offset
- Recompute checksums via WinDivert helper

## Fail-open triggers

- TLS header mismatch
- Buffer exceeds max-buffer
- Held packets exceed max-held-pkts
- Worker flow cap exceeds max-flows-per-worker
- Worker held bytes cap exceeds max-held-bytes-per-worker
- Worker reassembly bytes cap exceeds max-reassembly-bytes-per-worker
- collect-timeout exceeded
- IP fragmentation or invalid TCP header
- Any decode error

Fail-open behavior: reinject held packets in original order and set flow
state to PASS_THROUGH.

## Retransmission and duplicates

- While COLLECTING, merge duplicate/overlap segments into the reassembler.
- If flow already INJECTED or PASS_THROUGH, always pass-through.
- Do not attempt to suppress retransmissions beyond normal flow state.

## Concurrency model (Go)

Selected model: sharded workers.
- Single recv goroutine hashes FlowKey to N workers (default N=NumCPU).
- Each worker owns its flow shard and reassembly state.
- Per-worker send queue preserves in-order injection.
- Optional single-worker mode remains for debug or low-throughput use.

Use sync.Pool for packet buffers and avoid per-packet allocations.

## Shutdown behavior

On shutdown or worker exit:
- Workers fail-open any held packets (reinject originals) and drain any queued-but-unprocessed packets (pass-through).
- Shutdown draining is bounded by a timeout and max packet count to prevent Stop from hanging forever under load.
- After workers stop, the adapter performs a best-effort flush of adapter-level pending packets before the handle is closed.

## Data structures (suggested)

```text
type FlowKey struct {
  SrcIP, DstIP uint32
  SrcPort, DstPort uint16
  Proto uint8
}

type FlowState struct {
  State           enum
  BaseSeq         uint32
  LastActiveNs    int64
  Processed       bool
  HeldPackets     []Packet
  Reassembler     *ReassemblyBuffer
}

type ReassemblyBuffer struct {
  BaseSeq     uint32
  ContigLen   uint32
  Fragments   map[uint32][]byte
  TotalBytes  uint32
}
```

## Logging and metrics

- splits_ok, splits_fail_open, tls_mismatch, buffer_overflow
- reasm_bytes, held_pkts, flow_count
- sample logs for flow transitions and split decisions

## Testing and validation

- Unit tests for reassembly: gap, overlap, wrap-around
- TLS detection: valid and invalid headers
- Integration: pcap replay and handshake verification
- Compare original vs reinjected packets (checksums, flags)
