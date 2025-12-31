# Roadmap - WinDivert IPv4 Split-Only Engine

This roadmap captures scope, design, and phased tasks for a Go-based packet
splitter that uses WinDivert on Windows 10/11 x64. It targets outbound IPv4 TCP
dst port 443 only and performs "split only" (no payload mutation, TTL tricks,
fake packets, or reordering).

## Scope

- Windows 10/11 x64 only
- Requires admin privileges and WinDivert driver installed
- Hook outbound IPv4 TCP dst port 443 only
- Optional loopback/local/VPN interface exclusions
- Split only (no byte modification), no TTL tricks, no fake packets, no reordering
- Per-flow: process only the first ClientHello; then pass-through
- Fail-open on error or timeout

## Design decisions (current defaults)

- Split trigger mode: tls-hello (default), optional immediate
- TLS completion: first TLS record only (default)
- Split chunk: 5 bytes (default)
- Collect timeout: 250ms (default; tune later)
- Max buffer: 64KB (default; tune later)
- Max held packets: 32 (default; tune later)
- WinDivert queue: len=4096, time=2000ms, size=32MB (default)

## Modules

- WinDivert adapter: open/recv/send/close, filter, queue params
- Packet decoder: IPv4/TCP parsing, payload extraction, checksum utils
- Flow manager: 5-tuple key, state/timeout/GC
- Stream reassembler: seq-based fragment buffer, contiguous range assembly
- TLS inspector: ClientHello detection (record header + handshake type)
- Splitter/injector: segment creation, drop original, reinject

## Packet pipeline

1. Receive packet from WinDivert; apply IPv4/TCP/port filter
2. Update flow table; non-target or completed flows pass-through
3. Target flows feed reassembler with payload
4. When ClientHello condition is met, generate split plan
5. Drop original packet(s); reinject split segments in order

## State machine

NEW -> COLLECTING -> SPLIT_READY -> INJECTED -> PASS_THROUGH -> CLOSED
  \         |            |              |
   \--timeout/error------/--------------/

### State definitions

- NEW: first packet observed, initialize flow key and base seq
- COLLECTING: buffer payload, assemble contiguous ranges only
- SPLIT_READY: ClientHello criteria satisfied, split plan ready
- INJECTED: split segments sent, future packets pass-through
- PASS_THROUGH: parsing failed/timeout/buffer overflow, allow all
- CLOSED: FIN/RST/timeout, cleanup

## Core data structures

- FlowKey: 5-tuple (src/dst IP, src/dst port, proto)
- FlowState: state, last activity, processed flag
- ReassemblyBuffer: map seq->payload, contiguous range tracker
- TLSWindow: header/body availability, offsets
- SplitPlan: split rule, segment list
- HeldPackets: original packets buffered until decision

## Runtime flags

- split-mode: immediate | tls-hello (default tls-hello)
- split-chunk: N (default 5)
- collect-timeout: 250ms (default; tune later)
- max-buffer: 64KB (default; tune later)
- max-held-pkts: 32 (default; tune later)
- exclude-loopback / exclude-ifidx (default off/empty)
- queue-len: 4096 (default)
- queue-time: 2000ms (default)
- queue-size: 32MB (default)

## TLS detection (tls-hello default)

Minimum contiguous bytes required to trigger split:
- TLS record header (5B) with contentType=0x16 (handshake)
- Version in [0x0301..0x0304]
- Handshake type byte (0x01) present after record header

After validating, collect up to the first TLS record length (5 + recordLen).
If recordLen exceeds max-buffer or timeout occurs, fail-open.

## Split/inject rules

- Build split segments from the selected payload window (first TLS record).
- Use split-chunk size; first segment is exactly split-chunk bytes.
- Sequence numbers: baseSeq + offset
- Preserve TCP flags/options/window/ack; set PSH/FIN only on last segment
- Recompute checksums via WinDivert helpers

## Phase 1

- WinDivert wrapper selection; minimal API (Recv/Send/Close)
- Filter string: outbound and ip and tcp.DstPort==443
- Packet decoder and checksum utils
- Admin privilege check; driver load failure -> fail-fast
- Runtime config flags (split mode/chunk/timeout)

## Phase 2

- Flow table with state transitions
- Reassembler (dup/gap/overlap handling, buffer limits)
- TLS ClientHello detection
- Split strategy: split-chunk=5 (default)
- Fail-open transitions and GC policy

## Phase 3

- Segment creation (seq math, TCP flags/window)
- Drop original and reinject split segments
- Logging/metrics (split success, fail-open count, buffer usage)
- Integration tests (pcap diff, handshake validation)
- Packaging (driver include, install/remove, privilege notes)

## Long-term roadmap

- Handshake-complete mode: parse full ClientHello length and split after full
  handshake is buffered (optional trigger mode)
- SNI-aware split offsets (optional)
- Advanced interface exclusion (VPN/WSL) using IfIdx/IfSubIdx
- Performance tuning (flow map sharding, buffer pools)

## Linux roadmap (NFQUEUE MVP)

### Scope (MVP)

- Linux x86_64, kernel with netfilter/NFQUEUE
- Requires root or capabilities (CAP_NET_ADMIN, CAP_NET_RAW)
- Hook outbound IPv4 TCP dst port 443 via NFQUEUE
- Split only (no payload mutation), first ClientHello per flow only
- Fail-open on error/timeout; loopback/VPN exclusions optional
- Reinjection loop prevention via fwmark/connmark rules

### Modules

- NFQUEUE adapter: open/bind queue, set copy range, receive packets, verdicts
- Raw socket injector: send split segments with IP_HDRINCL, set SO_MARK
- Rule manager: iptables/nftables install/remove; bypass rules for marked packets
- Driver shim: Linux-specific config and capability checks (no WinDivert)

### Phase L1

- Choose NFQUEUE Go binding (libnetfilter_queue via cgo)
- Add Linux build-tagged adapter stubs
- Add flags: queue-num, copy-range, mark, rule-install/rule-remove
- Script: install/remove NFQUEUE rules (iptables + nftables variants)

### Phase L2

- Wire engine to NFQUEUE: drop originals, inject split segments
- Implement reinjection loop avoidance (SO_MARK + mangle table bypass)
- Verify checksum handling with raw socket injection

### Phase L3

- Integration tests (netns + pcap validation)
- systemd unit and capability setup guidance
- Packaging docs for Linux (rules, permissions, dist layout)

## BSD roadmap (pf divert, pfSense/OPNsense)

### Scope (MVP)

- FreeBSD-based pfSense CE 2.7.x and OPNsense 24.x
- LAN -> WAN outbound IPv4 TCP 443 only
- Split only (no payload mutation), first ClientHello per flow
- Fail-open on error/timeout; exclude loopback/VPN/VLAN initially

### Phase B1 (Target + PoC)

- Confirm target OS versions and interface scope (LAN -> WAN only)
- Verify pf divert availability (IPDIVERT) on target kernels
- FreeBSD VM PoC: divert socket recv + reinject pass-through
- Validate with tcpdump/curl and confirm no reinjection loops

### Phase B2 (Adapter + flags)

- Add freebsd build tags and divert adapter implementation
- Add CLI flags: divert-port, pf anchor path, bypass tag
- Implement checksum handling and reinjection parity with Linux

### Phase B3 (Rules + packaging)

- pf anchor install/remove scripts
- rc.d service template for FreeBSD
- pfSense/OPNsense install notes and defaults

### Phase B4 (Validation)

- PCAP verification, fail-open behavior, split correctness
- Offload guidance (TSO/LRO/CSUM) and performance tuning

## Open questions

- Production tuning for timeout/buffer/held-pkts defaults (start: 250ms/64KB/32)
- Default interface exclusion policy (loopback on/off; VPN/WSL via IfIdx/IfSubIdx)
- Driver packaging strategy (bundle vs external installer)

## Android roadmap (Root + Magisk, NFQUEUE)

See `docs/DESIGN_ANDROID.md` for the Android-specific plan and packaging details.
