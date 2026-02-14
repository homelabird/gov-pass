# Release Notes

## Unreleased

Highlights:
- Engine shutdown hardening: bounded worker fail-open drain + adapter-level flush before close.
- recvLoop ACK-only fast-path to reduce worker queue pressure.
- DoS guards: per-worker caps for flows/held/reassembly bytes (fail-open on pressure).
- Windows service/MSI: auto-start service + Start Menu admin shortcuts + ProgramData config/log.
- Windows UX: system tray app (`gov-pass-tray.exe`) to start/stop/status the `gov-pass` service (with UAC prompt).
- Windows release signing: Authenticode-signed EXE/MSI in CI (see `docs/CODESIGNING.md`).
- MSI uninstall cleanup options:
  - `GOVPASS_PURGE_PROGRAMDATA=1` to delete `C:\ProgramData\gov-pass\`
  - `GOVPASS_REMOVE_WINDIVERT=1` to stop/delete the global `WinDivert` service
  - best-effort cleanup of the `gov-pass-tray` autorun Run value (HKCU/HKLM)
- Windows security: ProgramData ACL hardening for service state (config/log/driver files).
- Windows WinDivert self-heal: optional auto-download from a pinned zip (SHA256 verified).
- Linux ops: safe "our rules only" nft/iptables management + offload restore + optional tool auto-install.

## v0.1.1-windows

Highlights:
- WinDivert installer now self-heals: it resolves the absolute driver path and
  updates an existing service if its `binPath` points to a missing `.sys`.
- Added `-ForceBinPath` to the installer script to override the service path
  explicitly.
- Documented third-party sources and Windows install notes in README.

Build/Run:
- Build: `go build -o dist\splitter.exe .\cmd\splitter`
- Package: `.\scripts\package.ps1 -ExePath .\dist\splitter.exe -WinDivertDir .\third_party\windivert\WinDivert-2.2.2-A\x64 -OutDir .\dist`
- Run (Admin): `.\dist\splitter.exe`
