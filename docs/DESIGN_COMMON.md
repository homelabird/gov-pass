# Common Split Engine Design

This document captures behavior shared by every platform backend.

## Scope

- target: outbound IPv4 TCP/443
- strategy: split only the first TLS ClientHello record per flow
- fallback: fail-open on parse error, timeout, or resource pressure

## Flow Lifecycle

`NEW -> COLLECTING -> SPLIT_READY -> INJECTED -> PASS_THROUGH -> CLOSED`

- `COLLECTING`: hold packets until the split decision is known
- `SPLIT_READY`: the first TLS record is fully available
- `INJECTED`: split segments were emitted instead of the originals
- `PASS_THROUGH`: later packets in the flow are forwarded unchanged

## Split Decision

The engine waits for 6 contiguous bytes and checks:

1. TLS content type `0x16`
2. TLS version `0x0301..0x0304`
3. Handshake type `0x01`

If the first record is valid, the engine waits for the full record
(`5 + recordLen`) and then splits only that record. Everything else stays
pass-through.

## Safety Rules

- Hold packets only while the split decision is pending.
- On success, drop held originals and inject split segments.
- On failure, reinject or accept held packets in original order.
- Bound shutdown with a timeout and packet limit so stop paths cannot hang.
