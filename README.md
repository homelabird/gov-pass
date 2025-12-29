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

Driver install notes:
- The installer resolves the absolute `.sys` path and will update an existing
  service if its `binPath` points to a missing file.
- To force-update the `binPath` even if it exists, use:
```powershell
.\scripts\install_windivert.ps1 -WinDivertDir .\dist -ForceBinPath
```

Run (Admin PowerShell):
```powershell
.\dist\splitter.exe
```

If you want to keep the driver installed after exit:
```powershell
.\dist\splitter.exe --auto-uninstall=false
```

## Linux build and run (NFQUEUE MVP)

Linux support is beta. Expect breaking changes and validate in your environment.

Prerequisites:
- Linux x86_64
- Go 1.21+
- `libnetfilter_queue` development package (cgo required)

Install dependencies:
```bash
# Debian/Ubuntu
sudo apt-get update
sudo apt-get install -y libnetfilter-queue-dev

# Fedora
sudo dnf install -y libnetfilter_queue-devel
```

Build:
```bash
CGO_ENABLED=1 go build -o dist/splitter ./cmd/splitter
```

Install NFQUEUE rules (iptables or nftables):
```bash
sudo ./scripts/linux/install_nfqueue.sh --queue-num 100 --mark 1
```

Run (root or with capabilities):
```bash
sudo ./dist/splitter --queue-num 100 --mark 1
```

Optional capabilities instead of root:
```bash
sudo setcap 'cap_net_admin,cap_net_raw=+ep' ./dist/splitter
```

Cleanup rules:
```bash
sudo ./scripts/linux/uninstall_nfqueue.sh --queue-num 100 --mark 1
```

## Third-party sources included

- WinDivert 2.2.2-A (binaries/docs) in `third_party\windivert\WinDivert-2.2.2-A`
  and `third_party\WinDivert.zip` (license: `third_party\windivert\WinDivert-2.2.2-A\LICENSE`)
