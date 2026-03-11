# BSD Design

Status: experimental. This path is intended for FreeBSD and pfSense-style
deployments and is not production-ready.

Shared engine behavior lives in [`DESIGN_COMMON.md`](DESIGN_COMMON.md).

## Data Path

`pf divert-to -> divert socket -> decoder -> flow shard -> reassembly -> split plan -> reinject`

`pf` rules send outbound TCP/443 traffic to a local divert port. The runtime
decides whether to split the first TLS record or reinject the original packets.

## Rule Model

- Manage rules through a dedicated `pf` anchor.
- Tag reinjected packets so they bypass the divert rule on the way back out.
- Keep anchor scope narrow so enable, disable, and cleanup remain predictable.

Use [`pf/`](pf/) for example anchor fragments.

## Runtime Behavior

- Hold packets only while the first TLS record is being evaluated.
- On split-ready, drop originals and reinject split segments.
- On failure, reinject held packets in original order.
- After the first decision, leave the rest of the flow in pass-through mode.

## Current Scope

- `reload` is not supported on FreeBSD; use service restart flows instead.
- `pf` policy remains operator-managed. The installer lays down helper scripts
  and an editable anchor template, but operators still choose when to apply
  `pfctl` changes.
- The current divert socket open/bind path is `AF_INET`-based. Treat IPv6
  divert handling as unsupported until the adapter grows an explicit IPv6
  receive/bind path.
- `splitter --check` validates root and required operator commands (`pfctl`,
  `service`, `sysrc`) and emits notes about the current scope; it does not
  attempt to prove that `pf` rules are already installed.

## Caveats

- Validate offload settings on each target NIC and OS combination.
- Confirm rule placement carefully around NAT and LAN-to-WAN forwarding.
- Treat this path as a lab or pilot feature, not a production deployment target.
