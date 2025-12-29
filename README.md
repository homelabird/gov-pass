# gov-pass

Split-only TLS ClientHello splitter for outbound IPv4 TCP 443 on Windows.

## Windows build (x64)

Prerequisites:
- Windows 10/11 x64
- Go 1.21+
- WinDivert 2.2.2-A x64 files (this repo includes them under
  `third_party\windivert\WinDivert-2.2.2-A\x64`)

Build the binary:
```powershell
go build -o dist\splitter.exe .\cmd\splitter
```

Package WinDivert alongside the binary:
```powershell
.\scripts\package.ps1 `
  -ExePath .\dist\splitter.exe `
  -WinDivertDir .\third_party\windivert\WinDivert-2.2.2-A\x64 `
  -OutDir .\dist
```

Expected files in `dist\`:
- `splitter.exe`
- `WinDivert.dll`
- `WinDivert64.sys`
- `WinDivert.cat` (optional; not included in the official zip)

Optional: install the driver once (requires Admin):
```powershell
.\scripts\install_windivert.ps1 -WinDivertDir .\dist
```

Run (Admin PowerShell):
```powershell
.\dist\splitter.exe
```

If you want to keep the driver installed after exit:
```powershell
.\dist\splitter.exe --auto-uninstall=false
```
