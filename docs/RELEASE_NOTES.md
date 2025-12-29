# Release Notes

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
