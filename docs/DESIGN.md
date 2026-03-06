# Windows Design

This document covers the Windows-specific path. Shared engine behavior lives in
[`DESIGN_COMMON.md`](DESIGN_COMMON.md).

## Data Path

`WinDivert -> decoder -> flow shard -> reassembly -> TLS check -> split plan -> WinDivert send`

Non-target traffic bypasses reassembly immediately. ACK-only packets are
fast-pathed. FIN and RST packets still go through workers so flow state is
cleaned up promptly.

## Platform Defaults

- filter: `outbound and ip and tcp.DstPort == 443`
- split mode: `tls-hello`
- split chunk: `5`
- queue length, time, and size are tuned for low-latency interception

## Flow Handling

- Packets are hashed to sharded workers by flow key.
- Each worker owns reassembly state and held packets for its shard.
- While collecting, original packets are held until the first TLS record is
  either accepted for split or failed open.

## Split And Reinjection

- Split only the first TLS record.
- Preserve the original headers, ACK/window state, and TCP options.
- Recompute lengths and checksums before `WinDivertSend`.
- After a successful split, the flow moves to pass-through mode.

## Fail-Open Rules

Fail open on:

- invalid TLS header
- timeout while collecting
- buffer or held-packet limits
- decode errors
- malformed TCP/IP input

Fail-open means the held originals are reinjected in order and the rest of the
flow continues unchanged.

## Service And Shutdown

- Interactive mode can auto-install and auto-remove WinDivert.
- Service mode keeps WinDivert and runtime state stable under
  `C:\ProgramData\gov-pass\`.
- Shutdown drains held work with explicit bounds and flushes adapter-level
  pending packets best-effort before the handle closes.
