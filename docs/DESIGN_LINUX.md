# Detailed Design - Linux NFQUEUE Split-Only TLS ClientHello (Go)

This doc follows the shared logic in [DESIGN_COMMON.md](DESIGN_COMMON.md).
For platform-specific behavior, this file focuses on the NFQUEUE adapter, verdict
handling, and Linux-specific offload/reinjection details.

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

## Packet handling and verdicts

- Non-target packets: immediately accept.
- Target flow, still collecting: hold packets, do not accept yet.
- Split-ready: drop held originals and send split segments.
- Fail-open: accept held packets in original order.
- After injection: accept all future packets in the flow.

## Rule installation (iptables)

Use mangle OUTPUT so we see post-routing packets:

```bash
# Create a dedicated chain so we only manage our own rules
iptables -t mangle -N GOVPASS_OUTPUT || true
iptables -t mangle -F GOVPASS_OUTPUT
iptables -t mangle -C OUTPUT -j GOVPASS_OUTPUT || iptables -t mangle -I OUTPUT 1 -j GOVPASS_OUTPUT

# Mark reinjected packets to bypass NFQUEUE
iptables -t mangle -A GOVPASS_OUTPUT -m mark --mark 0x1/0x1 -j RETURN

# Optional: exclude loopback
iptables -t mangle -A GOVPASS_OUTPUT -o lo -j RETURN

# Queue outbound TCP 443
iptables -t mangle -A GOVPASS_OUTPUT -p tcp --dport 443 -j NFQUEUE --queue-num 100 --queue-bypass
```

For nftables, use a similar rule set with `queue num 100 bypass`.
When using an `inet` table, ensure the queue rule is restricted to IPv4 only
(e.g., `meta nfproto ipv4 ...`) because the splitter currently binds AF_INET.

## Injection strategy (raw socket)

- Use a raw socket (AF_INET, SOCK_RAW, IPPROTO_RAW) with IP_HDRINCL.
- Send split segments with:
  - Original IP/TCP headers, updated seq/len/checksum
  - TCP flags preserved; PSH/FIN only on last segment of injection
- Set SO_MARK to the configured mark (default 0x1) so reinjected packets bypass NFQUEUE.

## Reassembly edge cases

- Out-of-order segments: buffer and merge, track contiguous window only.
- Overlap/duplicate segments: de-duplicate and keep earliest bytes.
- Buffer limit exceeded: fail-open and accept held packets.
- Retransmissions after injection: pass-through only.
- ACK-only packets: fast-pathed (pass-through) and not enqueued unless FIN/RST.

## IP fragmentation

- If IP fragments are observed, fail-open for the flow.
- Do not attempt to reassemble IP fragments in this phase.

## Offload considerations

Policy:
- Disable GRO/GSO/TSO on the egress interface for stable behavior.
- Raw socket reinjection does not re-segment large offloaded packets.

Operational default:
- If offload is disabled automatically, the app will attempt to restore the
  original GRO/GSO/TSO settings on exit (`--auto-offload-restore=true`) when it
  can read the initial state successfully.

Example:
```bash
sudo ethtool -K <iface> gro off gso off tso off
```

## Checksum handling

- With raw sockets + IP_HDRINCL, compute IPv4 and TCP checksums.
- If checksums are zeroed, do not assume kernel will fix them.

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
- Cap segment payload size to max-seg-payload (default 1460) and IPv4 total length
- seq = baseSeq + offset
- Recompute checksums (custom helper on Linux)

## Fail-open behavior

On any error, timeout, or buffer pressure:
- Verdict accept all held packets
- Mark flow PASS_THROUGH

Additional pressure guards (per worker):
- max-flows-per-worker
- max-reassembly-bytes-per-worker
- max-held-bytes-per-worker

## Concurrency model

Same sharded worker model as Windows:
- Single NFQUEUE recv loop
- Hash FlowKey to worker shard
- Per-worker in-order injection

## Shutdown behavior

On shutdown or worker exit:
- Workers fail-open held packets and drain queued-but-unprocessed packets (pass-through).
- Shutdown draining is bounded by a timeout and max packet count to prevent Stop from hanging forever under load.
- After workers stop, the adapter performs a best-effort flush of adapter-level pending packets before the handle is closed.

## Linux-specific considerations

- Packet offload (GSO/TSO): verify if NFQUEUE sees large segments.
  If so, document or disable offload for testing.
- Kernel versions: prefer 5.x+ with stable NFQUEUE behavior.
- Permissions: root or setcap for CAP_NET_ADMIN + CAP_NET_RAW.

## Open questions

- Default queue-num and queue-maxlen for throughput vs latency
- nftables vs iptables default (prefer nftables on modern distros)
- Whether to expose rule management in CLI or external scripts only
