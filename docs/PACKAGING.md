# Packaging

This document describes the packaging surface that matters for shipping and
operating `gov-pass`. For normal install and run instructions, start with
[`../README.md`](../README.md).

## Windows

Required runtime files:

- `splitter.exe`
- `WinDivert.dll`
- `WinDivert64.sys` or `WinDivert.sys`
- `WinDivert.cat` when available

Optional packaged files:

- `gov-pass-tui.exe`
- `gov-pass-msi-helper.exe`
- admin helper `.cmd` launchers

Runtime behavior:

- The executable auto-installs or auto-downloads WinDivert by default.
- Existing `WinDivert` services are treated as foreign when they already point
  at another valid driver path; use `allow_service_takeover` only for recovery
  or controlled migrations.
- Service mode stores config and logs under `C:\ProgramData\gov-pass\`.
- Config reload uses `sc.exe control gov-pass paramchange`.
- First service start creates `C:\ProgramData\gov-pass\config.json` when missing.
- Use [`examples/splitter.windows.json`](examples/splitter.windows.json) as the
  reference service config shape.

MSI expectations:

- Install path: `C:\Program Files\gov-pass\`
- Service name: `gov-pass`
- Start mode: automatic
- TUI controller can be bundled alongside the service install

Reload model:

- Reloadable in place:
  - most `engine.*` settings except worker topology
  - `windivert.queue_len`, `windivert.queue_time_ms`, `windivert.queue_size_bytes` when set to non-zero values
- Restart required:
  - `engine.workers`
  - `engine.worker_queue_size`
  - `windivert.filter`
  - `windivert_dir`
  - `windivert_sys`
  - queue values changed back to `0` (driver default)

Build notes:

- CI packages Windows artifacts from [`installer/windows/`](../installer/windows/).
- Local MSI builds render [`installer/windows/gov-pass.wxs.in`](../installer/windows/gov-pass.wxs.in)
  and then build it with WiX.
- Signing details live in [`CODESIGNING.md`](CODESIGNING.md).
- GitHub tag releases are published by [`../.github/workflows/release.yml`](../.github/workflows/release.yml)
  and upload the bootstrap installer plus signed checksum manifests that the
  Linux release-install flow depends on.

## Linux

Typical package contents:

- `/usr/libexec/gov-pass/splitter` or `/opt/gov-pass/dist/splitter`
- systemd unit: `gov-pass.service`
- env/config file: `/etc/default/gov-pass` or `/etc/sysconfig/gov-pass`
- NFQUEUE helper scripts for rule install and cleanup

Notes:

- native `.deb`/`.rpm` packages install the runtime and service files only
- the optional `gov-pass-tui` controller is installed by the source installer or
  the release installer with `INSTALL_TUI=1`
- the release tarball now carries `gov-pass.service` and `gov-pass.default` so
  the installer can stay version-pinned without fetching files from `main`
- release checksum manifests are published with detached signatures:
  - `SHA256SUMS` + `SHA256SUMS.sig`
  - `SHA256SUMS.deb` + `SHA256SUMS.deb.sig`
  - `SHA256SUMS.rpm` + `SHA256SUMS.rpm.sig`
- the bootstrap installer is published as a release asset:
  - `install_one_touch_curl.sh`
- the release installer requires a trusted PEM public key and verifies the
  manifest signature before it trusts any asset checksum
- bootstrap flows should verify `install_one_touch_curl.sh` against the signed
  `SHA256SUMS` manifest before executing it with elevated privileges

Preferred install paths:

- one-touch script: [`../scripts/install_one_touch.sh`](../scripts/install_one_touch.sh)
- release installer: [`../scripts/install_one_touch_curl.sh`](../scripts/install_one_touch_curl.sh)
- native packages: [`../packaging/deb/`](../packaging/deb/) and [`../packaging/rpm/`](../packaging/rpm/)

Service knobs exposed through env files:

- `GOV_PASS_QUEUE_NUM`
- `GOV_PASS_MARK`
- `GOV_PASS_ARGS`

Operational defaults:

- auto-manage NFQUEUE rules with `nft` or `iptables`
- auto-disable GRO/GSO/TSO when needed
- restore offload state on exit when the original state is known
- default to `--ip-family=auto` so single-stack Linux hosts can run without
  forcing both IPv4 and IPv6 adapter paths open
- service mode keeps `--auto-install-tools=false`; package or pre-provision
  `nftables` or `iptables`/`ip6tables`, `ethtool`, and `iproute2`
- set `--iface` explicitly for multi-egress, VPN, or container-heavy hosts

## FreeBSD

Packaging is intentionally minimal, but the source installer now lays down the
standard service/helper paths:

- install `splitter`
- optionally install `gov-pass-tui`
- install `rc.d` service: `/usr/local/etc/rc.d/gov-pass`
- install `pf` helpers:
  - `/usr/local/libexec/gov-pass/install_pf_anchor.sh`
  - `/usr/local/libexec/gov-pass/uninstall_pf_anchor.sh`
- install editable anchor template: `/usr/local/etc/gov-pass/pf.anchor.conf`
- keep `pf` policy operator-managed through the helper and anchor template

Use [`pf/`](pf/) for anchor templates and [`DESIGN_BSD.md`](DESIGN_BSD.md) for
the divert flow model.

Operator smoke scripts:

- Windows MSI + TUI smoke: [`../scripts/windows/ci_msi_e2e.ps1`](../scripts/windows/ci_msi_e2e.ps1)
- FreeBSD TUI smoke: [`../scripts/freebsd/ci_tui_smoke.sh`](../scripts/freebsd/ci_tui_smoke.sh)
