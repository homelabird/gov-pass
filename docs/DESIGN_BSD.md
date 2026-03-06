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

## Caveats

- Validate offload settings on each target NIC and OS combination.
- Confirm rule placement carefully around NAT and LAN-to-WAN forwarding.
- Treat this path as a lab or pilot feature, not a production deployment target.
