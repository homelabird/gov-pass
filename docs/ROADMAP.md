# Roadmap - gov-pass (Split-Only TLS ClientHello)

This roadmap tracks scope, status, and follow-up tasks for a Go-based packet
splitter that targets outbound IPv4 TCP dst port 443 and performs "split only"
(no payload mutation, TTL tricks, fake packets, or reordering).

## Status (2026-02-14)

- Windows (WinDivert): primary path
- Linux (NFQUEUE): beta
- FreeBSD / pfSense (pf divert): Experimental (not production)
- Android: Deprecated / Historical (not a maintained build target)

Key behaviors (implemented):
- Fail-open on errors/timeouts/pressure.
- ACK-only recv fast-path to reduce worker queue pressure (FIN/RST still go through workers).
- DoS guardrails (per worker): max flows, max held bytes, max reassembly bytes.
- Shutdown hardening:
  - bounded worker fail-open drain (timeout + max packets)
  - adapter-level pending packet flush before close (bounded by timeout)

## Release Artifacts (current)

Produced on Git tags by GitLab CI:
- Linux amd64 tar.gz
- Windows amd64 zip
- Windows amd64 MSI (service auto-start)
- `SHA256SUMS`

Experimental builds:
- FreeBSD amd64 tar.gz (not production)

Optional verification:
- Windows runner E2E can install/uninstall the MSI and smoke-test service start/stop/reload.

## Windows Roadmap (WinDivert + MSI/Service)

Implemented:
- WinDivert adapter with safer shutdown (cancel-aware recv path + adapter Flush).
- MSI installs `gov-pass` service (LocalSystem) with auto-start.
- Code signing for Windows EXE/MSI in CI (Authenticode via `osslsigncode`).
- ProgramData state:
  - config: `C:\ProgramData\gov-pass\config.json`
  - log: `C:\ProgramData\gov-pass\splitter.log`
  - ACL hardened in service mode (SYSTEM/Admin full, Users read-only).
- Service config reload:
  - `sc.exe control gov-pass paramchange` triggers in-process reload (in-place apply).
  - Some settings still require service restart (filter and driver file location changes).
- Optional WinDivert file self-heal:
  - download official WinDivert zip from GitHub releases
  - verify pinned SHA256
  - extract x64 `WinDivert.dll` + `WinDivert64.sys`

Next:
- Explicit versioned config schema docs and examples (service safe defaults).
- Service hardening follow-ups:
  - optional Event Log integration
  - optional `--service-log` rotation strategy
- Reload improvements:
  - optional in-place handle reopen to apply filter changes without full service restart
  - document which settings are reloadable vs restart-only as a table

## Linux Roadmap (NFQUEUE)

Implemented:
- Pure-Go NFQUEUE adapter (`go-nfqueue`) + raw socket reinjection + SO_MARK loop avoidance.
- Auto rule install/uninstall:
  - nftables: tagged rules (comment) and delete-by-handle ("only our rules")
  - iptables: dedicated chain (`GOVPASS_OUTPUT`)
  - nft `inet` queue rule is restricted to IPv4 (`meta nfproto ipv4 ...`).
- Offload handling:
  - optional auto disable GRO/GSO/TSO
  - optional restore on exit when initial state is readable (`--auto-offload-restore=true`).
- Optional package-manager auto-install of missing tools (nft/iptables/ip/ethtool).

Next:
- Expand netns integration tests into a CI-usable Linux verify stage (root-required runner).
- Improve interface detection and multi-egress handling (policy for containers/VPN).
- More robust nftables compatibility notes (iptables-nft, distros).
- IPv6 design and rollout plan (currently IPv4 only).

## FreeBSD / pfSense Roadmap (pf divert)

Implemented:
- Divert adapter implementation (recv + reinject) with shared engine pipeline.
- Design and PoC docs under `docs/POC_BSD.md` and `docs/pf/*`.

Next:
- Scripts/templates:
  - pf anchor install/remove scripts
  - rc.d service template
- pfSense/OPNsense operational guide:
  - where to hook rule reloads
  - offload guidance and troubleshooting
- Validation:
  - pcap comparison workflow
  - fail-open verification under load and shutdown

## Cross-Cutting Backlog

Correctness/Testing:
- Engine.Reload test coverage and restart-only constraints coverage.
- Adapter Flush ordering/timeout coverage across adapters.
- More unit tests:
  - reassembly wrap-around and overlap cases
  - checksum parity checks (Windows helper vs pure-Go)
- Deterministic integration harness:
  - pcap replay
  - handshake success regression tests

Observability:
- Add counters (splits_ok, fail_open reasons, pressure triggers).
- Structured logging with event IDs for troubleshooting.

Config/UX:
- Consolidate config documentation across platforms (CLI flags + Windows service config.json).
- Provide example configs for common environments (workstation, gateway/router).

Security/Operations:
- Maintain `SECURITY.md` and keep hardening features documented.
- Ensure all auto-mutating features have clear opt-out flags (rules/offload/tool install/download).
