# Linux Design

This document covers the Linux-specific path. Shared engine behavior lives in
[`DESIGN_COMMON.md`](DESIGN_COMMON.md).

## Data Path

`NFQUEUE -> decoder -> flow shard -> reassembly -> TLS check -> split plan -> raw socket send`

Non-target traffic is accepted immediately. Target flows are held only until the
split decision is complete.

## Packet Interception

- The runtime binds NFQUEUE on IPv4 outbound TCP/443.
- Rule helpers install a dedicated tagged rule set so cleanup touches only
  `gov-pass` rules.
- Reinjected packets carry `SO_MARK` so they bypass NFQUEUE on the return path.

## Operational Defaults

- queue number: `100`
- queue max length: `4096`
- copy range: full packet
- queue bypass enabled in firewall rules
- automatic GRO/GSO/TSO disable on the detected egress interface

## Split And Verdict Policy

- While collecting, packets are held and no accept verdict is returned yet.
- On split-ready, originals are dropped and split segments are injected.
- On failure, held packets are accepted in original order.
- After injection, later packets in the same flow are pass-through only.

## Safety Rules

Fail open on:

- invalid TLS header
- timeout while collecting
- buffer or memory guard limits
- retransmission state that cannot be merged safely
- IP fragmentation or malformed headers

## Shutdown

- Workers fail open any held flows during shutdown.
- Queued-but-unprocessed packets are drained as pass-through.
- Shutdown is bounded by timeout and packet count so the process does not hang
  under load.
