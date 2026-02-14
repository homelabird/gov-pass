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
- --auto-download-windivert: download pinned WinDivert zip if files are missing (default true)
  - if the exe directory is not writable, the downloader falls back to `C:\ProgramData\gov-pass\windivert`
  - in service mode, the ProgramData fallback is ACL-hardened (SYSTEM/Admin full, Users read-only)
- --service: run as a Windows service (SCM)

## Windows MSI (service auto-start)

The MSI installer installs gov-pass under Program Files and registers a Windows
service named `gov-pass` that starts automatically.

Service notes:
- The service runs `splitter.exe --service --service-name gov-pass`.
- The service reads config from `C:\ProgramData\gov-pass\config.json` (created on first run if missing).
- Logs are written to `C:\ProgramData\gov-pass\splitter.log`.
- In service mode, `C:\ProgramData\gov-pass\` is ACL-hardened (SYSTEM/Admin full, Users read-only).
  Editing `config.json` requires Admin.
- Uninstall behavior:
  - By default, the MSI keeps `C:\ProgramData\gov-pass\` (config/log) and does not remove the global `WinDivert` driver service.
  - To purge ProgramData state on uninstall: `msiexec.exe /x <gov-pass.msi> /qn /norestart GOVPASS_PURGE_PROGRAMDATA=1`
  - To remove the global WinDivert service on uninstall: `msiexec.exe /x <gov-pass.msi> /qn /norestart GOVPASS_REMOVE_WINDIVERT=1`
    - This may affect other WinDivert-based apps on the machine.
- Config reload: `sc.exe control gov-pass paramchange`
  - applies engine config in-place (except worker topology) and non-zero WinDivert queue settings
  - requires service restart for: `windivert.filter`, `windivert_dir` / `windivert_sys`, and reverting `queue_*` to `0` (driver defaults)
- Tray UI: the MSI also installs `gov-pass-tray.exe` and a Start Menu shortcut (`gov-pass tray`)
  to show service status and start/stop/reload it (with UAC prompt).

Build MSI in CI:
- GitLab release builds use `msitools` (`wixl`) with the template in `installer/windows/`.
- Optional: a Windows-runner E2E job (`verify_windows_msi_e2e`) can install/uninstall the MSI and
  smoke-test service start/stop/reload. Enable it by setting `WINDOWS_E2E=1` and providing a
  Windows runner tagged `windows`.

## Linux packaging and operations (NFQUEUE)

Default deployment layout:
- `dist/splitter` (Linux binary)
- `scripts/linux/*` (NFQUEUE rule helpers, test scripts)

Build:
```bash
go build -o dist/splitter ./cmd/splitter
```

Dependencies:
- root or capabilities: `CAP_NET_ADMIN`, `CAP_NET_RAW`

Install/Run (default, root):
```bash
sudo ./dist/splitter --queue-num 100 --mark 1
```

By default the Linux binary will:
- install NFQUEUE rules using nft or iptables
- disable GRO/GSO/TSO on the egress interface (auto-detected)
- by default, restore offload settings on exit when possible (`--auto-offload-restore=true`)

Override defaults:
- `--auto-rules=false` to manage rules manually
- `--auto-offload=false` to skip offload changes
- `--auto-offload-restore=false` to keep offload changes persistent after exit
- `--iface <iface>` to override the auto-detected interface
- `--auto-install-tools=false` to disable package-manager auto install of missing tools

Note: auto rules/offload require root because they invoke `nft/iptables/ethtool`.
If using `setcap`, disable the auto helpers and manage rules/offload manually.

Manual rule install (optional):
```bash
sudo ./scripts/linux/install_nfqueue.sh --queue-num 100 --mark 1
```

Systemd template:
- `scripts/linux/gov-pass.service` (edit paths as needed)
  - defaults are set in the unit file
  - optional override file: `/etc/default/gov-pass`
    - `GOV_PASS_QUEUE_NUM=100`
    - `GOV_PASS_MARK=1`
    - `GOV_PASS_ARGS=` (optional extra flags, e.g. `--auto-offload=false`)

Suggested installation:
```bash
sudo install -m 755 dist/splitter /opt/gov-pass/dist/splitter
sudo install -m 755 scripts/linux/install_nfqueue.sh /opt/gov-pass/scripts/linux/install_nfqueue.sh
sudo install -m 755 scripts/linux/uninstall_nfqueue.sh /opt/gov-pass/scripts/linux/uninstall_nfqueue.sh
sudo install -m 644 scripts/linux/gov-pass.service /etc/systemd/system/gov-pass.service
sudo systemctl daemon-reload
sudo systemctl enable --now gov-pass
```

## Android packaging (Magisk, arm64)

See `docs/DESIGN_ANDROID.md` for build and packaging details.
