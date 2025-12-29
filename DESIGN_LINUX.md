# Detailed Design - Linux NFQUEUE Split-Only TLS ClientHello (Go)

## Goals

- Intercept outbound IPv4 TCP dst port 443 on Linux using NFQUEUE.
- Perform "split only" on the first ClientHello per flow.
- Fail-open on any error, timeout, or buffer pressure.
- Keep latency and memory overhead low.

## Non-goals

- No payload mutation, fake packets, TTL tricks, or reordering.
- No IPv6 in this phase.
- No full TLS parsing beyond record header + handshake type.

## Architecture overview

Netfilter (NFQUEUE) -> Decoder -> Flow Manager -> Reassembler -> TLS Inspector
-> Split Plan -> Injector -> Raw Socket send

Non-target and completed flows are immediately accepted.

## NFQUEUE adapter

Recommended defaults:
- queue-num: 100 (configurable)
- copy-range: 0xffff (full packet)
- queue-maxlen: 4096 (configurable)
- fail-open: iptables/nftables `--queue-bypass`

NFQUEUE responsibilities:
- Bind to IPv4 (AF_INET)
- Receive packets and metadata (id, hook, indev/outdev)
- Provide verdicts: accept or drop

## Rule installation (iptables)

Use mangle OUTPUT so we see post-routing packets:

```bash
# Mark reinjected packets to bypass NFQUEUE
iptables -t mangle -A OUTPUT -m mark --mark 0x1/0x1 -j RETURN

# Optional: exclude loopback
iptables -t mangle -A OUTPUT -o lo -j RETURN

# Queue outbound TCP 443
iptables -t mangle -A OUTPUT -p tcp --dport 443 -j NFQUEUE --queue-num 100 --queue-bypass
```

For nftables, use a similar rule set with `queue num 100 bypass`.

## Injection strategy (raw socket)

- Use a raw socket (AF_INET, SOCK_RAW, IPPROTO_RAW) with IP_HDRINCL.
- Send split segments with:
  - Original IP/TCP headers, updated seq/len/checksum
  - TCP flags preserved; PSH/FIN only on last segment of injection
- Set SO_MARK to 0x1 so reinjected packets bypass NFQUEUE.

## Flow manager

Same as Windows:

NEW -> COLLECTING -> SPLIT_READY -> INJECTED -> PASS_THROUGH -> CLOSED

Held packets are kept only until split decision.

## Reassembly and TLS detection

Same logic as Windows:

Trigger once we have 6 contiguous bytes:
1. contentType == 0x16
2. version in 0x0301..0x0304
3. handshakeType == 0x01

Wait for full first TLS record (5 + recordLen), then split.

## Split plan

- Split window: first TLS record only
- First segment size = split-chunk (default 5)
- Remaining bytes in one or more segments
- seq = baseSeq + offset
- Recompute checksums (custom helper on Linux)

## Fail-open behavior

On any error, timeout, or buffer pressure:
- Verdict accept all held packets
- Mark flow PASS_THROUGH

## Concurrency model

Same sharded worker model as Windows:
- Single NFQUEUE recv loop
- Hash FlowKey to worker shard
- Per-worker in-order injection

## Linux-specific considerations

- Packet offload (GSO/TSO): verify if NFQUEUE sees large segments.
  If so, document or disable offload for testing.
- Kernel versions: prefer 5.x+ with stable NFQUEUE behavior.
- Permissions: root or setcap for CAP_NET_ADMIN + CAP_NET_RAW.

## Open questions

- Default queue-num and queue-maxlen for throughput vs latency
- nftables vs iptables default (prefer nftables on modern distros)
- Whether to expose rule management in CLI or external scripts only
