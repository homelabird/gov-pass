# Common Split Engine Design

This document captures behavior shared by Windows, Linux, and BSD engine paths.

## Common goals

- Intercept outbound IPv4 TCP port 443.
- Split only the first TLS ClientHello record per flow.
- Favor low latency and bounded memory.
- Fail-open on errors, parse failures, timeout, and pressure.

## Shared flow model

All platforms use the same flow lifecycle:

`NEW -> COLLECTING -> SPLIT_READY -> INJECTED -> PASS_THROUGH -> CLOSED`

- `NEW`: first payload observed.
- `COLLECTING`: reassembly window is filled while checking ClientHello.
- `SPLIT_READY`: first TLS record satisfied split conditions.
- `INJECTED`: split segments are emitted; flow becomes pass-through afterward.
- `PASS_THROUGH`: no further packet rewriting for the flow.
- `CLOSED`: FIN/RST/idle timeout.

## Shared ClientHello detection

Detection occurs after at least 6 contiguous bytes are available:

1. TLS content type: `0x16`
2. TLS version: `0x0301..0x0304`
3. Handshake type: `0x01`

Then read TLS record length and wait for the full record (`5 + recordLen` bytes).

- If record length exceeds buffers, mismatch occurs, or timeout is exceeded → fail-open.

## Shared split plan

- Split window is the first TLS record only.
- First segment default size: `split-chunk` (default 5 bytes).
- Remaining bytes in one or more segments, bounded by:
  - `max-seg-payload`
  - IPv4 total length limits.
- Segment sequence starts from flow base sequence and increments by offset.
- Keep TCP/IP header intent consistent with original headers and checksums recomputed by each platform.

## Shared queue/fail-open behavior

- Hold packets while collecting.
- If split succeeds: drop originals and inject split segments.
- If fail-open: reinject held packets in original order.
- Non-target packets generally pass immediately.
- ACK-only packets are typically fast-path where explicitly stated per platform.

## Shared limits and shutdown

- `max-flows-per-worker`
- `max-held-pkts`
- `max-held-bytes-per-worker`
- `max-reassembly-bytes-per-worker`
- `collect-timeout`

Shutdown behavior is bounded to avoid hang:
- worker flush timeout / max packet drain
- adapter pending packet flush before handle close
