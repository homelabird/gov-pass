# Detailed Design - BSD (pf divert) Split-Only TLS ClientHello (Go)

Status: Experimental (not production).
This doc focuses on pf-divert implementation details. Shared behavior is documented in
[DESIGN_COMMON.md](DESIGN_COMMON.md).

## Platform-specific notes

- pf divert socket/reinject behavior and rule semantics.
- Loop prevention approach (tagged packets, anchor behavior).
- Open questions and platform caveats for FreeBSD/pfSense.

## Target platforms

- pfSense CE 2.7.x (FreeBSD 14.0 base)
- OPNsense 24.x (FreeBSD 13.2/14.1 base)

## Architecture overview

pf (divert-to) -> Divert adapter -> Decoder -> Flow Manager -> Reassembler
-> TLS Inspector -> Split Plan -> Injector (divert send)

Non-target and completed flows are reinjected as-is.

## pf divert adapter

- Use pf `divert-to 127.0.0.1 port <divert_port>` for outbound TCP 443.
- Open a divert socket (IPPROTO_DIVERT) bound to the divert port.
- Receive packet bytes + metadata via `recvfrom`.
- Reinject via `sendto` with the original sockaddr.
- Drop by not reinjecting (used when replacing originals with split segments).

## Rule installation (pf)

Anchor example (schematic; final rules TBD):

```pf
anchor "gov-pass"
load anchor "gov-pass" from "/etc/pf.anchors/gov-pass"
```

```pf
# /etc/pf.anchors/gov-pass
lan_net = "192.168.1.0/24"
wan_if = "em0"
divert_port = "10000"

# Bypass reinjected packets (tagged)
match out on $wan_if inet proto tcp tagged GOVPASS -> $wan_if

# Divert LAN -> WAN HTTPS
pass out on $wan_if inet proto tcp from $lan_net to any port 443 \
  divert-to 127.0.0.1 port $divert_port tag GOVPASS
```

## Reinjection loop prevention

Pick one mechanism and standardize:
- Use `tag`/`tagged` to bypass divert on reinjected packets.
- Use a dedicated anchor with `pass quick` for tagged packets.
- Interface-based skip (e.g., loopback) where appropriate.

## Packet handling

- COLLECTING: hold packets until split-ready.
- SPLIT_READY: drop originals and inject split segments.
- FAIL-OPEN: reinject held packets in original order.
- PASS-THROUGH: reinject packets as-is for the rest of the flow.

## Shutdown behavior

On shutdown or worker exit:
- Workers fail-open held packets and drain queued-but-unprocessed packets (pass-through).
- Shutdown draining is bounded by a timeout and max packet count to prevent Stop from hanging forever under load.
- After workers stop, the adapter performs a best-effort flush of adapter-level pending packets before the handle is closed.

## Split plan

- Split window: first TLS record only
- First segment size = split-chunk (default 5)
- Remaining bytes in one or more segments
- Cap segment payload size to max-seg-payload (default 1460) and IPv4 total length

## Offload considerations

- TSO/LRO/CSUM offload may hide true packet boundaries from pf/divert.
- Document required `ifconfig` toggles per interface for testing.

## PoC plan (FreeBSD VM)

- 2-NIC router VM (LAN + WAN).
- Add pf anchor with `divert-to` for LAN -> WAN TCP 443.
- PoC binary: recv + reinject unchanged (no split) to validate loop-free pass-through.
- Verify with `tcpdump` and `curl` that TLS connections succeed.

## Open questions

- Is pf divert available/enabled in pfSense/OPNsense kernels (IPDIVERT)?
- Best pf rule placement relative to NAT for LAN -> WAN?
- Reliable loop-prevention mechanism across pfSense/OPNsense versions?
