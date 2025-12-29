# Packaging and Driver Setup

This project requires WinDivert user-mode DLL and kernel driver files.
Bundle these files with the executable for a self-contained distribution.

Default deployment layout:
- `dist\splitter.exe`
- `dist\WinDivert.dll`
- `dist\WinDivert64.sys` (or `WinDivert.sys`)
- `dist\WinDivert.cat`

The runtime auto-install uses the executable directory by default, so placing
the WinDivert files next to the exe is the standard setup. The service name
is fixed as `WinDivert`.

## Build

```powershell
go build -o dist\splitter.exe .\cmd\splitter
```

## Package (copy DLL/SYS/CAT)

```powershell
.\scripts\package.ps1 -ExePath dist\splitter.exe -WinDivertDir C:\path\to\WinDivert
```

This copies:
- WinDivert.dll
- WinDivert64.sys (or WinDivert.sys if present)
- WinDivert.cat (if present)

## Install/Uninstall (optional)

```powershell
.\scripts\install_windivert.ps1 -WinDivertDir dist
.\scripts\uninstall_windivert.ps1
```

## Runtime auto-install

By default the app will auto-install/start the driver if needed and
uninstall it on exit if it created the service.

Flags:
- --windivert-dir: directory containing WinDivert.dll/.sys/.cat
- --windivert-sys: override driver sys filename
- --auto-install: enable auto install/start (default true)
- --auto-uninstall: uninstall if installed by this run (default true)

## Linux packaging and operations (NFQUEUE)

Default deployment layout:
- `dist/splitter` (Linux binary)
- `scripts/linux/*` (NFQUEUE rule helpers, test scripts)

Build:
```bash
CGO_ENABLED=1 go build -o dist/splitter ./cmd/splitter
```

Dependencies:
- `libnetfilter_queue` (runtime)
- root or capabilities: `CAP_NET_ADMIN`, `CAP_NET_RAW`

Install/Run (example):
```bash
sudo ./scripts/linux/install_nfqueue.sh --queue-num 100 --mark 1
sudo ./dist/splitter --queue-num 100 --mark 1
```

Systemd template:
- `scripts/linux/gov-pass.service` (edit paths as needed)
  - defaults are set in the unit file
  - optional override file: `/etc/default/gov-pass`
    - `GOV_PASS_QUEUE_NUM=100`
    - `GOV_PASS_MARK=1`
    - `GOV_PASS_ARGS=` (optional extra flags)

Suggested installation:
```bash
sudo install -m 755 dist/splitter /opt/gov-pass/dist/splitter
sudo install -m 755 scripts/linux/install_nfqueue.sh /opt/gov-pass/scripts/linux/install_nfqueue.sh
sudo install -m 755 scripts/linux/uninstall_nfqueue.sh /opt/gov-pass/scripts/linux/uninstall_nfqueue.sh
sudo install -m 644 scripts/linux/gov-pass.service /etc/systemd/system/gov-pass.service
sudo systemctl daemon-reload
sudo systemctl enable --now gov-pass
```
