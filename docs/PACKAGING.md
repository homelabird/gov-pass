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
- Service mode stores config and logs under `C:\ProgramData\gov-pass\`.
- Config reload uses `sc.exe control gov-pass paramchange`.

MSI expectations:

- Install path: `C:\Program Files\gov-pass\`
- Service name: `gov-pass`
- Start mode: automatic
- TUI controller can be bundled alongside the service install

Build notes:

- CI packages Windows artifacts from [`installer/windows/`](../installer/windows/).
- Local MSI builds render [`installer/windows/gov-pass.wxs.in`](../installer/windows/gov-pass.wxs.in)
  and then build it with WiX.
- Signing details live in [`CODESIGNING.md`](CODESIGNING.md).

## Linux

Typical package contents:

- `/usr/libexec/gov-pass/splitter` or `/opt/gov-pass/dist/splitter`
- optional `gov-pass-tui`
- systemd unit: `gov-pass.service`
- env/config file: `/etc/default/gov-pass` or `/etc/sysconfig/gov-pass`
- NFQUEUE helper scripts for rule install and cleanup

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

## FreeBSD

Packaging is intentionally minimal:

- install `splitter`
- optionally install `gov-pass-tui`
- manage `pf` anchors outside the binary

Use [`pf/`](pf/) for anchor templates and [`DESIGN_BSD.md`](DESIGN_BSD.md) for
the divert flow model.
