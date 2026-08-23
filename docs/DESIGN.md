# Design

This document is the short architecture reference for `gov-pass`.

## Scope

- target: outbound TCP/443
- strategy: split only the first TLS ClientHello record
- fallback: fail open on parse error, timeout, malformed input, or resource pressure

## Shared Engine

Data path:

`adapter -> decoder -> flow shard -> reassembly -> TLS check -> split plan -> reinject or pass through`

Core behavior:

- flows are sharded by flow key so each worker owns its own reassembly state
- only the first TLS record is inspected
- while the split decision is pending, original packets are held
- on split success, originals are dropped and split segments are emitted
- on failure, held originals are released in order and the flow stays pass-through
- ACK-only packets always pass through immediately; when the lightweight
  liveness channel is full, the refresh is skipped instead of blocking the
  global receive loop
- collect timeouts are enforced independently of slower idle-flow GC even if a
  flow stops producing new packets mid-collection
- after split emission commits, held originals are detached before drop so
  shutdown and fail-open recovery cannot re-emit them after a partial split
- shutdown is bounded so stop paths cannot hang indefinitely under load

The TLS decision requires a contiguous 5-byte record header plus the first
handshake byte. The engine accepts only a valid TLS handshake record, waits for
the full first record, then splits that record once. Everything after the first
decision is pass-through.

## Windows

Path:

`WinDivert -> decoder -> engine -> WinDivert send`

Windows uses WinDivert for interception and reinjection. Interactive mode can
install and remove WinDivert automatically. Service mode keeps config, logs,
and driver state under `C:\ProgramData\gov-pass\`.

Service reload uses `sc.exe control gov-pass paramchange`. Most `engine.*`
settings reload in place. `engine.workers`, `windivert_dir`, and
`windivert_sys` remain restart-only. WinDivert filter and queue parameters
reload by reopening or updating the active handle.

## Linux

Path:

`NFQUEUE -> decoder -> engine -> raw socket send`

Linux binds outbound TCP/443 traffic to a dedicated NFQUEUE and tags reinjected
packets so they bypass interception on the return path. Runtime helpers manage
only `gov-pass` firewall rules and can disable offload features on the selected
egress interface when needed.

The Linux service surface is systemd-based. Reload is supported only when the
installed unit exposes a real reload action.

## FreeBSD

Status: usable, but still narrower than Linux and Windows.

Path:

`pf divert-to -> divert socket -> decoder -> engine -> reinject`

FreeBSD relies on a dedicated `pf` anchor and helper scripts under
[`pf/`](pf/). Operators still control when anchor rules are applied.

Current limits:

- reload is not supported; use restart flows
- IPv6 divert handling is not a supported deployment target yet
- `splitter --check` verifies prerequisites, `pf` enabled state, and whether
  the `gov-pass` live anchor has rules; operators still verify interface
  selectors for their topology
