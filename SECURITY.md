# Security and Operational Hardening

This project intercepts and reinjects packets. It can run with elevated
privileges (Administrator/root) and should be treated as high-impact software.

This document describes the intended security model and the hardening measures
implemented in the repo.

## Scope and Threat Model

In scope:
- Local machine threat model (unprivileged user trying to tamper with service state).
- Untrusted outbound TCP/443 packet input, including malformed headers, partial
  TLS records, and resource-pressure cases that must fail open instead of
  wedging the host.
- Operational safety: fail-open behavior, bounded shutdown, and rules cleanup.

Out of scope:
- Using this project as a security boundary or malware sandbox.
- Protection against a fully privileged local attacker.

Operational safety notes:
- Active pass-through flows stay live even when the lightweight ACK touch path is
  saturated; the engine falls back to a durable worker-queue update.
- Collect timeouts are enforced during worker GC, so partially collected flows do
  not linger indefinitely without fresh packets.
- Once split emission commits, held originals are detached before drop so
  shutdown or fail-open recovery cannot reinject them after partial split
  success.

## Windows (WinDivert + Service)

Privileges:
- WinDivert driver installation and packet interception require Administrator.
- The MSI installs a Windows service (`gov-pass`) that runs as LocalSystem.

Service state locations:
- Program Files: installs `splitter.exe` and WinDivert files next to it.
- ProgramData:
  - config: `C:\ProgramData\gov-pass\config.json`
  - log: `C:\ProgramData\gov-pass\splitter.log`
  - optional WinDivert fallback dir: `C:\ProgramData\gov-pass\windivert`

Hardening (implemented):
- ProgramData ACL hardening in service mode:
  - directory: SYSTEM/Admin full, Users read-only
  - files: SYSTEM/Admin full, Users read-only
  - prevents unprivileged config tampering and reduces DLL/driver path hijack risk.
- Service reload is explicit:
  - `sc.exe control gov-pass paramchange` triggers reload of `config.json`.
  - settings that require reopening WinDivert (filter/driver path changes) are not applied in-place.

External downloads:
- Optional WinDivert file auto-download is supported.
- The download uses a pinned official WinDivert release zip and verifies its SHA256.
- You can disable auto-download with `--auto-download-windivert=false` or via the service config.

Release integrity:
- Download the bootstrap installer from a release asset and verify it against
  the signed `SHA256SUMS` manifest before executing it with `sudo`.
- The Linux release installer requires a trusted PEM public key
  (`GOV_PASS_RELEASE_PUBKEY_PATH`, `GOV_PASS_RELEASE_PUBKEY_PEM`, or
  `GOV_PASS_RELEASE_PUBKEY_PEM_B64`) before it will install a release artifact.
- Published checksum manifests are shipped with detached signatures and the
  installer verifies the manifest signature before trusting any asset checksum.
- The release bootstrap is still an operator-run shell script; treat it
  separately from the steady-state privileged runtime and keep the signed-manifest
  verification step in place.

Operational guidance:
- Treat `C:\ProgramData\gov-pass\config.json` as an admin-managed file.
- Do not run an interactive instance while the service is running (avoid double interception).
- Prefer the MSI/Service for stable startup ordering and a consistent state directory.

## Linux (NFQUEUE)

Privileges:
- Auto helpers (rules install/offload changes/tool install) require root because they invoke `nft/iptables/ethtool/ip`.
- You can disable auto helpers and use capabilities (e.g. `CAP_NET_ADMIN`, `CAP_NET_RAW`) with manually managed rules/offload.

Rules hardening (implemented):
- nftables:
  - rules are tagged with a comment (`gov-pass`)
  - uninstall deletes only tagged rules (delete-by-handle), not user rules
  - when using an `inet` table, the NFQUEUE rule is restricted to IPv4 (`meta nfproto ipv4 ...`).
- iptables:
  - uses a dedicated chain (`GOVPASS_OUTPUT`) so uninstall does not flush user rules.

Offload safety (implemented):
- Optional offload restore on exit when the initial state is readable (`--auto-offload-restore=true`).

External tool installation:
- Optional package-manager installation of missing tools for auto helpers (`--auto-install-tools=true`).
- Disable this in locked-down environments and pre-provision tools instead.
- Packaged/systemd service mode keeps `--auto-install-tools=false` by default so
  dependency changes do not happen during steady-state service restarts.
- Package-manager installs run with a minimal inherited environment to reduce
  unexpected influence from caller-controlled variables.
- The trusted absolute-path lookup model applies to privileged runtime and TUI
  helpers. Bootstrap installers remain operator-run setup scripts, not part of
  the steady-state service path.

Operational guidance:
- Prefer `nftables`; use `iptables`/`ip6tables` fallback only where `nft` is not available.
- On multi-egress, VPN, or container-heavy hosts, set `--iface` explicitly instead of relying on route auto-detection.

## FreeBSD / pfSense (pf divert)

Privileges:
- Requires root and pf divert rules.

Operational notes:
- pf rules should be managed via an anchor so uninstall/reload is bounded to "our rules".
- The source installer now adds an `rc.d` service plus
  `/usr/local/libexec/gov-pass/install_pf_anchor.sh` and
  `/usr/local/libexec/gov-pass/uninstall_pf_anchor.sh`, but the actual anchor
  policy still requires operator review and editing before it is applied.
- Privileged helper invocations resolve `service`, `sysrc`, `sudo`, `doas`, and
  `pfctl` from trusted absolute-path locations instead of the ambient `PATH`.
- Offload may affect observed packet boundaries; validate per target NIC/OS.

## Vulnerability Reporting

If you discover a security issue:
- Prefer reporting privately (GitLab "confidential issue" if available in your deployment).
- If you must use a public issue, avoid posting exploit details and include only high-level impact and reproduction constraints.
