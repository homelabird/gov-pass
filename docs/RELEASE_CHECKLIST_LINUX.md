# Linux Release Checklist (NFQUEUE)

This document defines the checklist and phased plan required to promote the
Linux build to a stable release.

## Definition of Done

- Works on Ubuntu LTS + Debian or Fedora with the same settings
- NFQUEUE rules install/remove cleanly and repeatedly
- TLS handshake success rate >= 99% (per test scenario)
- Fail-open behavior documented and verified
- Checksums and reinjection verified (pcap comparison)

## Checklist

### Functionality/Correctness
- Ensure NFQUEUE receive -> split/inject -> original drop order
- On injection failure, accept originals (fail-open)
- Detect IP fragments and immediately pass-through
- Preserve TCP options/headers/flags
- Correct split behavior for the first TLS record only

### Loop Avoidance / Rule Alignment
- SO_MARK value matches NFQUEUE bypass rule
- Support both nftables and iptables
- Default `--mark` matches script defaults

### Checksums/Injection
- Verify IPv4/TCP checksum correctness
- Validate raw socket + IP_HDRINCL reinjection
- Decide GSO/TSO policy for NFQUEUE packet sizing (default: disable GRO/GSO/TSO)

### Performance/Stability
- Set default queue-num/queue-maxlen values
- Validate ENOBUFS/timeout handling
- Measure drop rate/latency under load

### Testing/Validation
- Add unit tests for reassembly and checksums
- Add netns-based integration tests
- Automate pcap before/after comparison
- Add a load probe script and document usage

### Packaging/Operations
- Confirm CGO + libnetfilter_queue build steps
- Provide systemd unit and capability (setcap) guidance
- Finalize dist packaging layout

### Docs/Licenses
- Add Linux limitations and troubleshooting notes
- Add go-nfqueue and dependencies to third-party list
- Change Linux note from beta to stable

## Phased Plan

### Phase L4: Correctness / Loop Avoidance
- Add IP fragment detection and pass-through
- Guarantee fail-open on injection error
- Validate SO_MARK + NFQUEUE bypass rule alignment

### Phase L5: Checksums / Injection Stability
- Add checksum unit tests
- Validate raw socket reinjection via pcap
- Decide and document GSO/TSO handling

### Phase L6: Tests / Performance
- Build netns integration test script
- Define load test scenarios and metrics
- Finalize queue defaults

### Phase L7: Packaging / Release
- Add systemd unit and capability guide
- Finalize Linux packaging docs
- Update third-party licenses
- Tag `v0.1.x-linux` once criteria are met
