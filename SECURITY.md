# Security and Operational Hardening

This project intercepts and reinjects packets. It can run with elevated
privileges (Administrator/root) and should be treated as high-impact software.

This document describes the intended security model and the hardening measures
implemented in the repo.

## Scope and Threat Model

In scope:
- Local machine threat model (unprivileged user trying to tamper with service state).
- Operational safety: fail-open behavior, bounded shutdown, and rules cleanup.

Out of scope:
- Using this project as a security boundary or malware sandbox.
- Protection against a fully privileged local attacker.

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

## FreeBSD / pfSense (pf divert)

Privileges:
- Requires root and pf divert rules.

Operational notes:
- pf rules should be managed via an anchor so uninstall/reload is bounded to "our rules".
- Offload may affect observed packet boundaries; validate per target NIC/OS.

## Vulnerability Reporting

If you discover a security issue:
- Prefer reporting privately (GitLab "confidential issue" if available in your deployment).
- If you must use a public issue, avoid posting exploit details and include only high-level impact and reproduction constraints.

