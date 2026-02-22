# Common Split Engine Design

This document captures behavior shared by Windows, Linux, and BSD engine paths.

## Shared design contract

- Target: outbound IPv4 TCP/443.
- Strategy: split only the first TLS ClientHello record per flow.
- Safety: fail-open on mismatch/parse error/timeout/pressure/error.

## Shared flow lifecycle

`NEW -> COLLECTING -> SPLIT_READY -> INJECTED -> PASS_THROUGH -> CLOSED`

- `NEW`: first payload observed.
- `COLLECTING`: reassembly until split decision.
- `SPLIT_READY`: first TLS record criteria met.
- `INJECTED`: split segments emitted.
- `PASS_THROUGH`: normal forwarding from this point.
- `CLOSED`: FIN, RST, or timeout.

## Shared split detection and fail-open rules

- Detect after 6 contiguous bytes:
  - TLS content type `0x16`
  - TLS version `0x0301..0x0304`
  - Handshake type `0x01`
- Read full record `5 + recordLen` before split.
- Fail-open if checks fail, limits are exceeded, or timeout occurs.

## Shared queue/shutdown policy

- Hold packets while collecting.
- On success: drop originals and inject split segments.
- On fail-open: reinject held packets in original order.
- Shutdown is bounded (timeout + packet limit), then flush adapter pending packets best-effort.
